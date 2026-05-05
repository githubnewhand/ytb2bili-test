package handler

import (
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/bilibili-go-sdk/bilibili"
	"net/http"

	"github.com/gin-gonic/gin"
)

type CategoryHandler struct {
	BaseHandler
}

func NewCategoryHandler(app *core.AppServer) *CategoryHandler {
	return &CategoryHandler{
		BaseHandler: BaseHandler{App: app},
	}
}

// RegisterRoutes 注册分类相关路由
func (h *CategoryHandler) RegisterRoutes(server *core.AppServer) {
	api := server.Engine.Group("/api/v1")

	category := api.Group("/category")
	{
		category.GET("/list", h.getCategoryList)
	}
}

// getCategoryList 获取分区列表
func (h *CategoryHandler) getCategoryList(c *gin.Context) {
	// 从请求头中获取用户的 Cookie
	cookies := c.GetHeader("Cookie")
	if cookies == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Cookie header is required",
		})
		return
	}

	// 创建B站客户端
	client := bilibili.NewClient()

	// 获取分区列表
	archiveData, err := client.GetArchivePre(cookies)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to get category list: " + err.Error(),
		})
		return
	}

	// 返回分区列表
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"typelist": archiveData.TypeList,
		},
	})
}
