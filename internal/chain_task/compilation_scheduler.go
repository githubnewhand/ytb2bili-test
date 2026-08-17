package chain_task

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/difyz9/ytb2bili/internal/chain_task/handlers"
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/services"
	"github.com/difyz9/ytb2bili/pkg/store/model"
	"github.com/difyz9/ytb2bili/pkg/utils"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type CompilationScheduler struct {
	App                *core.AppServer
	DB                 *gorm.DB
	Task               *cron.Cron
	CompilationService *services.ChargeCompilationService
	SavedVideoService  *services.SavedVideoService
	TaskStepService    *services.TaskStepService
	logger             *zap.SugaredLogger
	mu                 sync.Mutex
	running            bool
}

func NewCompilationScheduler(
	app *core.AppServer,
	db *gorm.DB,
	task *cron.Cron,
	compilationService *services.ChargeCompilationService,
	savedVideoService *services.SavedVideoService,
	taskStepService *services.TaskStepService,
) *CompilationScheduler {
	return &CompilationScheduler{
		App:                app,
		DB:                 db,
		Task:               task,
		CompilationService: compilationService,
		SavedVideoService:  savedVideoService,
		TaskStepService:    taskStepService,
		logger:             app.Logger,
	}
}

func (s *CompilationScheduler) SetUp() {
	if err := s.recoverInterruptedBatches(); err != nil {
		s.logger.Errorf("恢复中断的拼接批次失败: %v", err)
	}
	if _, err := s.Task.AddFunc("*/5 * * * * *", s.poll); err != nil {
		s.logger.Errorf("注册拼接批次调度器失败: %v", err)
		return
	}
	s.logger.Info("✓ Compilation scheduler started, checking every 5 seconds")
}

func (s *CompilationScheduler) recoverInterruptedBatches() error {
	return s.DB.Model(&model.CompilationBatch{}).
		Where("state IN ?", []string{model.CompilationStateMerging, model.CompilationStateProcessing}).
		Updates(map[string]interface{}{
			"state":      model.CompilationStateQueued,
			"last_error": "服务重启后自动恢复",
		}).Error
}

func (s *CompilationScheduler) poll() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	var batch model.CompilationBatch
	if err := s.DB.Where("state = ?", model.CompilationStateQueued).
		Order("updated_at ASC").
		First(&batch).Error; err != nil {
		s.mu.Unlock()
		if err != gorm.ErrRecordNotFound {
			s.logger.Errorf("查询待拼接批次失败: %v", err)
		}
		return
	}
	claim := s.DB.Model(&model.CompilationBatch{}).
		Where("id = ? AND state = ?", batch.ID, model.CompilationStateQueued).
		Updates(map[string]interface{}{
			"state":      model.CompilationStateMerging,
			"started_at": time.Now(),
			"last_error": "",
		})
	if claim.Error != nil || claim.RowsAffected != 1 {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()
	go func(batchID uint) {
		defer func() {
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
		}()
		if err := s.processBatch(batchID); err != nil {
			s.logger.Errorf("拼接批次 %d 处理失败: %v", batchID, err)
		}
	}(batch.ID)
}

