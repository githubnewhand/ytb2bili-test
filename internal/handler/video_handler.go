package handler

import (
	"encoding/json"
	"fmt"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/services"
	"github.com/difyz9/ytb2bili/pkg/store/model"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type VideoHandler struct {
	BaseHandler
	SavedVideoService *services.SavedVideoService
	TaskStepService   *services.TaskStepService
	UploadScheduler   interface {
		ExecuteManualUpload(videoID, taskType string) error
	}
	AnalyticsHandler *AnalyticsHandler
}

func NewVideoHandler(app *core.AppServer, savedVideoService *services.SavedVideoService, taskStepService *services.TaskStepService) *VideoHandler {
	return &VideoHandler{
		BaseHandler:       BaseHandler{App: app},
		SavedVideoService: savedVideoService,
		TaskStepService:   taskStepService,
		UploadScheduler:   nil, // Will be set later via SetUploadScheduler
	}
}

// SetUploadScheduler 设置上传调度器（避免循环依赖）
func (h *VideoHandler) SetUploadScheduler(scheduler interface {
	ExecuteManualUpload(videoID, taskType string) error
}) {
	h.UploadScheduler = scheduler
}

// RegisterRoutes 注册视频相关路由
func (h *VideoHandler) RegisterRoutes(api *gin.RouterGroup) {
	video := api.Group("/videos")
	{
		video.GET("", h.getVideoList)
		video.GET("/:id", h.getVideoDetail)
		video.POST("/:id/steps/:stepName/retry", h.retryTaskStep)
		video.GET("/:id/files", h.getVideoFiles)
		video.POST("/:id/upload/video", h.manualUploadVideo)
		video.POST("/:id/upload/subtitle", h.manualUploadSubtitle)
	}
}

// VideoListResponse 视频列表响应
type VideoListResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// VideoListData 视频列表数据
type VideoListData struct {
	Videos []VideoInfo `json:"videos"`
	Total  int         `json:"total"`
	Page   int         `json:"page"`
	Limit  int         `json:"limit"`
}

// VideoInfo 视频信息
type VideoInfo struct {
	ID             uint                   `json:"id"`
	VideoID        string                 `json:"video_id"`
	Title          string                 `json:"title"`
	URL            string                 `json:"url"`
	Status         string                 `json:"status"`
	Chapters       string                 `json:"chapters"`
	ChaptersStatus string                 `json:"chapters_status"`
	ChaptersMessage string                `json:"chapters_message"`
	ChaptersExtracted bool                `json:"chapters_extracted"`
	ChaptersCount int                    `json:"chapters_count"`
	GeneratedTitle string                 `json:"generated_title"`
	GeneratedDesc  string                 `json:"generated_desc"`
	GeneratedTags  string                 `json:"generated_tags"`
	BiliBVID       string                 `json:"bili_bvid"`
	BiliAID        int64                  `json:"bili_aid"`
	CreatedAt      string                 `json:"created_at"`
	UpdatedAt      string                 `json:"updated_at"`
	TaskSteps      []TaskStepInfo         `json:"task_steps,omitempty"`
	Progress       map[string]interface{} `json:"progress,omitempty"`
	CoverImage     string                 `json:"cover_image,omitempty"`
	MetaData       map[string]interface{} `json:"meta_data,omitempty"`
}

// TaskStepInfo 任务步骤信息
type TaskStepInfo struct {
	StepName        string                 `json:"step_name"`
	StepOrder       int                    `json:"step_order"`
	Status          string                 `json:"status"`
	StartTime       string                 `json:"start_time"`
	EndTime         string                 `json:"end_time"`
	Duration        int64                  `json:"duration"`
	ProgressPercent int                    `json:"progress_percent"`
	ProgressMessage string                 `json:"progress_message"`
	ErrorMsg        string                 `json:"error_msg"`
	ResultData      map[string]interface{} `json:"result_data,omitempty"`
	CanRetry        bool                   `json:"can_retry"`
}

// getVideoList 获取视频列表
func (h *VideoHandler) getVideoList(c *gin.Context) {
	// 解析分页参数
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "10")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		limit = 10
	}

	// 计算偏移量
	offset := (page - 1) * limit

	// 获取视频列表
	savedVideos, total, err := h.SavedVideoService.GetVideosPaginated(offset, limit)
	if err != nil {
		h.App.Logger.Errorf("获取视频列表失败: %v", err)
		c.JSON(http.StatusInternalServerError, VideoListResponse{
			Code:    500,
			Message: "获取视频列表失败",
		})
		return
	}

	// 转换为响应格式
	var videos []VideoInfo
	for _, sv := range savedVideos {
		progress, progressErr := h.TaskStepService.GetTaskProgress(sv.VideoID)
		if progressErr != nil {
			h.App.Logger.Warnf("获取任务进度失败 videoID=%s: %v", sv.VideoID, progressErr)
		}
		chaptersDetail := buildVideoChaptersDetail(&sv, nil)

		videos = append(videos, VideoInfo{
			ID:                sv.ID,
			VideoID:           sv.VideoID,
			Title:             sv.Title,
			URL:               sv.URL,
			Status:            sv.Status,
			Chapters:          chaptersDetail.Chapters,
			ChaptersStatus:    chaptersDetail.Status,
			ChaptersMessage:   chaptersDetail.Message,
			ChaptersExtracted: chaptersDetail.Extracted,
			ChaptersCount:     chaptersDetail.Count,
			GeneratedTitle:    sv.GeneratedTitle,
			GeneratedDesc:     sv.GeneratedDesc,
			GeneratedTags:     sv.GeneratedTags,
			BiliBVID:          sv.BiliBVID,
			BiliAID:           sv.BiliAID,
			CreatedAt:         sv.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:         sv.UpdatedAt.Format("2006-01-02 15:04:05"),
			Progress:          progress,
		})
	}

	c.JSON(http.StatusOK, VideoListResponse{
		Code:    200,
		Message: "success",
		Data: VideoListData{
			Videos: videos,
			Total:  total,
			Page:   page,
			Limit:  limit,
		},
	})
}

