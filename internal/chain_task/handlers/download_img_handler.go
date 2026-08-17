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

	opt := utils.DownloadOptions{
		SavePath:         t.StateManager.CurrentDir,
		FilenameTemplate: "{quality}",
		Timeout:          10 * time.Second,
		MaxRetries:       3,
		QualityFallback:  true,
		CreateDirs:       true,
		Overwrite:        false,
		ProxyURL:         t.proxyURL(),
	}

	result := utils.DownloadYouTubeThumbnail(t.StateManager.VideoID, "best", opt, "")
	downloadResult, ok := result.(utils.DownloadResult)
	if !ok {
		errMsg := "下载 YouTube 封面失败: 下载器返回了未知结果"
		context["error"] = errMsg
		t.App.Logger.Error("❌ " + errMsg)
		return false
	}

	if !downloadResult.Success {
		errMsg := fmt.Sprintf("下载 YouTube 封面失败: %s", downloadResult.ErrorMessage)
		context["error"] = errMsg
		t.App.Logger.Error("❌ " + errMsg)
		return false
	}

	context["cover_image_path"] = downloadResult.FilePath
	t.App.Logger.Infof("✓ YouTube 封面已下载: %s (%s, %d bytes)", downloadResult.FilePath, downloadResult.Quality, downloadResult.FileSize)

	var cosKeyName string
	if t.Client != nil {
		if uploadedKey, err := t.Client.UploadImageToCOS(downloadResult.FilePath, ""); err != nil {
			t.App.Logger.Warnf("⚠️ 封面上传 COS 失败，将仅使用本地封面投稿: %v", err)
		} else {
			cosKeyName = uploadedKey
		}
	}

	tbVideo := &models.TbVideo{
		Id:      t.StateManager.Id,
		VideoId: t.StateManager.VideoID,
		ImgURL:  cosKeyName,
		Status:  "img",
	}
	if t.StateManager.SaveUrlService != nil {
		if err := t.StateManager.UpdateTBVideo(tbVideo); err != nil {
			t.App.Logger.Warnf("⚠️ 更新封面状态失败: %v", err)
		}
	}
	return true
}

func (t *DownloadImgHandler) proxyURL() string {
	if t.App == nil || t.App.Config == nil || t.App.Config.ProxyConfig == nil {
		return ""
	}
	if !t.App.Config.ProxyConfig.UseProxy {
		return ""
	}
	return t.App.Config.ProxyConfig.ProxyHost
}
