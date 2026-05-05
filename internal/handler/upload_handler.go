package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	bilibili2 "github.com/difyz9/bilibili-go-sdk/bilibili"
	"github.com/difyz9/ytb2bili/internal/bilibiliutil"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/pkg/cos"

	"github.com/gin-gonic/gin"
)

const manualUploadAPITimeout = 2 * time.Minute

type UploadHandler struct {
	BaseHandler
}

func NewUploadHandler(app *core.AppServer) *UploadHandler {
	return &UploadHandler{
		BaseHandler: BaseHandler{App: app},
	}
}

func (h *UploadHandler) newUploadClient(loginInfo *bilibili2.LoginInfo) *bilibili2.UploadClient {
	return bilibili2.NewUploadClient(loginInfo, bilibiliutil.BuildOptions(h.App.Config, manualUploadAPITimeout)...)
}

// RegisterRoutes 注册上传相关路由
func (h *UploadHandler) RegisterRoutes(server *core.AppServer) {
	api := server.Engine.Group("/api/v1")

	upload := api.Group("/upload")
	{
		upload.POST("/video", h.uploadVideo)
		upload.POST("/cover", h.uploadCover)
		upload.POST("/submit", h.submitVideo)
	}
}

// UploadVideoRequest 上传视频请求
type UploadVideoRequest struct {
	LoginInfo *bilibili2.LoginInfo `json:"login_info" binding:"required"`
	VideoPath string               `json:"video_path" binding:"required"`
}

// UploadVideoResponse 上传视频响应
type UploadVideoResponse struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Video   *bilibili2.Video `json:"video,omitempty"`
}

// uploadVideo 上传视频文件（使用 COS 优化，不使用临时文件）
func (h *UploadHandler) uploadVideo(c *gin.Context) {
	// 处理文件上传
	file, err := c.FormFile("video")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "No video file uploaded: " + err.Error(),
		})
		return
	}

	// 获取登录信息
	loginInfoStr := c.PostForm("login_info")
	if loginInfoStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Login info is required",
		})
		return
	}

	var loginInfo bilibili2.LoginInfo
	if err := json.Unmarshal([]byte(loginInfoStr), &loginInfo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid login info: " + err.Error(),
		})
		return
	}

	log.Printf("🚀 Starting COS-optimized video upload: filename=%s, size=%d", file.Filename, file.Size)

	// 1. 先上传到腾讯云 COS
	cosClient, err := cos.NewCosClient(h.App.Config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to initialize COS client: " + err.Error(),
		})
		return
	}

	// 打开上传的文件
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to open uploaded file: " + err.Error(),
		})
		return
	}
	defer src.Close()

	// 上传到 COS
	log.Printf("📤 Uploading to COS: %s", file.Filename)
	cosKey, cosURL, err := cosClient.UploadVideoFromReader(src, file.Filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to upload to COS: " + err.Error(),
		})
		return
	}

	log.Printf("✅ COS upload successful: key=%s, url=%s", cosKey, cosURL)

	// 2. 从 COS URL 直接上传到 Bilibili（不使用临时文件）
	uploadClient := h.newUploadClient(&loginInfo)

	log.Printf("🎯 Uploading to Bilibili from COS URL: %s", cosURL)
	video, err := uploadClient.UploadVideoFromURL(cosURL, file.Filename, file.Size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Upload to Bilibili failed: " + err.Error(),
		})
		return
	}

	log.Printf("🎉 Upload completed successfully: filename=%s, title=%s", video.Filename, video.Title)

	c.JSON(http.StatusOK, UploadVideoResponse{
		Code:    0,
		Message: "Upload successful (via COS optimization)",
		Video:   video,
	})
}

// UploadCoverRequest 上传封面请求
type UploadCoverRequest struct {
	LoginInfo *bilibili2.LoginInfo `json:"login_info" binding:"required"`
	ImagePath string               `json:"image_path" binding:"required"`
}

// UploadCoverResponse 上传封面响应
type UploadCoverResponse struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	CoverURL string `json:"cover_url,omitempty"`
}

// uploadCover 上传封面
func (h *UploadHandler) uploadCover(c *gin.Context) {
	// 处理文件上传
	file, err := c.FormFile("cover")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "No cover file uploaded: " + err.Error(),
		})
		return
	}

	// 获取登录信息
	loginInfoStr := c.PostForm("login_info")
	if loginInfoStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Login info is required",
		})
		return
	}

	var loginInfo bilibili2.LoginInfo
	if err := json.Unmarshal([]byte(loginInfoStr), &loginInfo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid login info: " + err.Error(),
		})
		return
	}

	// 创建临时目录
	tempDir := "./temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to create temp directory: " + err.Error(),
		})
		return
	}

	// 保存上传的文件
	tempPath := filepath.Join(tempDir, file.Filename)
	if err := c.SaveUploadedFile(file, tempPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to save uploaded file: " + err.Error(),
		})
		return
	}

	// 确保在函数结束时删除临时文件
	defer os.Remove(tempPath)

	uploadClient := h.newUploadClient(&loginInfo)

	coverURL, err := uploadClient.UploadCover(tempPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Cover upload failed: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, UploadCoverResponse{
		Code:     0,
		Message:  "Cover upload successful",
		CoverURL: coverURL,
	})
}

// SubmitVideoRequest 提交视频请求
type SubmitVideoRequest struct {
	LoginInfo *bilibili2.LoginInfo `json:"login_info" binding:"required"`
	Studio    *bilibili2.Studio    `json:"studio" binding:"required"`
}

// SubmitVideoResponse 提交视频响应
type SubmitVideoResponse struct {
	Code    int                     `json:"code"`
	Message string                  `json:"message"`
	Result  *bilibili2.ResponseData `json:"result,omitempty"`
}

// submitVideo 提交视频到B站
func (h *UploadHandler) submitVideo(c *gin.Context) {
	var req SubmitVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("Request binding error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid request parameters: " + err.Error(),
		})
		return
	}

	log.Printf("Submit video request: Studio=%+v", req.Studio)

	uploadClient := h.newUploadClient(req.LoginInfo)

	result, err := uploadClient.SubmitVideo(req.Studio)
	if err != nil {
		log.Printf("Submit video error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Submit failed: " + err.Error(),
		})
		return
	}

	log.Printf("Submit video result: Code=%d, Message=%s, Data=%+v", result.Code, result.Message, result.Data)

	if result.Code != 0 {
		log.Printf("Submit failed with code %d: %s", result.Code, result.Message)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    result.Code,
			"message": "Submit failed: " + result.Message,
		})
		return
	}

	c.JSON(http.StatusOK, SubmitVideoResponse{
		Code:    0,
		Message: "Submit successful",
		Result:  result,
	})
}
