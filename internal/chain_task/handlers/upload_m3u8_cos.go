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
)

//从tb_upload表中提取视频转码并上传到腾讯cos

type UploadM3u82CosHandler struct {
	base.BaseTask
	App *core.AppServer
	DB  *gorm.DB
}

func NewUploadM3u82CosHandler(name string, app *core.AppServer, stateManager *manager.StateManager, client *cos.CosClient) *UploadM3u82CosHandler {
	return &UploadM3u82CosHandler{
		BaseTask: base.BaseTask{
			Name:         name,
			StateManager: stateManager,
			Client:       client,
		},
		App: app,
	}
}

func (t *UploadM3u82CosHandler) Execute(context map[string]interface{}) bool {
	//audio/mpegurl
	m3U8Files, err2 := utils.ParseM3U8File(t.StateManager.M3u8FileName)

	if err2 != nil {
		return false
	}
	//video/mp2t
	newKeyName, err := t.Client.UploadM3u8ToCOS(t.StateManager.M3u8FileName, "", "audio/mpegurl")
	if err != nil {
		fmt.Println("上传视频到cos失败")
		return false
	}

	for _, filename := range m3U8Files {
		_, err := t.Client.UploadM3u8ToCOS(filename, "", "video/mp2t")
		if err != nil {
			fmt.Println("上传视频到cos失败")
			return false
		}
	}

	fmt.Println("上传视频到cos完成")

	tbVideo := &models.TbVideo{
		Id:      t.StateManager.Id,
		M3u8:    newKeyName,
		VideoId: t.StateManager.VideoID,
		Status:  "m3u8",
	}
	err = t.StateManager.UpdateTBVideo(tbVideo)
	if err != nil {

	}
	return true
}
