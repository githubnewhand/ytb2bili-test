package auth

import (
	goauth "github.com/difyz9/go-auth"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Config 认证配置
type Config struct {
	Apps map[string]string // App ID -> App Secret
}

// Middleware 认证中间件
type Middleware struct {
	config *Config
	logger *zap.SugaredLogger
	auth   *goauth.AuthMiddleware
}

// NewMiddleware 创建认证中间件
func NewMiddleware(config *Config, logger *zap.SugaredLogger) *Middleware {
	if config == nil || len(config.Apps) == 0 {
		logger.Warn("Auth middleware initialized with empty config")
		return &Middleware{
			config: config,
			logger: logger,
		}
	}

	// 创建 go-auth 配置
	authConfig := goauth.NewConfig()
	
	// 遍历所有配置的应用并添加到 go-auth
	for appID, appSecret := range config.Apps {
		app := &goauth.AppConfig{
			AppID:     appID,
			AppSecret: appSecret,
			Enabled:   true,
		}
		authConfig.AddApp(app)
		logger.Infof("Registered auth app: %s", appID)
	}

	// 创建 go-auth 中间件
	authMiddleware := goauth.New(authConfig)

	logger.Infof("Auth middleware created with %d app(s)", len(config.Apps))

	return &Middleware{
		config: config,
		logger: logger,
		auth:   authMiddleware,
	}
}

// Handler 返回 Gin 中间件处理函数
func (m *Middleware) Handler() gin.HandlerFunc {
	if m.auth == nil {
		// 如果没有配置认证，返回一个直接通过的中间件
		return func(c *gin.Context) {
			c.Next()
		}
	}

	return m.auth.Authenticate()
}

// IsEnabled 返回认证是否启用
func (m *Middleware) IsEnabled() bool {
	return m.auth != nil
}
