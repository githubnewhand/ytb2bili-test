package handlers

import (
	"gorm.io/gorm"
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/models"
	"github.com/difyz9/ytb2bili/pkg/cos"
	"github.com/difyz9/ytb2bili/pkg/utils"

	"github.com/difyz9/ytb2bili/internal/chain_task/base"
)

type VidM3u8Handler struct {
	base.BaseTask
	App *core.AppServer
	DB  *gorm.DB
}

func NewVidm3u8Handler(name string, app *core.AppServer, stateManager *manager.StateManager, client *cos.CosClient) *VidM3u8Handler {
	return &VidM3u8Handler{
		BaseTask: base.BaseTask{
			Name:         name,
			StateManager: stateManager,
			Client:       client,
		},
		App: app,
	}
}

func (t *VidM3u8Handler) Execute(context map[string]interface{}) bool {

	err := utils.ConvertToHLS(t.StateManager.InputVideoPath, t.StateManager.M3u8FileDir)
	if err != nil {
		return false
	}

	tbVideo := &models.TbVideo{
		Id: t.StateManager.Id,

		Status: "to_m3u8",
	}
	err = t.StateManager.UpdateTBVideo(tbVideo)
	if err != nil {

	}

	return true
}