// getVideoDetail 获取视频详情
func (h *VideoHandler) getVideoDetail(c *gin.Context) {
	idStr := c.Param("id")

	// 尝试解析为数字ID，如果失败则当作video_id（字符串）处理
	var savedVideo *model.SavedVideo
	var err error

	if id, parseErr := strconv.ParseUint(idStr, 10, 32); parseErr == nil {
		// 如果可以解析为数字，则按ID查询
		savedVideo, err = h.SavedVideoService.GetByID(uint(id))
	} else {
		// 否则按video_id查询
		savedVideo, err = h.SavedVideoService.GetVideoByVideoID(idStr)
	}

	if err != nil {
		h.App.Logger.Errorf("获取视频详情失败: %v", err)
		c.JSON(http.StatusNotFound, VideoListResponse{
			Code:    404,
			Message: "视频不存在",
		})
		return
	}

	// 获取任务步骤
	taskSteps, err := h.TaskStepService.GetTaskStepsByVideoID(savedVideo.VideoID)
	if err != nil {
		h.App.Logger.Errorf("获取任务步骤失败: %v", err)
	}

	// 转换任务步骤格式
	var taskStepInfos []TaskStepInfo
	for _, step := range taskSteps {
		stepInfo := TaskStepInfo{
			StepName:        step.StepName,
			StepOrder:       step.StepOrder,
			Status:          step.Status,
			Duration:        step.Duration,
			ProgressPercent: step.Progress,
			ProgressMessage: step.ProgressMsg,
			ErrorMsg:        step.ErrorMsg,
			CanRetry:        step.CanRetry,
		}

		if step.StartTime != nil {
			stepInfo.StartTime = step.StartTime.Format("2006-01-02 15:04:05")
		}
		if step.EndTime != nil {
			stepInfo.EndTime = step.EndTime.Format("2006-01-02 15:04:05")
		}
		if strings.TrimSpace(step.ResultData) != "" {
			var resultData map[string]interface{}
			if err := json.Unmarshal([]byte(step.ResultData), &resultData); err != nil {
				h.App.Logger.Warnf("解析任务步骤结果失败 videoID=%s step=%s: %v", savedVideo.VideoID, step.StepName, err)
			} else {
				stepInfo.ResultData = resultData
			}
		}

		taskStepInfos = append(taskStepInfos, stepInfo)
	}

	// 获取任务进度
	progress, err := h.TaskStepService.GetTaskProgress(savedVideo.VideoID)
	if err != nil {
		h.App.Logger.Errorf("获取任务进度失败: %v", err)
	}

	// 获取元数据文件
	metaData := h.getVideoMetaData(savedVideo.VideoID)

	// 获取封面图片
	coverImage := h.getVideoCoverImage(savedVideo.VideoID)
	chaptersDetail := buildVideoChaptersDetail(savedVideo, taskStepInfos)

	videoInfo := VideoInfo{
		ID:                savedVideo.ID,
		VideoID:           savedVideo.VideoID,
		Title:             savedVideo.Title,
		URL:               savedVideo.URL,
		Status:            savedVideo.Status,
		Chapters:          chaptersDetail.Chapters,
		ChaptersStatus:    chaptersDetail.Status,
		ChaptersMessage:   chaptersDetail.Message,
		ChaptersExtracted: chaptersDetail.Extracted,
		ChaptersCount:     chaptersDetail.Count,
		GeneratedTitle:    savedVideo.GeneratedTitle,
		GeneratedDesc:     savedVideo.GeneratedDesc,
		GeneratedTags:     savedVideo.GeneratedTags,
		BiliBVID:          savedVideo.BiliBVID,
		BiliAID:           savedVideo.BiliAID,
		CreatedAt:         savedVideo.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:         savedVideo.UpdatedAt.Format("2006-01-02 15:04:05"),
		TaskSteps:         taskStepInfos,
		Progress:          progress,
		CoverImage:        coverImage,
		MetaData:          metaData,
	}

	c.JSON(http.StatusOK, VideoListResponse{
		Code:    200,
		Message: "success",
		Data:    videoInfo,
	})
}

