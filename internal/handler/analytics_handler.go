package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/difyz9/ytb2bili/pkg/analytics"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AnalyticsHandler 包装其他handler，添加分析功能
type AnalyticsHandler struct {
	analyticsClient *analytics.Client
	logger          *zap.SugaredLogger
}

// NewAnalyticsHandler 创建新的分析处理器
func NewAnalyticsHandler(client *analytics.Client, logger *zap.SugaredLogger) *AnalyticsHandler {
	return &AnalyticsHandler{
		analyticsClient: client,
		logger:          logger,
	}
}

// TrackVideoOperation 跟踪视频操作
func (h *AnalyticsHandler) TrackVideoOperation(c *gin.Context, operation string, videoID string, success bool, details map[string]interface{}) {
	if h.analyticsClient == nil {
		return
	}

	userID := h.getUserID(c)
	deviceID := h.getDeviceID(c)

	properties := map[string]interface{}{
		"operation": operation,
		"video_id":  videoID,
		"success":   success,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	// 合并详细信息
	for k, v := range details {
		properties[k] = v
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := h.analyticsClient.TrackUserAction(ctx, userID, deviceID, "video_"+operation, properties)
		if err != nil {
			h.logger.Errorf("Failed to track video operation %s: %v", operation, err)
		}
	}()
}

// TrackVideoUpload 跟踪视频上传
func (h *AnalyticsHandler) TrackVideoUpload(c *gin.Context, videoID string, fileSize int64, duration int, format string) {
	if h.analyticsClient == nil {
		return
	}

	userID := h.getUserID(c)
	deviceID := h.getDeviceID(c)

	videoInfo := map[string]interface{}{
		"video_id":  videoID,
		"file_size": fileSize,
		"duration":  duration,
		"format":    format,
		"upload_time": time.Now().Format(time.RFC3339),
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		videoID, _ := videoInfo["video_id"].(string)
		title, _ := videoInfo["title"].(string)
		size, _ := videoInfo["size_bytes"].(int64)

		err := h.analyticsClient.TrackVideoUpload(ctx, userID, deviceID, videoID, title, size)
		if err != nil {
			h.logger.Errorf("Failed to track video upload: %v", err)
		}
	}()
}

// TrackError 跟踪错误
func (h *AnalyticsHandler) TrackError(c *gin.Context, errorType string, errorMessage string, contextData map[string]interface{}) {
	if h.analyticsClient == nil {
		return
	}

	userID := h.getUserID(c)
	deviceID := h.getDeviceID(c)

	properties := map[string]interface{}{
		"endpoint": c.Request.URL.Path,
		"method":   c.Request.Method,
	}

	// 合并上下文信息
	for k, v := range contextData {
		properties[k] = v
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// 将properties转换为字符串格式的堆栈跟踪
		stackTrace := ""
		if st, exists := properties["stack_trace"]; exists {
			if stStr, ok := st.(string); ok {
				stackTrace = stStr
			}
		}

		err := h.analyticsClient.TrackError(ctx, userID, deviceID, errorType, errorMessage, stackTrace)
		if err != nil {
			h.logger.Errorf("Failed to track error: %v", err)
		}
	}()
}

// getUserID 从请求中获取用户ID
func (h *AnalyticsHandler) getUserID(c *gin.Context) string {
	// 尝试从多个地方获取用户ID
	
	// 1. 从JWT token中获取
	if userID, exists := c.Get("user_id"); exists {
		if uid, ok := userID.(string); ok {
			return uid
		}
	}

	// 2. 从请求头中获取
	if userID := c.GetHeader("X-User-ID"); userID != "" {
		return userID
	}

	// 3. 从查询参数中获取
	if userID := c.Query("user_id"); userID != "" {
		return userID
	}

	// 4. 使用IP地址作为匿名用户标识
	return "anonymous_" + c.ClientIP()
}

// getDeviceID 从请求中获取设备ID
func (h *AnalyticsHandler) getDeviceID(c *gin.Context) string {
	// 尝试从多个地方获取设备ID
	
	// 1. 从请求头中获取
	if deviceID := c.GetHeader("X-Device-ID"); deviceID != "" {
		return deviceID
	}

	// 2. 从User-Agent生成设备指纹
	userAgent := c.GetHeader("User-Agent")
	if userAgent != "" {
		// 简单的设备指纹生成（实际项目中可以使用更复杂的算法）
		return "device_" + hashString(userAgent+c.ClientIP())
	}

	// 3. 使用IP地址作为设备标识
	return "device_" + c.ClientIP()
}

// hashString 简单的字符串hash函数
func hashString(s string) string {
	hash := uint32(0)
	for _, c := range s {
		hash = hash*31 + uint32(c)
	}
	return strconv.FormatUint(uint64(hash), 16)
}