func (s *CompilationScheduler) processBatch(batchID uint) error {
	batch, err := s.CompilationService.GetBatch(batchID)
	if err != nil {
		return s.failBatch(batchID, nil, model.CompilationStateMergeFailed, fmt.Errorf("读取批次失败: %w", err))
	}
	outputVideo, err := s.ensureOutputVideo(batch)
	if err != nil {
		return s.failBatch(batchID, nil, model.CompilationStateMergeFailed, err)
	}
	root, err := filepath.Abs(s.App.Config.FileUpDir)
	if err != nil {
		return s.failBatch(batchID, outputVideo, model.CompilationStateMergeFailed, err)
	}
	stateManager := manager.NewStateManager(outputVideo.ID, outputVideo.VideoID, root, outputVideo.CreatedAt)
	stateManager.InputVideoPath = outputVideo.MediaPath
	if stateManager.InputVideoPath == "" {
		stateManager.InputVideoPath = filepath.Join(stateManager.CurrentDir, outputVideo.VideoID+".mp4")
	}

	transcriptionStep := "生成字幕"
	if s.App.Config.WhisperConfig != nil && s.App.Config.WhisperConfig.Enabled {
		transcriptionStep = "B站必剪转录"
	}
	stepNames := []string{
		"拼接视频",
		"生成组合封面",
		"分离音频",
		transcriptionStep,
		"翻译字幕",
		"生成元数据",
		"上传到Bilibili",
		"上传字幕到Bilibili",
	}
	if err := s.TaskStepService.InitTaskPlan(outputVideo.VideoID, stepNames); err != nil {
		return s.failBatch(batchID, outputVideo, model.CompilationStateMergeFailed, err)
	}
	result, reusedOutput := s.existingCompilationResult(batch, outputVideo)
	if reusedOutput {
		stateManager.InputVideoPath = result.OutputPath
		if err := s.TaskStepService.UpdateTaskStepStatus(outputVideo.VideoID, "拼接视频", model.TaskStepStatusCompleted); err != nil {
			s.logger.Warnf("保存拼接恢复状态失败: %v", err)
		}
		s.logger.Infof("复用已完成拼接成片: %s", result.OutputPath)
	} else {
		if err := s.TaskStepService.UpdateTaskStepStatus(outputVideo.VideoID, "拼接视频", model.TaskStepStatusRunning); err != nil {
			s.logger.Warnf("更新拼接步骤状态失败: %v", err)
		}

		sources := make([]utils.CompilationSource, 0, len(batch.Items))
		for _, item := range batch.Items {
			path := item.SourcePathSnapshot
			if item.SourceSavedVideo.MediaPath != "" {
				path = item.SourceSavedVideo.MediaPath
			}
			sources = append(sources, utils.CompilationSource{
				VideoID: item.SourceSavedVideo.VideoID,
				Title:   item.SourceSavedVideo.Title,
				Path:    path,
			})
		}
		options := utils.CompilationOptions{
			Width:        s.App.Config.ChargeCompilation.TargetWidth,
			Height:       s.App.Config.ChargeCompilation.TargetHeight,
			FPS:          s.App.Config.ChargeCompilation.TargetFPS,
			CRF:          s.App.Config.ChargeCompilation.VideoCRF,
			Preset:       s.App.Config.ChargeCompilation.VideoPreset,
			AudioBitrate: s.App.Config.ChargeCompilation.AudioBitrate,
		}
		buildContext, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
		var buildErr error
		result, buildErr = utils.BuildVideoCompilation(
			buildContext,
			sources,
			stateManager.CurrentDir,
			stateManager.InputVideoPath,
			options,
			func(percent int, message string) {
				_ = s.TaskStepService.UpdateTaskStepProgress(outputVideo.VideoID, "拼接视频", percent, message)
			},
		)
		cancel()
		if buildErr != nil {
			_ = s.TaskStepService.UpdateTaskStepStatus(outputVideo.VideoID, "拼接视频", model.TaskStepStatusFailed, buildErr.Error())
			return s.failBatch(batchID, outputVideo, model.CompilationStateMergeFailed, buildErr)
		}
		if err := s.TaskStepService.UpdateTaskStepStatus(outputVideo.VideoID, "拼接视频", model.TaskStepStatusCompleted); err != nil {
			s.logger.Warnf("保存拼接完成状态失败: %v", err)
		}
		if err := s.persistCompilationResult(batch, outputVideo, result); err != nil {
			return s.failBatch(batchID, outputVideo, model.CompilationStateMergeFailed, err)
		}
	}

	if err := s.DB.Model(&model.CompilationBatch{}).Where("id = ?", batch.ID).
		Update("state", model.CompilationStateProcessing).Error; err != nil {
		return s.failBatch(batchID, outputVideo, model.CompilationStateProcessFailed, err)
	}
	if !s.completedStepFileAvailable(outputVideo.VideoID, "生成组合封面", stateManager.ImageCover) {
		if err := s.generateCompilationCover(outputVideo.VideoID, stateManager); err != nil {
			return s.failBatch(batchID, outputVideo, model.CompilationStateProcessFailed, err)
		}
	}

	chain := manager.NewTaskChain()
	resumeProcessing := !s.completedStepFileAvailable(outputVideo.VideoID, "分离音频", stateManager.OriginalWAV)
	if resumeProcessing {
		chain.AddTask(s.wrapTask(
			handlers.NewExtractAudio("分离音频", s.App, stateManager, s.App.CosClient),
			outputVideo.VideoID,
		))
	}
	if resumeProcessing || !s.completedStepFileAvailable(outputVideo.VideoID, transcriptionStep, stateManager.OriginalSRT) {
		resumeProcessing = true
		if s.App.Config.WhisperConfig != nil && s.App.Config.WhisperConfig.Enabled {
			chain.AddTask(s.wrapTask(handlers.NewBcutHandler(
				"B站必剪转录",
				s.App,
				stateManager,
				s.App.CosClient,
				s.App.Config.WhisperConfig.Language,
			), outputVideo.VideoID))
		} else {
			chain.AddTask(s.wrapTask(
				handlers.NewGenerateSubtitles("生成字幕", s.App, stateManager, s.App.CosClient, s.SavedVideoService),
				outputVideo.VideoID,
			))
		}
	}
	if resumeProcessing || !s.completedStepFileAvailable(outputVideo.VideoID, "翻译字幕", stateManager.TranslateSRT) {
		resumeProcessing = true
		chain.AddTask(s.wrapTask(
			handlers.NewTranslateSubtitle("翻译字幕", s.App, stateManager, s.App.CosClient, s.DB, ""),
			outputVideo.VideoID,
		))
	}
	if resumeProcessing || !s.completedStepFileAvailable(outputVideo.VideoID, "生成元数据", filepath.Join(stateManager.CurrentDir, "meta.json")) {
		chain.AddTask(s.wrapTask(
			handlers.NewGenerateMetadata("生成元数据", s.App, stateManager, s.App.CosClient, "", s.DB, s.SavedVideoService),
			outputVideo.VideoID,
		))
	}
	processingResult := chain.Run(true)
	if errorValue, exists := processingResult["error"]; exists && errorValue != nil {
		return s.failBatch(
			batchID,
			outputVideo,
			model.CompilationStateProcessFailed,
			fmt.Errorf("成片处理失败: %v", errorValue),
		)
	}

	now := time.Now()
	scheduledAt := now
	batchState := model.CompilationStateReady
	if batch.UploadPolicy == model.UploadPolicyImmediate {
		batchState = model.CompilationStateUploadQueued
	}
	videoUpdates := map[string]interface{}{
		"status":                   "200",
		"workflow_state":           model.WorkflowStateReady,
		"ready_at":                 &now,
		"media_path":               result.OutputPath,
		"media_duration_ms":        result.DurationMS,
		"media_probe_json":         result.Probe.RawJSON,
		"classification_locked_at": &now,
	}
	if batch.UploadPolicy == model.UploadPolicyImmediate {
		videoUpdates["scheduled_upload_at"] = &scheduledAt
	} else {
		videoUpdates["scheduled_upload_at"] = nil
	}
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.SavedVideo{}).Where("id = ?", outputVideo.ID).Updates(videoUpdates).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.CompilationBatch{}).Where("id = ?", batch.ID).Updates(map[string]interface{}{
			"state":        batchState,
			"output_path":  result.OutputPath,
			"completed_at": &now,
			"last_error":   "",
		}).Error; err != nil {
			return err
		}
		if batch.UploadPolicy == model.UploadPolicyImmediate {
			return tx.Create(&model.UploadJob{
				SavedVideoID:       outputVideo.ID,
				CompilationBatchID: &batch.ID,
				JobType:            "video",
				TriggerType:        "compilation",
				ScheduledAt:        scheduledAt,
				Status:             model.UploadJobStatusQueued,
			}).Error
		}
		return nil
	}); err != nil {
		return s.failBatch(batchID, outputVideo, model.CompilationStateProcessFailed, err)
	}
	s.logger.Infof("✅ 拼接批次 %s 已完成处理", batch.BatchKey)
	return nil
}

