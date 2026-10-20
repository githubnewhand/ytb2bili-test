package handlers

import (
	"fmt"
	"github.com/difyz9/ytb2bili/internal/chain_task/base"
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/models"
	"github.com/difyz9/ytb2bili/pkg/cos"
	"github.com/difyz9/ytb2bili/pkg/utils"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"time"
)

type DownloadImgHandler struct {
	base.BaseTask
	App *core.AppServer
	DB  *gorm.DB
}

func NewDownloadImgHandler(name string, app *core.AppServer, stateManager *manager.StateManager, client *cos.CosClient) *DownloadImgHandler {
	return &DownloadImgHandler{
		BaseTask: base.BaseTask{
			Name:         name,
			StateManager: stateManager,
			Client:       client,
		},
		App: app,
	}

}

func (t *DownloadImgHandler) Execute(context map[string]interface{}) bool {
	coverPath, err := ensureLocalVideoCover(t.App, t.StateManager, context)
	if err != nil {
		context["error"] = err.Error()
		t.App.Logger.Errorf("封面准备失败: %v", err)
		return false
	}

	if t.Client == nil {
		return true
	}
	cosKeyName, err := t.Client.UploadImageToCOS(coverPath, "")
	if err != nil {
		t.App.Logger.Warnf("封面上传到 COS 失败，本地封面仍可用: %v", err)
		return true
	}
	tbVideo := &models.TbVideo{Id: t.StateManager.Id, VideoId: t.StateManager.VideoID, ImgURL: cosKeyName, Status: "img"}
	if err := t.StateManager.UpdateTBVideo(tbVideo); err != nil {
		t.App.Logger.Warnf("保存封面地址失败，本地封面仍可用: %v", err)
	}
	return true
}

func ensureLocalVideoCover(app *core.AppServer, stateManager *manager.StateManager, context map[string]interface{}) (string, error) {
	if coverPath := findLocalVideoCover(stateManager.CurrentDir, stateManager.VideoID); coverPath != "" {
		context["cover_image_path"] = coverPath
		return coverPath, nil
	}

	result, ok := utils.DownloadYouTubeThumbnail(stateManager.VideoID, "best", utils.DownloadOptions{
		SavePath:         stateManager.CurrentDir,
		FilenameTemplate: "{quality}",
		Timeout:          15 * time.Second,
		MaxRetries:       3,
		QualityFallback:  true,
		CreateDirs:       true,
		Overwrite:        false,
		ProxyURL:         thumbnailProxyURL(app),
	}, "cover").(utils.DownloadResult)
	if !ok || !result.Success {
		errorMessage := "未知错误"
		if ok && result.ErrorMessage != "" {
			errorMessage = result.ErrorMessage
		}
		return "", fmt.Errorf("无法下载视频封面: %s", errorMessage)
	}
	if coverPath := findLocalVideoCover(stateManager.CurrentDir, stateManager.VideoID); coverPath != "" {
		context["cover_image_path"] = coverPath
		app.Logger.Infof("视频封面已就绪（%s）: %s", result.Quality, coverPath)
		return coverPath, nil
	}
	return "", fmt.Errorf("封面下载完成但未找到有效图片")
}

func findLocalVideoCover(directory, videoID string) string {
	candidates := []string{
		"cover.jpg", "cover.jpeg", "cover.png", "cover.webp",
		"maxresdefault.jpg", "sddefault.jpg", "hqdefault.jpg", "mqdefault.jpg", "default.jpg",
		videoID + ".jpg", videoID + ".jpeg", videoID + ".png", videoID + ".webp",
	}
	for _, filename := range candidates {
		path := filepath.Join(directory, filename)
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Size() > 0 {
			return path
		}
	}
	return ""
}

func thumbnailProxyURL(app *core.AppServer) string {
	if app == nil || app.Config == nil || app.Config.ProxyConfig == nil || !app.Config.ProxyConfig.UseProxy {
		return ""
	}
	return app.Config.ProxyConfig.ProxyHost
}
