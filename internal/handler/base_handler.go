package handler

import (
	"github.com/difyz9/ytb2bili/internal/core"
	"fmt"
	"github.com/gin-gonic/gin"
)

// BaseHandler 基础Handler
type BaseHandler struct {
	App *core.AppServer
}

// GetInt 获取整型参数
func (h *BaseHandler) GetInt(c *gin.Context, key string, defaultValue int) int {
	value := c.Query(key)
	if value == "" {
		return defaultValue
	}
	
	intValue := 0
	if _, err := fmt.Sscanf(value, "%d", &intValue); err != nil {
		return defaultValue
	}
	return intValue
}

// GetString 获取字符串参数
func (h *BaseHandler) GetString(c *gin.Context, key string, defaultValue string) string {
	value := c.Query(key)
	if value == "" {
		return defaultValue
	}
	return value
}