func (s *CompilationScheduler) existingCompilationResult(batch *model.CompilationBatch, outputVideo *model.SavedVideo) (*utils.CompilationResult, bool) {
	if batch == nil || outputVideo == nil {
		return nil, false
	}
	path := strings.TrimSpace(batch.OutputPath)
	if path == "" {
		path = strings.TrimSpace(outputVideo.MediaPath)
	}
	expectedDuration := batch.TotalDurationMS
	if expectedDuration <= 0 {
		expectedDuration = outputVideo.MediaDurationMS
	}
	if path == "" || expectedDuration <= 0 {
		return nil, false
	}
	probeContext, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	probe, err := utils.ProbeMediaFile(probeContext, path)
	if err != nil {
		return nil, false
	}
	difference := probe.DurationMS - expectedDuration
	if difference < 0 {
		difference = -difference
	}
	if difference > 10000 {
		return nil, false
	}
	return &utils.CompilationResult{
		OutputPath:   path,
		DurationMS:   probe.DurationMS,
		Probe:        *probe,
		ManifestJSON: batch.ManifestJSON,
	}, true
}

func (s *CompilationScheduler) completedStepFileAvailable(videoID, stepName, path string) bool {
	step, err := s.TaskStepService.GetTaskStepByName(videoID, stepName)
	if err != nil || step.Status != model.TaskStepStatusCompleted {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func (s *CompilationScheduler) ensureOutputVideo(batch *model.CompilationBatch) (*model.SavedVideo, error) {
	if batch.OutputSavedVideoID != nil {
		video, err := s.SavedVideoService.GetVideoByID(*batch.OutputSavedVideoID)
		if err != nil {
			return nil, err
		}
		if err := s.DB.Model(video).Updates(map[string]interface{}{
			"status":         "002",
			"workflow_state": model.WorkflowStatePreparing,
		}).Error; err != nil {
			return nil, err
		}
		return video, nil
	}
	audience, ok := services.ChargeAudienceFromTier(batch.Tier)
	if !ok {
		return nil, fmt.Errorf("批次档位无效: %d", batch.Tier)
	}
	video := &model.SavedVideo{
		VideoID:              batch.BatchKey,
		URL:                  "",
		Title:                fmt.Sprintf("%d元充电视频合集 %s", batch.Tier, batch.BatchKey),
		Status:               "002",
		OperationType:        "compilation",
		PublishAudience:      audience,
		AudienceSelectedAt:   &batch.CreatedAt,
		UPowerPreviewSeconds: batch.PreviewSeconds,
		RecordType:           model.RecordTypeCompilation,
		WorkflowState:        model.WorkflowStatePreparing,
		UploadPolicy:         batch.UploadPolicy,
		RightsVerified:       true,
	}
	if err := s.SavedVideoService.CreateVideo(video); err != nil {
		return nil, err
	}
	if err := s.DB.Model(&model.CompilationBatch{}).Where("id = ?", batch.ID).
		Update("output_saved_video_id", video.ID).Error; err != nil {
		return nil, err
	}
	return video, nil
}

func (s *CompilationScheduler) persistCompilationResult(
	batch *model.CompilationBatch,
	outputVideo *model.SavedVideo,
	result *utils.CompilationResult,
) error {
	chapters := make([]string, 0, len(result.Segments))
	return s.DB.Transaction(func(tx *gorm.DB) error {
		for index, segment := range result.Segments {
			chapters = append(chapters, fmt.Sprintf("%s %s", formatChapterTime(segment.StartMS), segment.Title))
			if index < len(batch.Items) {
				if err := tx.Model(&model.CompilationItem{}).Where("id = ?", batch.Items[index].ID).
					Updates(map[string]interface{}{
						"source_duration_ms": segment.DurationMS,
						"timeline_start_ms":  segment.StartMS,
						"timeline_end_ms":    segment.EndMS,
					}).Error; err != nil {
					return err
				}
			}
		}
		chapterText := strings.Join(chapters, "\n")
		if err := tx.Model(&model.SavedVideo{}).Where("id = ?", outputVideo.ID).Updates(map[string]interface{}{
			"media_path":        result.OutputPath,
			"media_duration_ms": result.DurationMS,
			"media_probe_json":  result.Probe.RawJSON,
			"chapters":          chapterText,
			"description":       chapterText,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.CompilationBatch{}).Where("id = ?", batch.ID).Updates(map[string]interface{}{
			"output_path":       result.OutputPath,
			"total_duration_ms": result.DurationMS,
			"manifest_json":     result.ManifestJSON,
		}).Error
	})
}

func (s *CompilationScheduler) generateCompilationCover(videoID string, stateManager *manager.StateManager) error {
	if err := s.TaskStepService.UpdateTaskStepStatus(videoID, "生成组合封面", model.TaskStepStatusRunning); err != nil {
		s.logger.Warnf("更新封面步骤状态失败: %v", err)
	}
	if err := utils.ExtractThumbnail(stateManager.InputVideoPath, stateManager.ImageCover); err != nil {
		_ = s.TaskStepService.UpdateTaskStepStatus(videoID, "生成组合封面", model.TaskStepStatusFailed, err.Error())
		return fmt.Errorf("生成组合封面失败: %w", err)
	}
	return s.TaskStepService.UpdateTaskStepStatus(videoID, "生成组合封面", model.TaskStepStatusCompleted)
}

func (s *CompilationScheduler) wrapTask(task interface {
	Execute(map[string]interface{}) bool
	GetName() string
	InsertTask() error
	UpdateStatus(status, message string) error
}, videoID string) *TaskStepWrapper {
	return &TaskStepWrapper{
		task:            task,
		videoID:         videoID,
		taskStepService: s.TaskStepService,
		logger:          s.logger,
	}
}

func (s *CompilationScheduler) failBatch(batchID uint, outputVideo *model.SavedVideo, state string, cause error) error {
	message := cause.Error()
	_ = s.DB.Model(&model.CompilationBatch{}).Where("id = ?", batchID).Updates(map[string]interface{}{
		"state":      state,
		"last_error": message,
	}).Error
	if outputVideo != nil {
		_ = s.DB.Model(&model.SavedVideo{}).Where("id = ?", outputVideo.ID).Updates(map[string]interface{}{
			"status":         "999",
			"workflow_state": model.WorkflowStateFailed,
		}).Error
	}
	return cause
}

func formatChapterTime(milliseconds int64) string {
	totalSeconds := milliseconds / 1000
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}
