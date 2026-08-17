package chain_task

import (
	"fmt"
	"github.com/difyz9/ytb2bili/internal/chain_task/handlers"
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/services"
	"github.com/difyz9/ytb2bili/internal/core/types"
	"github.com/difyz9/ytb2bili/pkg/store/model"
	"path/filepath"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const autoUploadDelay = time.Hour

// UploadScheduler 上传调度器
// 负责定时上传视频和字幕到Bilibili
type UploadScheduler struct {
	App               *core.AppServer
	SavedVideoService *services.SavedVideoService
	TaskStepService   *services.TaskStepService
	Db                *gorm.DB
	Task              *cron.Cron
	mutex             sync.Mutex
	logger            *zap.SugaredLogger

	// 上传队列跟踪
	lastVideoUploadTime    time.Time // 最后一次视频上传时间
	lastSubtitleUploadTime time.Time // 最后一次字幕上传时间
}

// NewUploadScheduler 创建上传调度器实例
func NewUploadScheduler(
	app *core.AppServer,
	task *cron.Cron,
	db *gorm.DB,
	savedVideoService *services.SavedVideoService,
	taskStepService *services.TaskStepService,
) *UploadScheduler {
	return &UploadScheduler{
		App:               app,
		Task:              task,
		Db:                db,
		SavedVideoService: savedVideoService,
		TaskStepService:   taskStepService,
		logger:            app.Logger,
	}
}

// SetUp 启动上传调度器
func (s *UploadScheduler) SetUp() {
	if err := s.recoverOrphanedUploadTasks(); err != nil {
		s.logger.Errorf("恢复中断的上传任务失败: %v", err)
	}
	if err := s.recoverUploadJobs(); err != nil {
		s.logger.Errorf("恢复持久化上传任务失败: %v", err)
	}

	// 每分钟检查一次是否需要自动上传
	if _, err := s.Task.AddFunc("@every 1m", func() {
		s.mutex.Lock()
		defer s.mutex.Unlock()

		// 1. 检查是否有准备好超过10分钟且未手动上传的视频
		s.logger.Info("🔍 检查待自动上传的视频...")
		if err := s.uploadNextVideo(); err != nil {
			s.logger.Errorf("上传视频失败: %v", err)
		} else {
			s.lastVideoUploadTime = time.Now()
		}

		// 字幕上传改为手动触发（失败后由前端重试按钮触发）
	}); err != nil {
		s.logger.Errorf("注册上传调度任务失败: %v", err)
		return
	}

	s.logger.Info("✓ Upload scheduler started, checking every minute")
}

// uploadNextVideo claims one durable upload job and executes it.
func (s *UploadScheduler) uploadNextVideo() error {
	if err := s.enqueueDueVideoJobs(); err != nil {
		return err
	}
	job, err := s.claimNextVideoUploadJob()
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			s.logger.Debug("没有待自动上传的视频")
			return nil
		}
		return err
	}
	return s.executeClaimedVideoJob(job)
}

