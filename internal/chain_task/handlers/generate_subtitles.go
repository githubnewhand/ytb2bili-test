package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/difyz9/ytb2bili/internal/chain_task/base"
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/services"
	"github.com/difyz9/ytb2bili/internal/core/types"
	"github.com/difyz9/ytb2bili/pkg/cos"
	// "github.com/difyz9/ytb2bili/pkg/utils"
	"github.com/difyz9/ytb2bili/pkg/store/model"
	"github.com/difyz9/ytb2bili/pkg/subtitle"
)

type GenerateSubtitles struct {
	base.BaseTask
	App               *core.AppServer
	SavedVideoService *services.SavedVideoService
}

func NewGenerateSubtitles(name string, app *core.AppServer, stateManager *manager.StateManager, client *cos.CosClient, savedVideoService *services.SavedVideoService) *GenerateSubtitles {
	return &GenerateSubtitles{
		BaseTask: base.BaseTask{
			Name:         name,
			StateManager: stateManager,
			Client:       client,
		},
		App:               app,
		SavedVideoService: savedVideoService,
	}
}

// formatTime 将秒数转换为 SRT 时间格式 (HH:MM:SS,mmm)
func (t *GenerateSubtitles) formatTime(seconds float64) string {
	hours := int(seconds / 3600)
	minutes := int((seconds - float64(hours*3600)) / 60)
	secs := int(seconds - float64(hours*3600) - float64(minutes*60))
	milliseconds := int((seconds - float64(int(seconds))) * 1000)

	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, secs, milliseconds)
}

// generateSRT 生成 SRT 格式字幕内容
func (t *GenerateSubtitles) generateSRT(subtitles []model.SavedVideoSubtitle) string {
	var srtContent strings.Builder

	for i, subtitle := range subtitles {
		// SRT 序号（从1开始）
		srtContent.WriteString(fmt.Sprintf("%d\n", i+1))

		// 时间轴
		startTime := t.formatTime(subtitle.Offset)
		endTime := t.formatTime(subtitle.Offset + subtitle.Duration)
		srtContent.WriteString(fmt.Sprintf("%s --> %s\n", startTime, endTime))

		// 字幕文本
		srtContent.WriteString(subtitle.Text)
		srtContent.WriteString("\n\n")
	}

	return srtContent.String()
}

