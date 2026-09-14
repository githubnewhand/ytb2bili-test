package chain_task

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/difyz9/ytb2bili/internal/chain_task/handlers"
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	models2 "github.com/difyz9/ytb2bili/internal/core/models"
	"github.com/difyz9/ytb2bili/internal/core/services"
	"github.com/difyz9/ytb2bili/internal/core/types"
	"github.com/difyz9/ytb2bili/pkg/store/model"

	"sync"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"gorm.io/gorm"
)

// ChainTaskHandler 任务链执行器的实现
type ChainTaskHandler struct {
	App *core.AppServer

	SavedVideoService *services.SavedVideoService
	TaskStepService   *services.TaskStepService

	Task          *cron.Cron
	Db            *gorm.DB
	mutex         sync.Mutex
	runningVideos map[string]struct{}
}

func NewChainTaskHandler(app *core.AppServer, task *cron.Cron, db *gorm.DB, savedVideoService *services.SavedVideoService, taskStepService *services.TaskStepService) *ChainTaskHandler {
	return &ChainTaskHandler{
		App:               app,
		Task:              task,
		Db:                db,
		SavedVideoService: savedVideoService,
		TaskStepService:   taskStepService,
		mutex:             sync.Mutex{},
		runningVideos:     make(map[string]struct{}),
	}
}

// SetUp 启动任务消费者
func (h *ChainTaskHandler) SetUp() {
	// 应用启动时重置所有"运行中"的任务步骤
	h.resetRunningTasksOnStartup()

	// 添加定时任务
	h.Task.AddFunc("*/5 * * * * *", func() {

		h.mutex.Lock()
		defer h.mutex.Unlock()

		maxConcurrent := h.maxConcurrentPrepare()
		availableSlots := maxConcurrent - len(h.runningVideos)
		if availableSlots <= 0 {
			h.App.Logger.Debugf("准备阶段并发已满 (%d/%d)，跳过本次调度", len(h.runningVideos), maxConcurrent)
			return
		}

		// 1. 优先处理重试的任务步骤
		retrySteps, err := h.getRetrySteps()
		if err != nil {
			h.App.Logger.Errorf("查询重试步骤失败: %v", err)
		} else if len(retrySteps) > 0 {
			h.App.Logger.Infof("发现 %d 个待重试的步骤", len(retrySteps))

			for _, step := range retrySteps {
				if availableSlots <= 0 {
					return
				}
				if h.isVideoRunningLocked(step.VideoID) {
					continue
				}
				h.markVideoRunningLocked(step.VideoID)
				availableSlots--

				go h.runRetryStep(step.VideoID, step.StepName)
				break
			}

			if availableSlots <= 0 {
				return
			}
		}

		// 2. 处理新的视频任务
		// 查询状态为 '001' 的任务
		pendingTasks, err := h.getPendingTasks(availableSlots)
		if err != nil {
			h.App.Logger.Errorf("查询待处理任务失败: %v", err)
			return
		}

		if len(pendingTasks) == 0 {
			h.App.Logger.Debug("没有待处理的任务")
			return
		}

		// 状态流转

		// 001 (待处理) → 002 (处理中) → 100 (完成) 或 999 (失败)

		for _, task := range pendingTasks {
			if availableSlots <= 0 {
				return
			}
			if h.isVideoRunningLocked(task.VideoId) {
				continue
			}

			claimed, err := h.SavedVideoService.TryUpdateStatus(task.Id, "001", "002")
			if err != nil {
				h.App.Logger.Errorf("更新任务状态为处理中时出错: %v", err)
				continue
			}
			if !claimed {
				h.App.Logger.Debugf("任务 %s 状态已变化，跳过本次调度", task.VideoId)
				continue
			}

			h.markVideoRunningLocked(task.VideoId)
			availableSlots--

			h.App.Logger.Infof("找到待处理任务，VideoId: %s", task.VideoId)
			go h.runTaskChainAsync(*task)
		}
	})

	// 启动 cron 调度器
	h.Task.Start()
	h.App.Logger.Info("✓ Cron scheduler started, checking for tasks every 5 seconds")
}

// resetRunningTasksOnStartup 应用启动时重置所有"运行中"的任务步骤
func (h *ChainTaskHandler) resetRunningTasksOnStartup() {
	h.App.Logger.Info("🔄 正在重置应用重启前的运行中任务...")

	// 重置所有"运行中"状态的任务步骤为"待执行"
	err := h.TaskStepService.ResetAllRunningTasks()
	if err != nil {
		h.App.Logger.Errorf("❌ 重置运行中任务失败: %v", err)
		return
	}

	h.App.Logger.Info("✅ 已重置所有运行中的任务步骤，它们将在下次调度时重新执行")
}