func (s *UploadScheduler) enqueueDueVideoJobs() error {
	now := time.Now()
	legacyBefore := now.Add(-autoUploadDelay)
	var videos []model.SavedVideo
	err := s.Db.
		Where("status = ?", "200").
		Where(
			"(publish_audience = ? AND record_type <> ? AND upload_policy <> ?) OR (record_type = ? AND publish_audience IN ? AND upload_policy = ?)",
			"free",
			model.RecordTypeCompilation,
			model.UploadPolicyManual,
			model.RecordTypeCompilation,
			[]string{"charge_30", "charge_50"},
			model.UploadPolicyImmediate,
		).
		Where(
			"(scheduled_upload_at IS NOT NULL AND scheduled_upload_at <= ?) OR (scheduled_upload_at IS NULL AND audience_selected_at IS NOT NULL AND audience_selected_at <= ?)",
			now,
			legacyBefore,
		).
		Order("COALESCE(scheduled_upload_at, audience_selected_at) ASC").
		Limit(10).
		Find(&videos).Error
	if err != nil {
		return fmt.Errorf("查询到期上传视频失败: %w", err)
	}
	for _, video := range videos {
		var active int64
		if err := s.Db.Model(&model.UploadJob{}).
			Where("saved_video_id = ? AND job_type = ? AND status IN ?", video.ID, "video", []string{model.UploadJobStatusQueued, model.UploadJobStatusRunning}).
			Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			continue
		}
		scheduledAt := now
		if video.ScheduledUploadAt != nil {
			scheduledAt = *video.ScheduledUploadAt
		}
		trigger := "scheduled"
		var batchID *uint
		if video.RecordType == model.RecordTypeCompilation {
			trigger = "compilation"
			var batch model.CompilationBatch
			if err := s.Db.Where("output_saved_video_id = ?", video.ID).First(&batch).Error; err == nil {
				batchID = &batch.ID
			}
		}
		if err := s.Db.Create(&model.UploadJob{
			SavedVideoID:       video.ID,
			CompilationBatchID: batchID,
			JobType:            "video",
			TriggerType:        trigger,
			ScheduledAt:        scheduledAt,
			Status:             model.UploadJobStatusQueued,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *UploadScheduler) claimNextVideoUploadJob() (*model.UploadJob, error) {
	var claimed model.UploadJob
	err := s.Db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		var job model.UploadJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("job_type = ? AND status = ? AND scheduled_at <= ?", "video", model.UploadJobStatusQueued, now).
			Order("scheduled_at ASC").
			First(&job).Error; err != nil {
			return err
		}
		leaseUntil := now.Add(6 * time.Hour)
		update := tx.Model(&model.UploadJob{}).
			Where("id = ? AND status = ?", job.ID, model.UploadJobStatusQueued).
			Updates(map[string]interface{}{
				"status":      model.UploadJobStatusRunning,
				"lease_until": &leaseUntil,
				"started_at":  &now,
				"attempts":    gorm.Expr("attempts + 1"),
				"last_error":  "",
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return tx.First(&claimed, job.ID).Error
	})
	if err != nil {
		return nil, err
	}
	return &claimed, nil
}

func (s *UploadScheduler) executeClaimedVideoJob(job *model.UploadJob) error {
	video, err := s.SavedVideoService.GetVideoByID(job.SavedVideoID)
	if err != nil {
		s.finishUploadJob(job.ID, model.UploadJobStatusFailed, err)
		return err
	}
	if video.BiliBVID != "" || video.BiliAID != 0 {
		s.finishUploadJob(job.ID, model.UploadJobStatusSucceeded, nil)
		return nil
	}
	if video.PublishAudience == "" {
		err := fmt.Errorf("尚未选择发布范围")
		s.finishUploadJob(job.ID, model.UploadJobStatusFailed, err)
		return err
	}
	if video.RecordType != model.RecordTypeCompilation &&
		(video.PublishAudience == "charge_30" || video.PublishAudience == "charge_50") {
		err := fmt.Errorf("充电源素材不能单独上传")
		s.finishUploadJob(job.ID, model.UploadJobStatusFailed, err)
		return err
	}
	claimed, err := s.SavedVideoService.TryUpdateStatus(video.ID, video.Status, "201")
	if err != nil || !claimed {
		if err == nil {
			err = fmt.Errorf("视频状态已变化，上传任务未执行")
		}
		s.finishUploadJob(job.ID, model.UploadJobStatusFailed, err)
		return err
	}
	_ = s.Db.Model(&model.SavedVideo{}).Where("id = ?", video.ID).
		Update("workflow_state", model.WorkflowStateUploading).Error
	if job.CompilationBatchID != nil {
		_ = s.Db.Model(&model.CompilationBatch{}).Where("id = ?", *job.CompilationBatchID).
			Update("state", model.CompilationStateUploading).Error
	}
	if err := s.executeUploadTask(video.VideoID, "上传到Bilibili"); err != nil {
		_ = s.SavedVideoService.UpdateStatus(video.ID, "299")
		_ = s.Db.Model(&model.SavedVideo{}).Where("id = ?", video.ID).
			Update("workflow_state", model.WorkflowStateFailed).Error
		if job.CompilationBatchID != nil {
			_ = s.Db.Model(&model.CompilationBatch{}).Where("id = ?", *job.CompilationBatchID).
				Updates(map[string]interface{}{"state": model.CompilationStateUploadFailed, "last_error": err.Error()}).Error
		}
		s.finishUploadJob(job.ID, model.UploadJobStatusFailed, err)
		return fmt.Errorf("上传视频失败: %w", err)
	}
	if err := s.SavedVideoService.UpdateStatus(video.ID, "300"); err != nil {
		s.finishUploadJob(job.ID, model.UploadJobStatusFailed, err)
		return err
	}
	_ = s.Db.Model(&model.SavedVideo{}).Where("id = ?", video.ID).
		Update("workflow_state", model.WorkflowStateUploaded).Error
	if job.CompilationBatchID != nil {
		if err := s.markCompilationUploaded(*job.CompilationBatchID); err != nil {
			s.logger.Errorf("标记拼接批次素材已使用失败: %v", err)
		}
	}
	s.finishUploadJob(job.ID, model.UploadJobStatusSucceeded, nil)
	s.logger.Infof("✅ 视频上传成功: %s", video.VideoID)
	return nil
}

func (s *UploadScheduler) finishUploadJob(jobID uint, status string, cause error) {
	now := time.Now()
	updates := map[string]interface{}{
		"status":      status,
		"lease_until": nil,
		"finished_at": &now,
	}
	if cause != nil {
		updates["last_error"] = cause.Error()
	}
	if err := s.Db.Model(&model.UploadJob{}).Where("id = ?", jobID).Updates(updates).Error; err != nil {
		s.logger.Errorf("保存上传任务状态失败: %v", err)
	}
}

func (s *UploadScheduler) markCompilationUploaded(batchID uint) error {
	return s.Db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Model(&model.CompilationBatch{}).Where("id = ?", batchID).Updates(map[string]interface{}{
			"state":        model.CompilationStateUploaded,
			"completed_at": &now,
			"last_error":   "",
		}).Error; err != nil {
			return err
		}
		var poolItems []model.ChargePoolItem
		if err := tx.Where("reserved_batch_id = ? AND state = ?", batchID, model.ChargePoolStateReserved).
			Find(&poolItems).Error; err != nil {
			return err
		}
		for _, item := range poolItems {
			if err := tx.Model(&item).Updates(map[string]interface{}{
				"state":       model.ChargePoolStateConsumed,
				"consumed_at": &now,
				"version":     gorm.Expr("version + 1"),
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *UploadScheduler) recoverUploadJobs() error {
	var jobs []model.UploadJob
	if err := s.Db.Where("status = ?", model.UploadJobStatusRunning).Find(&jobs).Error; err != nil {
		return err
	}
	const unknownMessage = "服务重启时上传结果未知，请先在B站创作中心核对稿件，再决定是否重试"
	for _, job := range jobs {
		var video model.SavedVideo
		err := s.Db.Transaction(func(tx *gorm.DB) error {
			now := time.Now()
			if err := tx.Model(&model.UploadJob{}).Where("id = ? AND status = ?", job.ID, model.UploadJobStatusRunning).
				Updates(map[string]interface{}{
					"status":      model.UploadJobStatusUnknown,
					"lease_until": nil,
					"finished_at": &now,
					"last_error":  unknownMessage,
				}).Error; err != nil {
				return err
			}
			if err := tx.First(&video, job.SavedVideoID).Error; err != nil {
				return err
			}
			if err := tx.Model(&video).Updates(map[string]interface{}{
				"status":         "299",
				"workflow_state": model.WorkflowStateFailed,
			}).Error; err != nil {
				return err
			}
			if job.CompilationBatchID != nil {
				if err := tx.Model(&model.CompilationBatch{}).Where("id = ?", *job.CompilationBatchID).
					Updates(map[string]interface{}{
						"state":      model.CompilationStateUploadFailed,
						"last_error": unknownMessage,
					}).Error; err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if err := s.TaskStepService.UpdateTaskStepStatus(video.VideoID, "上传到Bilibili", model.TaskStepStatusFailed, unknownMessage); err != nil {
			s.logger.Warnf("保存未知上传结果提示失败 (videoID=%s): %v", video.VideoID, err)
		}
	}
	return nil
}

// recoverOrphanedUploadTasks 恢复服务重启或 goroutine 中断后遗留的上传中状态。
func (s *UploadScheduler) recoverOrphanedUploadTasks() error {
	type orphanedUpload struct {
		ID      uint
		VideoID string
		Status  string
	}

	var videos []orphanedUpload
	err := s.Db.Model(&model.SavedVideo{}).
		Select("tb_saved_videos.id, tb_saved_videos.video_id, tb_saved_videos.status").
		Joins("LEFT JOIN tb_task_steps ON tb_task_steps.video_id = tb_saved_videos.video_id AND tb_task_steps.step_name = ?", "上传到Bilibili").
		Where("tb_saved_videos.status = ?", "201").
		Where("tb_task_steps.id IS NULL OR tb_task_steps.status IN ?", []string{model.TaskStepStatusPending, model.TaskStepStatusRunning}).
		Find(&videos).Error
	if err != nil {
		return fmt.Errorf("查询中断的视频上传任务失败: %w", err)
	}

	for _, video := range videos {
		if err := s.TaskStepService.ResetTaskStep(video.VideoID, "上传到Bilibili"); err != nil {
			s.logger.Warnf("重置上传步骤失败 (videoID=%s): %v", video.VideoID, err)
		}
		if err := s.SavedVideoService.UpdateStatus(video.ID, "200"); err != nil {
			return fmt.Errorf("恢复视频 %s 到待上传状态失败: %w", video.VideoID, err)
		}
		s.logger.Warnf("已恢复中断的视频上传任务: %s (201 -> 200)", video.VideoID)
	}

	if len(videos) > 0 {
		s.logger.Infof("已恢复 %d 个中断的视频上传任务", len(videos))
	}
	return nil
}

// uploadNextSubtitle 上传下一个待上传字幕的视频
func (s *UploadScheduler) uploadNextSubtitle() error {
	// 查询状态为 '300' (视频已上传，待上传字幕) 且上传时间超过1小时的视频
	var videos []struct {
		ID        uint
		VideoID   string
		Title     string
		UpdatedAt time.Time
		CreatedAt time.Time
	}

	oneHourAgo := time.Now().Add(-time.Hour)

	err := s.Db.Model(&model.SavedVideo{}).
		Select("id, video_id, title, updated_at, created_at").
		Where("status = ? AND updated_at <= ?", "300", oneHourAgo).
		Order("updated_at ASC").
		Limit(1).
		Find(&videos).Error

	if err != nil {
		return fmt.Errorf("查询待上传字幕的视频失败: %v", err)
	}

	if len(videos) == 0 {
		s.logger.Debug("没有待上传字幕的视频")
		return nil
	}

	video := videos[0]
	s.logger.Infof("📝 开始上传字幕: %s (VideoID: %s)", video.Title, video.VideoID)

	// 更新状态为 '301' (上传字幕中)
	if err := s.SavedVideoService.UpdateStatus(video.ID, "301"); err != nil {
		return fmt.Errorf("更新视频状态失败: %v", err)
	}

	// 执行上传字幕任务
	if err := s.executeUploadTask(video.VideoID, "上传字幕到Bilibili"); err != nil {
		// 上传失败，更新状态为 '399' (字幕上传失败)
		s.SavedVideoService.UpdateStatus(video.ID, "399")
		return fmt.Errorf("上传字幕失败: %v", err)
	}

	// 上传成功，更新状态为 '400' (全部完成)
	if err := s.SavedVideoService.UpdateStatus(video.ID, "400"); err != nil {
		return fmt.Errorf("更新视频状态失败: %v", err)
	}

	s.logger.Infof("✅ 字幕上传成功: %s", video.VideoID)
	return nil
}

// executeUploadTask 执行上传任务
func (s *UploadScheduler) executeUploadTask(videoID, taskName string) error {
	// 获取视频信息
	savedVideo, err := s.SavedVideoService.GetVideoByVideoID(videoID)
	if err != nil {
		return fmt.Errorf("获取视频信息失败: %v", err)
	}

	// 获取当前目录
	currentDir, err := filepath.Abs(s.App.Config.FileUpDir)
	if err != nil {
		return fmt.Errorf("获取文件上传目录失败: %v", err)
	}

	// 创建状态管理器
	stateManager := manager.NewStateManager(savedVideo.ID, savedVideo.VideoID, currentDir, savedVideo.CreatedAt)

	// 更新步骤状态为运行中
	if err := s.TaskStepService.UpdateTaskStepStatus(videoID, taskName, "running"); err != nil {
		s.logger.Errorf("更新任务步骤状态失败: %v", err)
	}

	// 创建任务链
	chain := manager.NewTaskChain()

	// 根据任务名称创建对应的任务
	switch taskName {
	case "上传到Bilibili":
		chain.AddTask(handlers.NewExtractChaptersHandler("提取视频章节", s.App, stateManager, s.App.CosClient, s.SavedVideoService))
		chain.AddTask(handlers.NewUploadToBilibili("上传到Bilibili", s.App, stateManager, s.App.CosClient, s.SavedVideoService))
	case "上传字幕到Bilibili":
		chain.AddTask(handlers.NewUploadSubtitleToBilibili("上传字幕到Bilibili", s.App, stateManager, s.App.CosClient, s.SavedVideoService))
		chain.AddTask(handlers.NewPublishCommentHandler("发布章节评论", s.App, stateManager, s.App.CosClient, s.SavedVideoService))
	default:
		return fmt.Errorf("未知的任务类型: %s", taskName)
	}

	chain.Context[types.TaskProgressReporterKey] = newTaskProgressReporter(videoID, taskName, s.TaskStepService, s.logger)
	types.ReportTaskProgress(chain.Context, 0, "开始执行")

	s.logger.Infof("开始执行上传任务: %s (VideoID: %s)", taskName, videoID)

	// 执行任务
	result := chain.Run(true)

	// 检查执行结果
	success := true
	var errorMsg string
	if errorMsgInterface, exists := result["error"]; exists && errorMsgInterface != nil {
		success = false
		errorMsg = fmt.Sprintf("%v", errorMsgInterface)
	}

	// 更新步骤状态
	if success {
		types.ReportTaskProgress(chain.Context, 100, "已完成")
		if err := s.TaskStepService.UpdateTaskStepStatus(videoID, taskName, "completed"); err != nil {
			s.logger.Errorf("更新任务步骤状态失败: %v", err)
		}
		if err := s.TaskStepService.UpdateTaskStepResult(videoID, taskName, result); err != nil {
			s.logger.Errorf("更新任务步骤结果失败: %v", err)
		}
		s.logger.Infof("任务 %s 执行成功", taskName)
		return nil
	} else {
		if err := s.TaskStepService.UpdateTaskStepStatus(videoID, taskName, "failed", errorMsg); err != nil {
			s.logger.Errorf("更新任务步骤状态失败: %v", err)
		}
		s.logger.Errorf("任务 %s 执行失败: %s", taskName, errorMsg)
		return fmt.Errorf("任务执行失败: %s", errorMsg)
	}
}

// ExecuteManualUpload queues video uploads durably. Subtitle uploads retain the
// existing synchronous execution path because they depend on an existing BVID.
func (s *UploadScheduler) ExecuteManualUpload(videoID, taskType string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.logger.Infof("🎯 手动执行上传任务: VideoID=%s, TaskType=%s", videoID, taskType)
	if taskType == "subtitle" {
		return s.executeUploadTask(videoID, "上传字幕到Bilibili")
	}
	if taskType != "video" {
		return fmt.Errorf("未知的任务类型: %s", taskType)
	}
	video, err := s.SavedVideoService.GetVideoByVideoID(videoID)
	if err != nil {
		return err
	}
	var active int64
	if err := s.Db.Model(&model.UploadJob{}).
		Where("saved_video_id = ? AND job_type = ? AND status IN ?", video.ID, "video", []string{model.UploadJobStatusQueued, model.UploadJobStatusRunning}).
		Count(&active).Error; err != nil {
		return err
	}
	if active > 0 {
		return fmt.Errorf("上传任务已经在队列中")
	}
	now := time.Now()
	var batchID *uint
	if video.RecordType == model.RecordTypeCompilation {
		var batch model.CompilationBatch
		if err := s.Db.Where("output_saved_video_id = ?", video.ID).First(&batch).Error; err == nil {
			batchID = &batch.ID
		}
	}
	return s.Db.Create(&model.UploadJob{
		SavedVideoID:       video.ID,
		CompilationBatchID: batchID,
		JobType:            "video",
		TriggerType:        "manual",
		ScheduledAt:        now,
		Status:             model.UploadJobStatusQueued,
	}).Error
}