type videoChaptersDetail struct {
	Chapters  string
	Status    string
	Message   string
	Extracted bool
	Count     int
}

func buildVideoChaptersDetail(savedVideo *model.SavedVideo, taskSteps []TaskStepInfo) videoChaptersDetail {
	chapters := ""
	if savedVideo != nil {
		chapters = strings.TrimSpace(savedVideo.Chapters)
	}

	detail := videoChaptersDetail{
		Chapters:  chapters,
		Count:     countNonEmptyLines(chapters),
		Extracted: chapters != "",
		Status:    "not_extracted",
		Message:   "尚未提取到章节",
	}
	if detail.Extracted {
		detail.Status = "extracted"
		detail.Message = fmt.Sprintf("已保存 %d 条章节", detail.Count)
	}

	if result := latestChaptersExtractionResult(taskSteps); result != nil {
		if status, ok := result["status"].(string); ok && strings.TrimSpace(status) != "" {
			detail.Status = status
		}
		if message, ok := result["message"].(string); ok && strings.TrimSpace(message) != "" {
			detail.Message = message
		}
		if count, ok := intFromJSONValue(result["chapter_count"]); ok && count > detail.Count {
			detail.Count = count
		}
		if hasChapters, ok := result["has_chapters"].(bool); ok && hasChapters {
			detail.Extracted = true
		}
		if success, ok := result["success"].(bool); ok && success && detail.Count > 0 {
			detail.Extracted = true
		}
	}

	if detail.Chapters != "" {
		detail.Count = countNonEmptyLines(detail.Chapters)
		detail.Extracted = true
	}
	if detail.Extracted && detail.Status == "not_extracted" {
		detail.Status = "extracted"
	}
	if detail.Extracted && strings.TrimSpace(detail.Message) == "" {
		detail.Message = fmt.Sprintf("已提取 %d 条章节", detail.Count)
	}

	return detail
}