// getPendingTasks 获取状态为 '001' 的待处理任务（从 SavedVideo 表查询）
func (h *ChainTaskHandler) getPendingTasks(limit int) ([]*models2.TbVideo, error) {
	if limit <= 0 {
		return nil, nil
	}
	// 使用 SavedVideoService 查询状态为 '001' 的任务
	savedVideos, err := h.SavedVideoService.GetPendingVideos(limit)
	if err != nil {
		return nil, err
	}

	// 将 SavedVideo 转换为 TbVideo 格式
	var tasks []*models2.TbVideo
	for _, sv := range savedVideos {
		task := &models2.TbVideo{
			Id:        sv.ID,
			URL:       sv.URL,
			Title:     sv.Title,
			VideoId:   sv.VideoID,
			Status:    sv.Status,
			CreatedAt: sv.CreatedAt,
			UpdatedAt: sv.UpdatedAt,
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

func (h *ChainTaskHandler) maxConcurrentPrepare() int {
	if h.App != nil && h.App.Config.TaskConfig.MaxConcurrentPrepare > 0 {
		return h.App.Config.TaskConfig.MaxConcurrentPrepare
	}
	return 1
}

func (h *ChainTaskHandler) isVideoRunningLocked(videoID string) bool {
	_, exists := h.runningVideos[videoID]
	return exists
}

func (h *ChainTaskHandler) markVideoRunningLocked(videoID string) {
	h.runningVideos[videoID] = struct{}{}
}

func (h *ChainTaskHandler) unmarkVideoRunning(videoID string) {
	h.mutex.Lock()
	delete(h.runningVideos, videoID)
	h.mutex.Unlock()
}

func (h *ChainTaskHandler) runRetryStep(videoID, stepName string) {
	defer h.unmarkVideoRunning(videoID)

	h.App.Logger.Infof("🔄 开始重试步骤: %s - %s", videoID, stepName)
	if err := h.RunSingleTaskStep(videoID, stepName); err != nil {
		h.App.Logger.Errorf("重试步骤失败: %v", err)
	}
}

func (h *ChainTaskHandler) runTaskChainAsync(task models2.TbVideo) {
	defer h.unmarkVideoRunning(task.VideoId)

	h.App.Logger.Debugf("开始执行任务链: %s", task.VideoId)
	h.RunTaskChain(task)
	h.App.Logger.Debugf("任务链执行完成: %s", task.VideoId)
}

// getRetrySteps 获取状态为 'pending' 的重试步骤
func (h *ChainTaskHandler) getRetrySteps() ([]*model.TaskStep, error) {
	return h.TaskStepService.GetPendingSteps()
}
func (h *ChainTaskHandler) RunTaskChain(video models2.TbVideo) {

	currentDir, err := filepath.Abs(h.App.Config.FileUpDir)
	if err != nil {
		h.App.Logger.Errorf("获取文件上传目录失败: %v", err)
		// 任务失败，更新状态为失败
		if updateErr := h.SavedVideoService.UpdateStatus(video.Id, "999"); updateErr != nil {
			h.App.Logger.Errorf("更新任务状态为失败时出错: %v", updateErr)
		}
		return

	}

	// 初始化任务步骤
	if err := h.TaskStepService.InitTaskSteps(video.VideoId); err != nil {
		h.App.Logger.Errorf("初始化任务步骤失败: %v", err)
	}

	stateManager := manager.NewStateManager(video.Id, video.VideoId, currentDir, video.CreatedAt)
	chain := manager.NewTaskChain()

	//// 任务1: 下载视频
	downloadTask := handlers.NewDownloadVideo("下载视频", h.App, stateManager, h.App.CosClient, h.SavedVideoService)
	chain.AddTask(h.wrapTaskWithStepTracking(downloadTask, video.VideoId))

	// 任务2: 生成字幕文件
	extractAudioTask := handlers.NewExtractAudio("分离音频", h.App, stateManager, h.App.CosClient)
	chain.AddTask(h.wrapTaskWithStepTracking(extractAudioTask, video.VideoId))

	// 任务3: 使用 B站必剪 转录生成字幕（如果启用）
	if h.App.Config.WhisperConfig != nil && h.App.Config.WhisperConfig.Enabled {
		h.App.Logger.Info("✓ B站必剪 已启用，将使用 B站必剪 进行语音转录")
		whisperTask := handlers.NewBcutHandler(
			"B站必剪转录",
			h.App,
			stateManager,
			h.App.CosClient,
			h.App.Config.WhisperConfig.Language,
		)
		chain.AddTask(h.wrapTaskWithStepTracking(whisperTask, video.VideoId))
	} else {
		// 备用方案：使用原有的字幕生成方法
		h.App.Logger.Info("使用默认字幕生成方法")
		subtitleTask := handlers.NewGenerateSubtitles("生成字幕", h.App, stateManager, h.App.CosClient, h.SavedVideoService)
		chain.AddTask(h.wrapTaskWithStepTracking(subtitleTask, video.VideoId))
	}
	chain.AddTask(handlers.NewDownloadImgHandler("下载封面", h.App, stateManager, h.App.CosClient))
	// 任务3: 翻译字幕（动态检查配置）
	translateTask := handlers.NewTranslateSubtitle("翻译字幕", h.App, stateManager, h.App.CosClient, h.Db, "")
	chain.AddTask(h.wrapTaskWithStepTracking(translateTask, video.VideoId))

	// 任务4: 生成视频标题和描述（动态检查配置）
	metadataTask := handlers.NewGenerateMetadata("生成元数据", h.App, stateManager, h.App.CosClient, "", h.Db, h.SavedVideoService)
	chain.AddTask(h.wrapTaskWithStepTracking(metadataTask, video.VideoId))

	// 注意: 上传任务已移至 UploadScheduler 定时执行
	// - 视频上传: 每小时上传一个视频
	// - 字幕上传: 视频上传后1小时再上传字幕

	h.App.Logger.Info("开始执行任务链（准备阶段）")
	startTime := time.Now()

	// 执行任务链
	result := chain.Run(true)

	duration := time.Since(startTime)
	h.App.Logger.Infof("任务链执行完成, 耗时: %v", duration)

	// 检查任务链是否成功执行（如果context中有错误信息，则认为失败）
	success := true
	if errorMsg, exists := result["error"]; exists && errorMsg != nil {
		success = false
		h.App.Logger.Errorf("任务链执行过程中发生错误: %v", errorMsg)
	}

	// 根据执行结果更新任务状态
	if success {
		// 任务成功完成，更新状态为完成
		if err := h.updateSavedVideoStatus(video.Id, "200"); err != nil {
			h.App.Logger.Errorf("更新任务状态为完成时出错: %v", err)
		} else {
			h.App.Logger.Infof("任务 %s 执行成功，状态已更新为完成", video.VideoId)
		}
	} else {
		// 任务失败，更新状态为失败
		if err := h.updateSavedVideoStatus(video.Id, "999"); err != nil {
			h.App.Logger.Errorf("更新任务状态为失败时出错: %v", err)
		} else {
			h.App.Logger.Errorf("任务 %s 执行失败，状态已更新为失败", video.VideoId)
		}
	}

}

// RunSingleTaskStep 执行单个任务步骤
func (h *ChainTaskHandler) RunSingleTaskStep(videoID, stepName string) error {
	// 注意：此方法假设调用方已经获得了锁，因此不在这里加锁

	// 获取视频信息
	savedVideo, err := h.SavedVideoService.GetVideoByVideoID(videoID)
	if err != nil {
		return fmt.Errorf("获取视频信息失败: %v", err)
	}

	// 转换为TbVideo格式
	video := models2.TbVideo{
		Id:        savedVideo.ID,
		URL:       savedVideo.URL,
		Title:     savedVideo.Title,
		VideoId:   savedVideo.VideoID,
		Status:    savedVideo.Status,
		CreatedAt: savedVideo.CreatedAt,
		UpdatedAt: savedVideo.UpdatedAt,
	}

	// 获取当前目录
	currentDir, err := filepath.Abs(h.App.Config.FileUpDir)
	if err != nil {
		return fmt.Errorf("获取文件上传目录失败: %v", err)
	}

	// 创建状态管理器
	stateManager := manager.NewStateManager(video.Id, video.VideoId, currentDir, video.CreatedAt)
	stepOrder := 0
	if step, stepErr := h.TaskStepService.GetTaskStepByName(videoID, stepName); stepErr == nil {
		stepOrder = step.StepOrder
	}

	// 单步重试时同步主任务状态，避免前端继续显示旧失败状态
	if err := h.updateSavedVideoStatus(video.Id, getRetryRunningVideoStatus(stepOrder)); err != nil {
		h.App.Logger.Warnf("update retry-running status failed (videoID=%s, step=%s): %v", videoID, stepName, err)
	}

	// 重置步骤状态
	if err := h.TaskStepService.ResetTaskStep(videoID, stepName); err != nil {
		h.App.Logger.Errorf("重置任务步骤失败: %v", err)
	}

	// 更新步骤状态为运行中
	if err := h.TaskStepService.UpdateTaskStepStatus(videoID, stepName, "running"); err != nil {
		h.App.Logger.Errorf("更新任务步骤状态失败: %v", err)
	}

	// 创建单个任务的链
	chain := manager.NewTaskChain()
	var task types.Task

	// 根据步骤名称创建对应的任务
	switch stepName {
	case "下载视频":
		task = handlers.NewDownloadVideo("下载视频", h.App, stateManager, h.App.CosClient, h.SavedVideoService)
	case "分离音频":
		task = handlers.NewExtractAudio("分离音频", h.App, stateManager, h.App.CosClient)
	case "Whisper转录":
		// 从配置中读取 Whisper 参数
		if h.App.Config.WhisperConfig != nil && h.App.Config.WhisperConfig.Enabled {
			task = handlers.NewBcutHandler(
				"B站必剪转录",
				h.App,
				stateManager,
				h.App.CosClient,
				h.App.Config.WhisperConfig.Language,
			)
		} else {
			return fmt.Errorf("B站必剪 未启用或配置不完整")
		}
	case "生成字幕":
		task = handlers.NewGenerateSubtitles("生成字幕", h.App, stateManager, h.App.CosClient, h.SavedVideoService)
	case "翻译字幕":
		// 不再在这里检查配置，让任务运行时动态检查最新配置
		task = handlers.NewTranslateSubtitle("翻译字幕", h.App, stateManager, h.App.CosClient, h.Db, "")
	case "生成元数据":
		// 不再在这里检查配置，让任务运行时动态检查最新配置
		task = handlers.NewGenerateMetadata("生成元数据", h.App, stateManager, h.App.CosClient, "", h.Db, h.SavedVideoService)
	case "上传到Bilibili":
		chain.AddTask(handlers.NewExtractChaptersHandler("提取视频章节", h.App, stateManager, h.App.CosClient, h.SavedVideoService))
		chain.AddTask(handlers.NewUploadToBilibili("上传到Bilibili", h.App, stateManager, h.App.CosClient, h.SavedVideoService))
	case "上传字幕到Bilibili":
		chain.AddTask(handlers.NewUploadSubtitleToBilibili("上传字幕到Bilibili", h.App, stateManager, h.App.CosClient, h.SavedVideoService))
		chain.AddTask(handlers.NewPublishCommentHandler("发布章节评论", h.App, stateManager, h.App.CosClient, h.SavedVideoService))
	default:
		return fmt.Errorf("未知的任务步骤: %s", stepName)
	}

	// 添加任务到链
	if task != nil {
		chain.AddTask(task)
	}
	chain.Context[types.TaskProgressReporterKey] = newTaskProgressReporter(videoID, stepName, h.TaskStepService, h.App.Logger)
	types.ReportTaskProgress(chain.Context, 0, "开始执行")

	h.App.Logger.Infof("开始执行单个任务步骤: %s (VideoID: %s)", stepName, videoID)

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
		if err := h.TaskStepService.UpdateTaskStepStatus(videoID, stepName, "completed"); err != nil {
			h.App.Logger.Errorf("更新任务步骤状态失败: %v", err)
		}
		if err := h.TaskStepService.UpdateTaskStepResult(videoID, stepName, result); err != nil {
			h.App.Logger.Errorf("更新任务步骤结果失败: %v", err)
		}
		h.App.Logger.Infof("任务步骤 %s 执行成功", stepName)
		if err := h.updateSavedVideoStatus(video.Id, getRetrySuccessVideoStatus(stepOrder)); err != nil {
			h.App.Logger.Warnf("update retry-success status failed (videoID=%s, step=%s): %v", videoID, stepName, err)
		}
	} else {
		if err := h.TaskStepService.UpdateTaskStepStatus(videoID, stepName, "failed", errorMsg); err != nil {
			h.App.Logger.Errorf("更新任务步骤状态失败: %v", err)
		}
		if err := h.updateSavedVideoStatus(video.Id, getRetryFailedVideoStatus(stepOrder)); err != nil {
			h.App.Logger.Warnf("update retry-failed status failed (videoID=%s, step=%s): %v", videoID, stepName, err)
		}
		h.App.Logger.Errorf("任务步骤 %s 执行失败: %s", stepName, errorMsg)
		return fmt.Errorf("任务执行失败: %s", errorMsg)
	}

	return nil
}

// wrapTaskWithStepTracking 包装任务以添加步骤跟踪
func (h *ChainTaskHandler) wrapTaskWithStepTracking(task types.Task, videoID string) types.Task {
	return &TaskStepWrapper{
		task:            task,
		videoID:         videoID,
		taskStepService: h.TaskStepService,
		logger:          h.App.Logger,
	}
}

// TaskStepWrapper 任务步骤包装器
type TaskStepWrapper struct {
	task            types.Task
	videoID         string
	taskStepService *services.TaskStepService
	logger          *zap.SugaredLogger
}

func (w *TaskStepWrapper) GetName() string {
	return w.task.GetName()
}

func (w *TaskStepWrapper) InsertTask() error {
	return w.task.InsertTask()
}

func (w *TaskStepWrapper) UpdateStatus(status, message string) error {
	return w.task.UpdateStatus(status, message)
}

func (w *TaskStepWrapper) Execute(context map[string]interface{}) bool {
	stepName := w.task.GetName()

	// 更新步骤状态为运行中
	if err := w.taskStepService.UpdateTaskStepStatus(w.videoID, stepName, "running"); err != nil {
		w.logger.Errorf("更新任务步骤状态失败: %v", err)
	}
	context[types.TaskProgressReporterKey] = newTaskProgressReporter(w.videoID, stepName, w.taskStepService, w.logger)
	types.ReportTaskProgress(context, 0, "开始执行")

	// 执行原始任务
	success := w.task.Execute(context)

	// 更新步骤状态
	if success {
		types.ReportTaskProgress(context, 100, "已完成")
		if err := w.taskStepService.UpdateTaskStepStatus(w.videoID, stepName, "completed"); err != nil {
			w.logger.Errorf("更新任务步骤状态失败: %v", err)
		}

		// 保存执行结果
		result := map[string]interface{}{}
		for k, v := range context {
			if k != "error" { // 排除错误信息
				result[k] = v
			}
		}
		if err := w.taskStepService.UpdateTaskStepResult(w.videoID, stepName, result); err != nil {
			w.logger.Errorf("更新任务步骤结果失败: %v", err)
		}
	} else {
		errorMsg := ""
		if err, exists := context["error"]; exists {
			errorMsg = fmt.Sprintf("%v", err)
		}
		if errorMsg == "" {
			errorMsg = fmt.Sprintf("任务 %s 执行失败", stepName)
			context["error"] = errorMsg
		}

		if err := w.taskStepService.UpdateTaskStepStatus(w.videoID, stepName, "failed", errorMsg); err != nil {
			w.logger.Errorf("更新任务步骤状态失败: %v", err)
		}
	}

	return success
}

func newTaskProgressReporter(videoID, stepName string, taskStepService *services.TaskStepService, logger *zap.SugaredLogger) types.ProgressReporter {
	var mutex sync.Mutex
	lastPercent := -1
	lastMessage := ""
	lastSentAt := time.Time{}

	return func(percent int, message string) {
		if taskStepService == nil {
			return
		}

		percent = clampTaskProgress(percent)
		mutex.Lock()
		defer mutex.Unlock()

		if percent == lastPercent && message == lastMessage && time.Since(lastSentAt) < 2*time.Second {
			return
		}

		if err := taskStepService.UpdateTaskStepProgress(videoID, stepName, percent, message); err != nil {
			if logger != nil {
				logger.Warnf("更新任务步骤进度失败 videoID=%s step=%s progress=%d: %v", videoID, stepName, percent, err)
			}
			return
		}

		lastPercent = percent
		lastMessage = message
		lastSentAt = time.Now()
	}
}

func clampTaskProgress(percent int) int {
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}

func getRetryRunningVideoStatus(stepOrder int) string {
	switch stepOrder {
	case 5:
		return "201"
	case 6:
		return "301"
	default:
		return "002"
	}
}

func getRetrySuccessVideoStatus(stepOrder int) string {
	switch stepOrder {
	case 4:
		return "200"
	case 5:
		return "300"
	case 6:
		return "400"
	default:
		return "002"
	}
}

func getRetryFailedVideoStatus(stepOrder int) string {
	switch stepOrder {
	case 5:
		return "299"
	case 6:
		return "399"
	default:
		return "999"
	}
}

// updateSavedVideoStatus 更新 SavedVideo 的状态
func (h *ChainTaskHandler) updateSavedVideoStatus(id uint, status string) error {
	return h.SavedVideoService.UpdateStatus(id, status)
}
