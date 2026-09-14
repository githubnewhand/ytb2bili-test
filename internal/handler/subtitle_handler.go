package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/difyz9/ytb2bili/internal/core"
	authpkg "github.com/difyz9/ytb2bili/pkg/auth"
	"github.com/difyz9/ytb2bili/pkg/store/model"
	"github.com/difyz9/ytb2bili/pkg/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SubtitleHandler struct {
	BaseHandler
}

func NewSubtitleHandler(app *core.AppServer) *SubtitleHandler {

	return &SubtitleHandler{
		BaseHandler: BaseHandler{App: app},
	}
}

// SaveVideoRequest 保存视频请求
type SaveVideoRequest struct {
	URL           string                     `json:"url" binding:"required"`
	Title         string                     `json:"title"`
	Description   string                     `json:"description"`
	OperationType string                     `json:"operationType"`
	Subtitles     []model.SavedVideoSubtitle `json:"subtitles"`
	PlaylistID    string                     `json:"playlistId"`
	Timestamp     string                     `json:"timestamp"`
	SavedAt       string                     `json:"savedAt"`
}

func (h *SubtitleHandler) saveVideoSubtitles(c *gin.Context) {
	var req SaveVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request parameters: " + err.Error(),
		})
		return
	}

	fmt.Println("Received saveVideoSubtitles request for URL:", req.URL)
	// 从 URL 中提取 videoId
	videoID := utils.ExtractVideoID(req.URL)
	if videoID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid video URL: cannot extract video ID",
		})
		return
	}
	fmt.Println("Extracted videoId:", videoID)

	if decryptedCookies, ok := c.Get("decryptedCookies"); ok {
		if cookieString, ok := decryptedCookies.(string); ok && strings.TrimSpace(cookieString) != "" {
			if err := h.saveVideoCookies(videoID, cookieString); err != nil {
				h.App.Logger.Warnf("save video cookies failed (%s): %v", videoID, err)
			} else {
				h.App.Logger.Infof("saved video cookies for %s", videoID)
			}
		}
	}

	// 将字幕数组转换为JSON字符串
	subtitlesJSON, err := json.Marshal(req.Subtitles)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to marshal subtitles: " + err.Error(),
		})
		return
	}

	// 检查字幕数据大小
	subtitlesJSONStr := string(subtitlesJSON)
	fmt.Printf("字幕数据长度: %d 字符\n", len(subtitlesJSONStr))
	fmt.Printf("字幕条目数量: %d\n", len(req.Subtitles))

	// 如果数据太大，截断前100个字符用于调试
	if len(subtitlesJSONStr) > 100 {
		fmt.Printf("字幕数据前100字符: %s...\n", subtitlesJSONStr[:100])
	} else {
		fmt.Printf("字幕数据: %s\n", subtitlesJSONStr)
	}

	// 检查是否已存在相同的 videoId
	var existingVideo model.SavedVideo
	err = h.App.DB.Where("video_id = ?", videoID).First(&existingVideo).Error

	var savedVideo *model.SavedVideo
	isExisting := false

	if err == nil {
		// 找到了记录，更新字段
		isExisting = true
		existingVideo.URL = req.URL
		existingVideo.Title = req.Title
		existingVideo.Description = req.Description
		existingVideo.OperationType = req.OperationType
		existingVideo.Subtitles = subtitlesJSONStr
		existingVideo.PlaylistID = req.PlaylistID
		existingVideo.Timestamp = req.Timestamp
		existingVideo.SavedAt = req.SavedAt

		// 更新到数据库
		if err := h.App.DB.Save(&existingVideo).Error; err != nil {
			fmt.Printf("更新视频失败，字幕数据长度: %d\n", len(subtitlesJSONStr))
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to update video: " + err.Error(),
			})
			return
		}
		savedVideo = &existingVideo
	} else if err == gorm.ErrRecordNotFound {
		// 记录不存在，创建新记录
		savedVideo = &model.SavedVideo{
			VideoID:       videoID,
			URL:           req.URL,
			Title:         req.Title,
			Status:        "001",
			Description:   req.Description,
			OperationType: req.OperationType,
			Subtitles:     subtitlesJSONStr,
			PlaylistID:    req.PlaylistID,
			Timestamp:     req.Timestamp,
			SavedAt:       req.SavedAt,
		}

		// 保存到数据库
		if err := h.App.DB.Create(savedVideo).Error; err != nil {
			fmt.Printf("创建视频失败，字幕数据长度: %d\n", len(subtitlesJSONStr))
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to save video: " + err.Error(),
			})
			return
		}
	} else {
		// 数据库查询出错
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Database error: " + err.Error(),
		})
		return
	}

	// 计算字幕数量
	subtitleCount := len(req.Subtitles)

	message := "Video saved successfully"
	if isExisting {
		message = "Video updated successfully"
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": message,
		"data": gin.H{
			"id":            savedVideo.ID,
			"title":         savedVideo.Title,
			"operationType": savedVideo.OperationType,
			"subtitleCount": subtitleCount,
			"isExisting":    isExisting,
		},
	})
}

// RegisterRoutes ????????????
func (h *SubtitleHandler) RegisterRoutes(server *core.AppServer) {
	api := server.Engine.Group("/api/v1")
	decryptKey := strings.TrimSpace(server.Config.APIAuth.CookiesDecryptKey)
	if decryptKey != "" {
		api.POST("/submit", authpkg.DecryptCookies(decryptKey), h.saveVideoSubtitles)
		return
	}

	api.POST("/submit", h.saveVideoSubtitles)
}

func (h *SubtitleHandler) saveVideoCookies(videoID, cookieString string) error {
	cookiesDir := filepath.Join(h.App.Config.DataPath, "cookies")
	if err := os.MkdirAll(cookiesDir, 0755); err != nil {
		return err
	}

	filename := fmt.Sprintf("cookies_%s_%d.txt", videoID, time.Now().Unix())
	cookiePath := filepath.Join(cookiesDir, filename)
	return os.WriteFile(cookiePath, []byte(strings.TrimSpace(cookieString)), 0600)
}
