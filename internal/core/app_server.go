package core

import (
	"github.com/difyz9/ytb2bili/internal/core/types"
	"github.com/difyz9/ytb2bili/pkg/cos"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AppServer 应用服务器
type AppServer struct {
	Config    *types.AppConfig
	Engine    *gin.Engine
	Logger    *zap.SugaredLogger
	DB        *gorm.DB
	CosClient *cos.CosClient // COS客户端

}

// NewServer 创建新的服务器实例
func NewServer(config *types.AppConfig, logger *zap.SugaredLogger) *AppServer {
	// 设置Gin模式
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = io.Discard
	return &AppServer{
		Config: config,
		Engine: gin.Default(),
		Logger: logger,
	}
}

// Init 初始化服务器
func (s *AppServer) Init(db *gorm.DB) {
	s.DB = db

	// 设置中间件
	s.setupMiddleware()

	// 设置静态文件
	s.Engine.Static("/static", "./static")
}

// setupMiddleware 设置中间件
func (s *AppServer) setupMiddleware() {
	// CORS中间件
	s.Engine.Use(func(c *gin.Context) {
		method := c.Request.Method
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE, UPDATE")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Accept, Cache-Control, X-Requested-With")
			c.Header("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers, Content-Type")
			c.Header("Access-Control-Max-Age", "172800")
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if method == http.MethodOptions {
			c.JSON(http.StatusOK, "ok!")
		}

		defer func() {
			if err := recover(); err != nil {
				s.Logger.Infof("Panic info is: %v", err)
			}
		}()

		c.Next()
	})

	// 日志中间件
	s.Engine.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("%s - [%s] \"%s %s %s %d %s \"%s\" %s\"\n",
			param.ClientIP,
			param.TimeStamp.Format(time.RFC1123),
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency,
			param.Request.UserAgent(),
			param.ErrorMessage,
		)
	}))

	// 错误处理中间件
	s.Engine.Use(func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				s.Logger.Errorf("Handler Panic: %v", r)
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "Internal server error",
				})
				c.Abort()
			}
		}()
		c.Next()
	})
}

// Run 启动服务器
func (s *AppServer) Run() error {
	s.Logger.Infof("Starting server on %s", s.Config.Listen)
	s.Logger.Infof("Environment: %s", s.Config.Environment)

	fmt.Println("listening on ---> ", s.Config.Listen)

	return s.Engine.Run(s.Config.Listen)
}

// Shutdown 优雅关闭服务器
func (s *AppServer) Shutdown(ctx context.Context) error {
	s.Logger.Info("Shutting down server...")

	return nil
}
