package analytics

import (
	"context"
	"fmt"
	"time"

	analysis "github.com/difyz9/go-analysis-client"
	"go.uber.org/zap"
)

// Client 分析客户端封装
type Client struct {
	client *analysis.Client
	logger *zap.SugaredLogger
}

// Config 分析客户端配置
type Config struct {
	ServerURL     string `toml:"server_url" json:"server_url"`
	APIKey        string `toml:"api_key" json:"api_key"`
	ProductID     string `toml:"product_id" json:"product_id"`
	Debug         bool   `toml:"debug" json:"debug"`
	EncryptionKey string `toml:"encryption_key" json:"encryption_key"` // AES加密密钥（可选）
}

// NewClient 创建新的分析客户端
func NewClient(config *Config, logger *zap.SugaredLogger) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("analytics config is required")
	}

	if config.ServerURL == "" {
		return nil, fmt.Errorf("analytics server URL is required")
	}

	if config.ProductID == "" {
		return nil, fmt.Errorf("analytics product ID is required")
	}

	// 创建分析客户端选项
	var opts []analysis.ClientOption

	// 启用调试模式
	if config.Debug {
		opts = append(opts, analysis.WithDebug(true))
	}

	// 配置加密（如果提供了密钥）
	if config.EncryptionKey != "" {
		opts = append(opts, analysis.WithEncryption(config.EncryptionKey))
		logger.Infof("Analytics encryption enabled with key length: %d", len(config.EncryptionKey))
	}

	// 创建分析客户端（使用新的简化 API）
	client := analysis.NewClient(config.ServerURL, config.ProductID, opts...)

	client.ReportInstall()

	analyticsClient := &Client{
		client: client,
		logger: logger,
	}

	logger.Info("Analytics client initialized successfully")
	return analyticsClient, nil
}

// TrackEvent 跟踪事件
func (c *Client) TrackEvent(ctx context.Context, eventName string, properties map[string]interface{}) error {
	if c.client == nil {
		c.logger.Warn("Analytics client not initialized, skipping event tracking")
		return nil
	}

	// 记录日志
	c.logger.Debugf("Tracking event: %s", eventName)

	// 发送事件（使用新的 Track API）
	c.client.Track(eventName, properties)

	return nil
}

// TrackUserAction 跟踪用户操作
func (c *Client) TrackUserAction(ctx context.Context, userID, deviceID, action string, properties map[string]interface{}) error {
	if properties == nil {
		properties = make(map[string]interface{})
	}

	properties["user_id"] = userID
	properties["device_id"] = deviceID
	properties["timestamp"] = time.Now().Unix()

	return c.TrackEvent(ctx, action, properties)
}

// TrackAPIRequest 跟踪API请求
func (c *Client) TrackAPIRequest(ctx context.Context, endpoint, method, userID, deviceID string, statusCode int, duration time.Duration) error {
	properties := map[string]interface{}{
		"endpoint":     endpoint,
		"method":       method,
		"status_code":  statusCode,
		"duration_ms":  duration.Milliseconds(),
		"request_time": time.Now().Format(time.RFC3339),
		"user_id":      userID,
		"device_id":    deviceID,
	}

	return c.TrackEvent(ctx, "api_request", properties)
}

// TrackVideoUpload 跟踪视频上传事件
func (c *Client) TrackVideoUpload(ctx context.Context, userID, deviceID, videoID, title string, size int64) error {
	properties := map[string]interface{}{
		"video_id":   videoID,
		"title":      title,
		"size_bytes": size,
		"size_mb":    float64(size) / 1024.0 / 1024.0,
		"user_id":    userID,
		"device_id":  deviceID,
		"timestamp":  time.Now().Unix(),
	}

	return c.TrackEvent(ctx, "video_upload", properties)
}

// TrackError 跟踪错误事件
func (c *Client) TrackError(ctx context.Context, userID, deviceID, errorType, errorMessage, stackTrace string) error {
	properties := map[string]interface{}{
		"error_type":    errorType,
		"error_message": errorMessage,
		"stack_trace":   stackTrace,
		"user_id":       userID,
		"device_id":     deviceID,
		"timestamp":     time.Now().Unix(),
	}

	return c.TrackEvent(ctx, "error", properties)
}

// Flush 强制刷新缓冲区
func (c *Client) Flush(ctx context.Context) error {
	if c.client == nil {
		return nil
	}

	c.logger.Debug("Flushing analytics events")

	// Flush 方法是同步的，会等待所有事件发送完成
	c.client.Flush()

	return nil
}

// Close 关闭客户端
func (c *Client) Close() error {
	if c.client == nil {
		return nil
	}

	c.logger.Info("Closing analytics client")
	return c.client.Close()
}

// IsEnabled 检查分析是否启用
func (c *Client) IsEnabled() bool {
	return c.client != nil
}