func (t *GenerateSubtitles) Execute(context map[string]interface{}) bool {
	stepName := "生成字幕"
	t.App.Logger.Infof("开始执行任务: %s", stepName)
	types.ReportTaskProgress(context, 5, "检查字幕来源")

	// 1. 获取视频信息
	savedVideo, err := t.SavedVideoService.GetVideoByID(t.StateManager.Id)
	if err != nil {
		t.App.Logger.Errorf("获取视频信息失败: %v", err)
		return false
	}

	targetSrtPath := filepath.Join(t.StateManager.CurrentDir, savedVideo.VideoID+".srt")

	// =================================================================
	// === 阶段零：优先尝试下载远端字幕 (新增功能) ===
	// =================================================================
	if savedVideo.URL != "" {
		t.App.Logger.Info(">>> 尝试下载 YouTube 原生字幕...")
		types.ReportTaskProgress(context, 15, "尝试下载原生字幕")

		// 【关键步骤 1】强制删除旧文件，确保下载的是最新版，且防止 yt-dlp 跳过下载
		if _, err := os.Stat(targetSrtPath); err == nil {
			t.App.Logger.Infof("🗑️ 删除旧字幕文件以获取最新: %s", targetSrtPath)
			_ = os.Remove(targetSrtPath)
		}

		downloader := subtitle.NewYtdlpSubtitleDownloader(t.App.Logger)

		// 临时文件前缀
		tempPrefix := filepath.Join(t.StateManager.CurrentDir, savedVideo.VideoID)

		// 1. 优先尝试中文字幕
		downloadedFile, err := downloader.DownloadChineseSubtitle(savedVideo.URL, "srt", tempPrefix)
		if err != nil {
			// 2. 如果没有中文，尝试英文字幕
			t.App.Logger.Infof("未找到中文字幕，尝试英文字幕...")
			downloadedFile, err = downloader.DownloadEnglishSubtitle(savedVideo.URL, "srt", tempPrefix)
		}

		// 检查是否下载成功
		if err == nil && downloadedFile != "" {
			if info, err := os.Stat(downloadedFile); err == nil && info.Size() > 0 {
				t.App.Logger.Infof("✅ 已下载原生字幕: %s", downloadedFile)

				// 【关键步骤 2】智能处理文件路径
				// 情况 A: yt-dlp 下载的文件名正好就是 targetSrtPath (例如 .../id.srt)
				// 此时不需要移动，直接返回成功
				if downloadedFile == targetSrtPath {
					t.App.Logger.Infof("✅ 原生字幕已就位: %s", targetSrtPath)
					types.ReportTaskProgress(context, 95, "原生字幕已就位")
					return true
				}

				// 情况 B: yt-dlp 下载的是带后缀的文件 (例如 .../id.zh-Hans.srt)
				// 需要重命名为 targetSrtPath
				if err := os.Rename(downloadedFile, targetSrtPath); err == nil {
					t.App.Logger.Infof("✅ 原生字幕已应用: %s", targetSrtPath)
					types.ReportTaskProgress(context, 95, "原生字幕已应用")
					return true
				} else {
					t.App.Logger.Errorf("重命名原生字幕失败: %v", err)
					// 重命名失败不立即返回 false，尝试让后续流程兜底
				}
			}
		} else {
			t.App.Logger.Warnf("⚠️ 下载原生字幕失败或不存在，将降级使用本地生成逻辑... (错误: %v)", err)
		}
	} else {
		t.App.Logger.Warn("⚠️ 视频 URL 为空，无法下载原生字幕，跳过此步骤")
	}

	// =================================================================
	// === 阶段一：现场抢救 (修复 en.srt 问题) ===
	// =================================================================
	// 检查是否有 Whisper 生成的默认文件名 (en.srt, main.srt 等)
	possibleNames := []string{"en.srt", "zh.srt", "output.srt", "main.srt"}
	for _, name := range possibleNames {
		wrongPath := filepath.Join(t.StateManager.CurrentDir, name)
		if info, err := os.Stat(wrongPath); err == nil && info.Size() > 0 {
			t.App.Logger.Infof("🔧 发现遗留字幕文件: %s (大小: %d)，正在重命名...", name, info.Size())

			// 确保目标位置干净
			_ = os.Remove(targetSrtPath)

			if err := os.Rename(wrongPath, targetSrtPath); err == nil {
				t.App.Logger.Infof("✅ 已成功重命名为: %s", targetSrtPath)
				t.App.Logger.Infof("⚠️ 注意：字幕仅存在于文件中，数据库可能仍为空，翻译步骤可能会受影响。")
				types.ReportTaskProgress(context, 95, "字幕文件已修复")
				return true
			}
		}
	}

	// 如果目标文件已经存在且不为空（可能是下载失败后，旧文件其实还在？不，我们在阶段零删了。所以这里是检查是否有其他生成的）
	if info, err := os.Stat(targetSrtPath); err == nil && info.Size() > 0 {
		t.App.Logger.Infof("✅ 字幕文件已存在: %s，跳过生成", targetSrtPath)
		types.ReportTaskProgress(context, 95, "字幕文件已存在")
		return true
	}

	// =================================================================
	// === 阶段二：智能补全 (数据库为空时触发) ===
	// =================================================================
	isDBEmpty := savedVideo.Subtitles == "" || savedVideo.Subtitles == "null" || savedVideo.Subtitles == "[]" || len(savedVideo.Subtitles) < 10

	if isDBEmpty {
		t.App.Logger.Warn("⚠️ 数据库字幕缺失且本地无字幕文件，启动【自动补全】流程...")
		types.ReportTaskProgress(context, 35, "补全字幕数据")

		wavPath := filepath.Join(t.StateManager.CurrentDir, savedVideo.VideoID+".wav")

		// 1. 检查音频，没有则分离
		if info, err := os.Stat(wavPath); err != nil || info.Size() == 0 {
			t.App.Logger.Info(">>> 补办任务: 分离音频...")
			types.ReportTaskProgress(context, 45, "分离音频")
			extractTask := NewExtractAudio("强制分离音频", t.App, t.StateManager, t.App.CosClient)
			extractTask.Execute(context)
		}

		// 2. 执行 Whisper
		if t.App.Config.WhisperConfig != nil && t.App.Config.WhisperConfig.Enabled {
			t.App.Logger.Info(">>> 补办任务: Whisper转录...")
			types.ReportTaskProgress(context, 60, "语音转录")
			whisperTask := NewWhisperHandler(
				"强制Whisper",
				t.App,
				t.StateManager,
				t.App.CosClient,
				t.App.Config.WhisperConfig.ModelPath,
				t.App.Config.WhisperConfig.Language,
				t.App.Config.WhisperConfig.Threads,
			)
			whisperTask.Execute(context)

			// 3. 再次检查是否生成了 en.srt 等怪文件
			for _, name := range possibleNames {
				wrongPath := filepath.Join(t.StateManager.CurrentDir, name)
				if info, err := os.Stat(wrongPath); err == nil && info.Size() > 0 {
					os.Rename(wrongPath, targetSrtPath)
					t.App.Logger.Infof("✅ 补全成功，已重命名文件: %s", targetSrtPath)
					types.ReportTaskProgress(context, 95, "字幕补全完成")
					return true
				}
			}

			// 刷新数据库
			savedVideo, _ = t.SavedVideoService.GetVideoByID(t.StateManager.Id)
		}
	}

	// =================================================================
	// === 阶段三：常规生成 (原始功能) ===
	// =================================================================
	if savedVideo.Subtitles == "" || savedVideo.Subtitles == "null" || savedVideo.Subtitles == "[]" {
		t.App.Logger.Error("❌ 最终检查: 数据库无字幕数据，且未能生成有效字幕文件")
		return false
	}

	type SubtitleItem struct {
		From    string `json:"from"`
		To      string `json:"to"`
		Content string `json:"content"`
	}
	var subtitles []SubtitleItem

	if err := json.Unmarshal([]byte(savedVideo.Subtitles), &subtitles); err != nil {
		t.App.Logger.Errorf("解析字幕数据失败: %v", err)
		return false
	}

	if len(subtitles) == 0 {
		t.App.Logger.Error("❌ 字幕数组为空")
		return false
	}

	t.App.Logger.Infof("正在从数据库生成SRT文件: %s", targetSrtPath)
	types.ReportTaskProgress(context, 80, "写入SRT文件")
	file, err := os.Create(targetSrtPath)
	if err != nil {
		t.App.Logger.Errorf("创建文件失败: %v", err)
		return false
	}
	defer file.Close()

	for i, sub := range subtitles {
		if sub.Content == "" {
			continue
		}
		fmt.Fprintf(file, "%d\n%s --> %s\n%s\n\n", i+1, sub.From, sub.To, sub.Content)
	}

	t.App.Logger.Info("✅ 字幕文件生成成功 (来源: 数据库)")
	types.ReportTaskProgress(context, 95, "字幕文件生成完成")
	return true
}

// truncateString 截断字符串，避免日志过长
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
