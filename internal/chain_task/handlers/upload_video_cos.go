package handlers

import (
	"fmt"
	"gorm.io/gorm"
	"github.com/difyz9/ytb2bili/internal/chain_task/base"
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/models"
	"github.com/difyz9/ytb2bili/pkg/cos"
	"github.com/difyz9/ytb2bili/pkg/utils"
)

//从tb_upload表中提取视频转码并上传到腾讯cos

type UploadVideo2CosHandler struct {
	base.BaseTask
	App *core.AppServer
	DB  *gorm.DB
}

func NewUploadVideo2CosHandler(name string, app *core.AppServer, stateManager *manager.StateManager, client *cos.CosClient) *UploadVideo2CosHandler {
	return &UploadVideo2CosHandler{
		BaseTask: base.BaseTask{
			Name:         name,
			StateManager: stateManager,
			Client:       client,
		},
		App: app,
	}
}

func (t *UploadVideo2CosHandler) ProcessThumbnail() {
	err := utils.ExtractThumbnail(t.StateManager.InputVideoPath, t.StateManager.ImageCover)
	if err != nil {
		fmt.Println("提取视频封面失败")
		//return false
	}
	fmt.Println("提取视频封面成功")
	fmt.Println("开始上传图片到cos")
	if imgKey, err := t.Client.UploadImageToCOS(t.StateManager.ImageCover, ""); err != nil {
		fmt.Println("上传图片到cos失败")
		tbVideo := &models.TbVideo{
			Id:      t.StateManager.Id,
			VideoId: t.StateManager.VideoID,
			ImgURL:  imgKey,
			Status:  "img",
		}
		err = t.StateManager.UpdateTBVideo(tbVideo)
		if err != nil {

		}
	}
}

func (t *UploadVideo2CosHandler) Execute(context map[string]interface{}) bool {

	fmt.Println("视频转码并上传腾讯cos")
	t.ProcessThumbnail()

	fmt.Println(t.StateManager.InputVideoPath)
	newKeyName, err := t.Client.UploadVideoToCOS(t.StateManager.InputVideoPath, "")
	if err != nil {
		fmt.Println("上传视频到cos失败")
		return false
	}

	tbVideo := &models.TbVideo{
		Id:      t.StateManager.Id,
		VcosKey: newKeyName,
		VideoId: t.StateManager.VideoID,
		Status:  "uideo",
	}
	err = t.StateManager.UpdateTBVideo(tbVideo)
	if err != nil {

	}

	return true
}
