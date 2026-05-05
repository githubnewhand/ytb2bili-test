package analytics

import (
	"context"
	"errors"
	"fmt"
	"time"

	analysis "github.com/difyz9/go-analysis-client"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Middleware 分析中间件
type Middleware struct {
	client *Client
	logger *zap.SugaredLogger
}

// NewMiddleware 创建新的分析中间件
func NewMiddleware(client *Client, logger *zap.SugaredLogger) *Middleware {
	return &Middleware{
		client: client,
		logger: logger,
	}
}

// Handler 中间件处理函数
func (m *Middleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if m.client == nil || !m.client.IsEnabled() {
			c.Next()
			return
		}

		// 记录请求开始时间
		startTime := time.Now()

		// 继续处理请求
		c.Next()

		// 计算请求duration
		duration := time.Since(startTime)

		// 获取用户ID和设备ID（从请求头或上下文中）
		userID := m.getUserID(c)
		deviceID := m.getDeviceID(c)

		// 跟踪API请求
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := m.client.TrackAPIRequest(
				ctx,
				c.Request.URL.Path,
				c.Request.Method,
				userID,
				deviceID,
				c.Writer.Status(),
				duration,
			)
			if err != nil {
				// 使用新的错误处理系统
				var netErr *analysis.NetworkError
				if errors.As(err, &netErr) {
					if netErr.Retryable {
						m.logger.Warnf("Network error tracking API request (retryable): %v", err)
					} else {
						m.logger.Errorf("Network error tracking API request (non-retryable): %v", err)
					}
					return
				}
				
				var clientErr *analysis.ClientError
				if errors.As(err, &clientErr) {
					m.logger.Errorf("Client error tracking API request: %v (context: %+v)", clientErr.Err, clientErr.Context)
					return
				}
				
				m.logger.Errorf("Failed to track API request: %v", err)
			}
		}()
	}
}

// getUserID 从请求中获取用户ID
func (m *Middleware) getUserID(c *gin.Context) string {
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
func (m *Middleware) getDeviceID(c *gin.Context) string {
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
	return fmt.Sprintf("%x", hash)
}

// TrackCustomEvent 在路由处理器中跟踪自定义事件的辅助函数
func (m *Middleware) TrackCustomEvent(c *gin.Context, eventName string, properties map[string]interface{}) {
	if m.client == nil || !m.client.IsEnabled() {
		return
	}

	userID := m.getUserID(c)
	deviceID := m.getDeviceID(c)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := m.client.TrackUserAction(ctx, userID, deviceID, eventName, properties)
		if err != nil {
			// 使用新的错误处理系统
			var netErr *analysis.NetworkError
			if errors.As(err, &netErr) {
				if netErr.Retryable {
					m.logger.Warnf("Network error tracking custom event %s (retryable): %v", eventName, err)
				} else {
					m.logger.Errorf("Network error tracking custom event %s (non-retryable): %v", eventName, err)
				}
				return
			}
			
			var clientErr *analysis.ClientError
			if errors.As(err, &clientErr) {
				m.logger.Errorf("Client error tracking custom event %s: %v (context: %+v)", eventName, clientErr.Err, clientErr.Context)
				return
			}
			
			m.logger.Errorf("Failed to track custom event %s: %v", eventName, err)
		}
	}()
}