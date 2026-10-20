package handlers

import (
	"fmt"
	"os"

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

	contextPath, _ := context["downloaded_file"].(string)
	sourcePath, err := findAudioSource(t.StateManager.InputVideoPath, contextPath, t.StateManager.CurrentDir)
	if err != nil {
		errMsg := fmt.Sprintf("无法生成 WAV：%v", err)
		context["error"] = errMsg
		if t.App != nil && t.App.Logger != nil {
			t.App.Logger.Error(errMsg)
		}
		return false
	}

	if err := os.MkdirAll(t.StateManager.CurrentDir, 0755); err != nil {
		errMsg := fmt.Sprintf("创建音频目录失败: %v", err)
		context["error"] = errMsg
		return false
	}

	if err := utils.ExtractWaveAudio(sourcePath, t.StateManager.OriginalWAV); err != nil {
		errMsg := fmt.Sprintf("从媒体文件提取 WAV 失败（来源: %s）: %v", sourcePath, err)
		context["error"] = errMsg
		if t.App != nil && t.App.Logger != nil {
			t.App.Logger.Error(errMsg)
		}
		_ = os.Remove(t.StateManager.OriginalWAV)
		return false
	}
	if !nonEmptyFile(t.StateManager.OriginalWAV) {
		errMsg := fmt.Sprintf("WAV 提取命令结束但输出文件无效: %s", t.StateManager.OriginalWAV)
		context["error"] = errMsg
		return false
	}

	context["audio_source"] = sourcePath
	context["wav_path"] = t.StateManager.OriginalWAV
	delete(context, "error")
	fmt.Println("分离音频完成")
	types.ReportTaskProgress(context, 90, "音频分离完成")
	return true
}