func latestChaptersExtractionResult(taskSteps []TaskStepInfo) map[string]interface{} {
	for i := len(taskSteps) - 1; i >= 0; i-- {
		if taskSteps[i].ResultData == nil {
			continue
		}
		result, ok := normalizeMapValue(taskSteps[i].ResultData["chapters_result"])
		if ok {
			return result
		}
	}
	return nil
}

func normalizeMapValue(value interface{}) (map[string]interface{}, bool) {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed, true
	case string:
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(typed), &result); err == nil {
			return result, true
		}
	}
	return nil, false
}

func intFromJSONValue(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		number, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(number), true
	}
	return 0, false
}

func countNonEmptyLines(text string) int {
	count := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

// retryTaskStep 重新执行任务步骤
func (h *VideoHandler) retryTaskStep(c *gin.Context) {
	idStr := c.Param("id")
	stepName := c.Param("stepName")

	// 尝试解析为数字ID，如果失败则当作video_id处理
	var savedVideo *model.SavedVideo
	var err error

	if id, parseErr := strconv.ParseUint(idStr, 10, 32); parseErr == nil {
		savedVideo, err = h.SavedVideoService.GetByID(uint(id))
	} else {
		savedVideo, err = h.SavedVideoService.GetVideoByVideoID(idStr)
	}

	if err != nil {
		h.App.Logger.Errorf("获取视频详情失败: %v", err)
		c.JSON(http.StatusNotFound, VideoListResponse{
			Code:    404,
			Message: "视频不存在",
		})
		return
	}

	// 检查步骤是否存在且可重试
	taskStep, err := h.TaskStepService.GetTaskStepByName(savedVideo.VideoID, stepName)
	if err != nil {
		c.JSON(http.StatusNotFound, VideoListResponse{
			Code:    404,
			Message: "任务步骤不存在",
		})
		return
	}

	if !taskStep.CanRetry {
		c.JSON(http.StatusBadRequest, VideoListResponse{
			Code:    400,
			Message: "此任务步骤不支持重试",
		})
		return
	}

	if taskStep.Status == model.TaskStepStatusPending || taskStep.Status == model.TaskStepStatusRunning {
		c.JSON(http.StatusBadRequest, VideoListResponse{
			Code:    400,
			Message: "任务步骤正在等待或执行中，无需重试",
		})
		return
	}

	if taskStep.Status != model.TaskStepStatusFailed && taskStep.Status != model.TaskStepStatusSkipped {
		c.JSON(http.StatusBadRequest, VideoListResponse{
			Code:    400,
			Message: "只有失败或跳过的任务步骤可以重试",
		})
		return
	}

	// 重新执行任务步骤
	h.App.Logger.Infof("🔄 用户请求重试任务步骤: %s - %s", savedVideo.VideoID, stepName)

	// 重置任务步骤状态为待执行
	err = h.TaskStepService.ResetTaskStep(savedVideo.VideoID, stepName)
	if err != nil {
		h.App.Logger.Errorf("更新任务步骤状态失败: %v", err)
		c.JSON(http.StatusInternalServerError, VideoListResponse{
			Code:    500,
			Message: "更新任务状态失败",
		})
		return
	}

	if taskStep.StepOrder > 0 && taskStep.StepOrder < 4 {
		if err := h.TaskStepService.ResetDownstreamSteps(savedVideo.VideoID, taskStep.StepOrder, 4); err != nil {
			h.App.Logger.Warnf("reset downstream steps failed (videoID=%s, step=%s): %v", savedVideo.VideoID, stepName, err)
		}
	}

	// 重试排队后同步主任务状态，避免界面继续显示旧失败态
	if err := h.SavedVideoService.UpdateStatus(savedVideo.ID, getRetryPendingVideoStatus(taskStep.StepOrder)); err != nil {
		h.App.Logger.Warnf("update retry-pending status failed (videoID=%s, step=%s): %v", savedVideo.VideoID, stepName, err)
	}

	// =================================================================
	// === 新增联动逻辑: 重试"生成字幕"时，自动重置前置任务 ===
	// =================================================================
	if stepName == "生成字幕" {
		h.App.Logger.Infof("🔗 检测到[生成字幕]重试，正在联动重置前置任务...")

		// 1. 强制重置 "分离音频" (确保有最新的音频文件)
		if err := h.TaskStepService.UpdateTaskStepStatus(savedVideo.VideoID, "分离音频", "pending"); err == nil {
			h.App.Logger.Infof("   -> 已联动重置 [分离音频]")
		}

		// 2. 强制重置转录任务 (确保生成字幕数据)
		// 尝试重置 "Whisper转录" (如果存在)
		if err := h.TaskStepService.UpdateTaskStepStatus(savedVideo.VideoID, "Whisper转录", "pending"); err == nil {
			h.App.Logger.Infof("   -> 已联动重置 [Whisper转录]")
		}
		// 尝试重置 "B站必剪转录" (如果存在 - 兼容旧任务)
		if err := h.TaskStepService.UpdateTaskStepStatus(savedVideo.VideoID, "B站必剪转录", "pending"); err == nil {
			h.App.Logger.Infof("   -> 已联动重置 [B站必剪转录]")
		}
	}
	// =================================================================

	h.App.Logger.Infof("✅ 任务步骤 %s 已重置为待执行状态，等待调度器处理", stepName)

	c.JSON(http.StatusOK, VideoListResponse{
		Code:    200,
		Message: fmt.Sprintf("任务步骤 %s 已加入重新执行队列", stepName),
		Data: gin.H{
			"video_id":  savedVideo.VideoID,
			"step_name": stepName,
			"status":    "pending",
			"message":   "任务已重置，将在下次调度时重新执行",
		},
	})
}

func getRetryPendingVideoStatus(stepOrder int) string {
	switch stepOrder {
	case 5:
		return "200"
	case 6:
		return "300"
	default:
		return "002"
	}
}

// getVideoFiles 获取视频相关文件列表
func (h *VideoHandler) getVideoFiles(c *gin.Context) {
	idStr := c.Param("id")

	// 尝试解析为数字ID，如果失败则当作video_id处理
	var savedVideo *model.SavedVideo
	var err error

	if id, parseErr := strconv.ParseUint(idStr, 10, 32); parseErr == nil {
		savedVideo, err = h.SavedVideoService.GetByID(uint(id))
	} else {
		savedVideo, err = h.SavedVideoService.GetVideoByVideoID(idStr)
	}

	if err != nil {
		h.App.Logger.Errorf("获取视频详情失败: %v", err)
		c.JSON(http.StatusNotFound, VideoListResponse{
			Code:    404,
			Message: "视频不存在",
		})
		return
	}

	// 获取视频文件目录
	videoDir := h.getVideoDirectory(savedVideo.VideoID)
	files := h.listVideoFiles(videoDir)

	c.JSON(http.StatusOK, VideoListResponse{
		Code:    200,
		Message: "success",
		Data: gin.H{
			"video_id":  savedVideo.VideoID,
			"directory": videoDir,
			"files":     files,
		},
	})
}

// getVideoMetaData 获取视频元数据
func (h *VideoHandler) getVideoMetaData(videoID string) map[string]interface{} {
	videoDir := h.getVideoDirectory(videoID)
	metaPath := filepath.Join(videoDir, "meta.json")

	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(metaPath)
	if err != nil {
		h.App.Logger.Errorf("读取meta.json失败: %v", err)
		return nil
	}

	var metaData map[string]interface{}
	if err := json.Unmarshal(data, &metaData); err != nil {
		h.App.Logger.Errorf("解析meta.json失败: %v", err)
		return nil
	}

	return metaData
}

// getVideoCoverImage 获取视频封面图片路径
func (h *VideoHandler) getVideoCoverImage(videoID string) string {
	videoDir := h.getVideoDirectory(videoID)
	coverExtensions := []string{".jpg", ".jpeg", ".png", ".webp"}

	for _, ext := range coverExtensions {
		coverPath := filepath.Join(videoDir, "cover"+ext)
		if _, err := os.Stat(coverPath); err == nil {
			// 返回相对于静态文件服务器的路径
			return fmt.Sprintf("/static/videos/%s/cover%s", videoID, ext)
		}
	}

	return ""
}

// getVideoDirectory 获取视频文件目录
func (h *VideoHandler) getVideoDirectory(videoID string) string {
	// 根据配置获取文件上传目录
	baseDir := h.App.Config.FileUpDir

	// 按日期组织的目录结构：/file_upload/media/2025-10-13/videoID/
	// 这里简化处理，实际需要根据创建时间确定日期
	return filepath.Join(baseDir, "media", "*", videoID)
}

// listVideoFiles 列出视频目录中的所有文件
func (h *VideoHandler) listVideoFiles(dirPattern string) []map[string]interface{} {
	var files []map[string]interface{}

	// 使用glob匹配目录
	matches, err := filepath.Glob(dirPattern)
	if err != nil || len(matches) == 0 {
		return files
	}

	dir := matches[0] // 取第一个匹配的目录
	entries, err := os.ReadDir(dir)
	if err != nil {
		h.App.Logger.Errorf("读取目录失败: %v", err)
		return files
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		fileType := h.getFileType(entry.Name())
		files = append(files, map[string]interface{}{
			"name":     entry.Name(),
			"size":     info.Size(),
			"type":     fileType,
			"modified": info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	return files
}

// getFileType 根据文件扩展名判断文件类型
func (h *VideoHandler) getFileType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".mp4", ".flv", ".mkv", ".webm", ".avi", ".mov":
		return "video"
	case ".srt", ".vtt":
		return "subtitle"
	case ".jpg", ".jpeg", ".png", ".webp":
		return "image"
	case ".json":
		return "metadata"
	case ".mp3", ".wav", ".m4a":
		return "audio"
	default:
		return "other"
	}
}

// manualUploadVideo 手动触发视频上传
func (h *VideoHandler) manualUploadVideo(c *gin.Context) {
	idStr := c.Param("id")

	// 尝试解析为数字ID，如果失败则当作video_id处理
	var savedVideo *model.SavedVideo
	var err error

	if id, parseErr := strconv.ParseUint(idStr, 10, 32); parseErr == nil {
		savedVideo, err = h.SavedVideoService.GetByID(uint(id))
	} else {
		savedVideo, err = h.SavedVideoService.GetVideoByVideoID(idStr)
	}

	if err != nil {
		h.App.Logger.Errorf("获取视频详情失败: %v", err)
		c.JSON(http.StatusNotFound, VideoListResponse{
			Code:    404,
			Message: "视频不存在",
		})
		return
	}

	// 检查视频状态是否允许上传
	if savedVideo.Status != "200" && savedVideo.Status != "299" {
		c.JSON(http.StatusBadRequest, VideoListResponse{
			Code:    400,
			Message: fmt.Sprintf("当前状态 %s 不允许上传视频，只有状态为 200(准备就绪) 或 299(上传失败) 的视频才能上传", savedVideo.Status),
		})
		return
	}

	// 检查上传调度器是否已设置
	if h.UploadScheduler == nil {
		c.JSON(http.StatusInternalServerError, VideoListResponse{
			Code:    500,
			Message: "上传调度器未初始化",
		})
		return
	}

	h.App.Logger.Infof("🚀 用户手动触发视频上传: %s (%s)", savedVideo.VideoID, savedVideo.Title)

	// 更新状态为上传中
	if err := h.SavedVideoService.UpdateStatus(savedVideo.ID, "201"); err != nil {
		h.App.Logger.Errorf("更新视频状态失败: %v", err)
		c.JSON(http.StatusInternalServerError, VideoListResponse{
			Code:    500,
			Message: "更新视频状态失败",
		})
		return
	}

	// 异步执行上传任务
	go func() {
		if err := h.UploadScheduler.ExecuteManualUpload(savedVideo.VideoID, "video"); err != nil {
			h.App.Logger.Errorf("手动上传视频失败: %v", err)
			// 上传失败，更新状态为 299
			h.SavedVideoService.UpdateStatus(savedVideo.ID, "299")
		} else {
			h.App.Logger.Infof("✅ 手动上传视频成功: %s", savedVideo.VideoID)
			// 上传成功，更新状态为 300
			h.SavedVideoService.UpdateStatus(savedVideo.ID, "300")
		}
	}()

	c.JSON(http.StatusOK, VideoListResponse{
		Code:    200,
		Message: "视频上传任务已启动",
		Data: gin.H{
			"video_id": savedVideo.VideoID,
			"status":   "201",
			"message":  "视频正在后台上传中，请稍后刷新查看结果",
		},
	})
}

// manualUploadSubtitle 手动触发字幕上传
func (h *VideoHandler) manualUploadSubtitle(c *gin.Context) {
	idStr := c.Param("id")

	// 尝试解析为数字ID，如果失败则当作video_id处理
	var savedVideo *model.SavedVideo
	var err error

	if id, parseErr := strconv.ParseUint(idStr, 10, 32); parseErr == nil {
		savedVideo, err = h.SavedVideoService.GetByID(uint(id))
	} else {
		savedVideo, err = h.SavedVideoService.GetVideoByVideoID(idStr)
	}

	if err != nil {
		h.App.Logger.Errorf("获取视频详情失败: %v", err)
		c.JSON(http.StatusNotFound, VideoListResponse{
			Code:    404,
			Message: "视频不存在",
		})
		return
	}

	// 确保任务步骤完整（兼容老数据缺失“上传字幕到Bilibili”步骤）
	if err := h.TaskStepService.InitTaskSteps(savedVideo.VideoID); err != nil {
		h.App.Logger.Warnf("初始化任务步骤失败（忽略并继续）: %v", err)
	}

	// 检查视频状态是否允许上传字幕
	if savedVideo.Status != "300" && savedVideo.Status != "399" {
		c.JSON(http.StatusBadRequest, VideoListResponse{
			Code:    400,
			Message: fmt.Sprintf("当前状态 %s 不允许上传字幕，只有状态为 300(视频已上传) 或 399(字幕上传失败) 的视频才能上传字幕", savedVideo.Status),
		})
		return
	}

	// 检查是否已有BVID
	if savedVideo.BiliBVID == "" {
		c.JSON(http.StatusBadRequest, VideoListResponse{
			Code:    400,
			Message: "视频尚未上传到Bilibili，无法上传字幕",
		})
		return
	}

	// 检查上传调度器是否已设置
	if h.UploadScheduler == nil {
		c.JSON(http.StatusInternalServerError, VideoListResponse{
			Code:    500,
			Message: "上传调度器未初始化",
		})
		return
	}

	h.App.Logger.Infof("🚀 用户手动触发字幕上传: %s (%s)", savedVideo.VideoID, savedVideo.Title)

	// 更新状态为上传字幕中
	if err := h.SavedVideoService.UpdateStatus(savedVideo.ID, "301"); err != nil {
		h.App.Logger.Errorf("更新视频状态失败: %v", err)
		c.JSON(http.StatusInternalServerError, VideoListResponse{
			Code:    500,
			Message: "更新视频状态失败",
		})
		return
	}

	// 异步执行上传字幕任务
	go func() {
		if err := h.UploadScheduler.ExecuteManualUpload(savedVideo.VideoID, "subtitle"); err != nil {
			h.App.Logger.Errorf("手动上传字幕失败: %v", err)
			// 上传失败，更新状态为 399
			h.SavedVideoService.UpdateStatus(savedVideo.ID, "399")
		} else {
			h.App.Logger.Infof("✅ 手动上传字幕成功: %s", savedVideo.VideoID)
			// 上传成功，更新状态为 400
			h.SavedVideoService.UpdateStatus(savedVideo.ID, "400")
		}
	}()

	c.JSON(http.StatusOK, VideoListResponse{
		Code:    200,
		Message: "字幕上传任务已启动",
		Data: gin.H{
			"video_id": savedVideo.VideoID,
			"status":   "301",
			"message":  "字幕正在后台上传中，请稍后刷新查看结果",
		},
	})
}
