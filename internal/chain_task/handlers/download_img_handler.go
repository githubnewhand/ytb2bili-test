package handlers

import (
	"github.com/difyz9/ytb2bili/internal/chain_task/base"
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/models"
	"github.com/difyz9/ytb2bili/pkg/cos"
	"github.com/difyz9/ytb2bili/pkg/utils"
	"fmt"
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
	}

	//utils.QualityMax,
	qualities := []utils.ImageQuality{utils.QualityMax, utils.QualityStandard}
	results := utils.DownloadYouTubeThumbnail(t.StateManager.VideoID, qualities, opt, "").(map[string]utils.DownloadResult)

	var maxQualityCoverPath string

	for k, v := range results {
		if v.Success {
			fmt.Printf("下载成功: %s - %s (%d bytes)\n", k, v.FilePath, v.FileSize)
			cosKeyName, _ := t.Client.UploadImageToCOS(v.FilePath, "")

			// 如果是最高质量的封面，保存到context中供后续上传使用
			if k == string(utils.QualityMax) {
				maxQualityCoverPath = v.FilePath
				context["cover_image_path"] = v.FilePath
				t.App.Logger.Infof("✓ 最高质量封面已下载: %s", v.FilePath)
			}

			// 更新数据库记录
			tbVideo := &models.TbVideo{
				Id:      t.StateManager.Id,
				VideoId: t.StateManager.VideoID,
				ImgURL:  cosKeyName,
				Status:  "img",
			}
			err := t.StateManager.UpdateTBVideo(tbVideo)
			if err != nil {

			}
		} else {
			fmt.Printf("下载失败: %s - %s\n", k, v.ErrorMessage)
		}
	}

	// 如果没有下载到最高质量的封面，使用其他质量的封面
	if maxQualityCoverPath == "" {
		for _, v := range results {
			if v.Success {
				context["cover_image_path"] = v.FilePath
				t.App.Logger.Infof("✓ 备用质量封面已设置: %s", v.FilePath)
				break
			}
		}
	}

	return true
}
