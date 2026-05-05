package handlers

import (
	"fmt"

	"github.com/difyz9/ytb2bili/internal/chain_task/base"
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/types"
	"github.com/difyz9/ytb2bili/pkg/cos"
	"github.com/difyz9/ytb2bili/pkg/utils"
	"gorm.io/gorm"
)

type ExtractAudio struct {
	base.BaseTask
	App *core.AppServer
	DB  *gorm.DB
}

func NewExtractAudio(name string, app *core.AppServer, stateManager *manager.StateManager, client *cos.CosClient) *ExtractAudio {
	return &ExtractAudio{
		BaseTask: base.BaseTask{
			Name:         name,
			StateManager: stateManager,
			Client:       client,
		},
		App: app,
	}
}

func (t *ExtractAudio) Execute(context map[string]interface{}) bool {
	fmt.Println("开始分离音频")
	types.ReportTaskProgress(context, 20, "开始分离音频")
	if err := utils.ExtractWaveAudio(t.StateManager.InputVideoPath, t.StateManager.OriginalWAV); err != nil {
		fmt.Println("--- 分离音频失败-----")
	}
	fmt.Println("分离音频完成")
	types.ReportTaskProgress(context, 90, "音频分离完成")
	return true
}
