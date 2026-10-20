package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/difyz9/ytb2bili/internal/core/types"
)

func (g *GenerateMetadata) executeWithOllama(taskContext map[string]interface{}) bool {
	endpoint, model, timeout, err := ollamaMetadataSettings(g.App.Config)
	if err == nil {
		err = checkOllamaReadyAt(endpoint, model)
	}
	if err != nil {
		taskContext["error"] = fmt.Sprintf("本机 Ollama 元数据服务不可用: %v", err)
		return false
	}
	types.ReportTaskProgress(taskContext, 20, "读取中文字幕")
	source, err := os.ReadFile(filepath.Join(g.StateManager.CurrentDir, "zh.srt"))
	if err != nil {
		taskContext["error"] = fmt.Sprintf("读取中文字幕失败: %v", err)
		return false
	}
	text := g.extractTextFromSRT(string(source))
	if strings.TrimSpace(text) == "" {
		taskContext["error"] = "中文字幕为空，无法生成元数据"
		return false
	}
	types.ReportTaskProgress(taskContext, 50, "本地生成标题和简介")
	metadata, err := g.generateMetadataFromOllama(endpoint, model, timeout, sampleMetadataText(text), g.originalTitleForMetadata())
	if err != nil {
		taskContext["error"] = fmt.Sprintf("Ollama 生成元数据失败: %v", err)
		return false
	}
	return g.saveMetadataResults(metadata, taskContext)
}

func sampleMetadataText(text string) string {
	runes := []rune(text)
	if len(runes) <= 2400 {
		return text
	}
	middle := (len(runes) - 800) / 2
	return string(runes[:800]) + " … [中段] … " + string(runes[middle:middle+800]) + " … [末段] … " + string(runes[len(runes)-800:])
}

func (g *GenerateMetadata) originalTitleForMetadata() string {
	if g.SavedVideoService != nil {
		if video, err := g.SavedVideoService.GetVideoByVideoID(g.StateManager.VideoID); err == nil && strings.TrimSpace(video.Title) != "" {
			return strings.TrimSpace(video.Title)
		}
	}
	data, err := os.ReadFile(filepath.Join(g.StateManager.CurrentDir, "source_metadata.json"))
	if err != nil {
		return ""
	}
	var original struct {
		Title string `json:"title"`
	}
	if json.Unmarshal(data, &original) != nil {
		return ""
	}
	return strings.TrimSpace(original.Title)
}

func (g *GenerateMetadata) generateMetadataFromOllama(endpoint, model string, timeout time.Duration, subtitleText, originalTitle string) (*VideoMetadata, error) {
	system := "你是谨慎的中文视频编辑。原视频标题比自动识别字幕更可靠。只概述标题和字幕共同支持的主题；不要引用对话、列举识别错词、推测具体手法或用品，也不要编造人物身份、效果或外部链接。只输出JSON对象。"
	prompt := fmt.Sprintf("原视频标题：%s\n中文字幕摘录（依时间顺序，含开头、中段和末段）：%s\n请返回JSON对象，字段为title、description、tags。title不超过30个汉字；description为80至160字的中文简介，只写主题和观看体验，并用一句话说明字幕可能有自动识别误差；不要写具体操作技术、发型分析、错词例子或字幕原句；tags为3至5个简短中文标签。不要写任何网址，原视频链接由上传程序单独添加。", originalTitle, subtitleText)
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		content, err := callOllamaJSON(endpoint, model, timeout, system, prompt)
		if err != nil {
			lastErr = err
			continue
		}
		normalized, err := normalizeMetadataJSON(content)
		if err != nil {
			lastErr = err
			continue
		}
		var metadata VideoMetadata
		if err := json.Unmarshal([]byte(normalized), &metadata); err != nil {
			lastErr = err
			continue
		}
		if err := validateGeneratedMetadata(&metadata); err != nil {
			lastErr = err
			continue
		}
		if utf8.RuneCountInString(metadata.Title) > 80 || utf8.RuneCountInString(metadata.Description) > 300 || strings.Contains(strings.ToLower(metadata.Description), "http") || strings.Contains(strings.ToLower(metadata.Description), "example") || strings.ContainsAny(metadata.Description, "“”‘’") {
			lastErr = fmt.Errorf("标题或描述包含超长内容或未经核实的链接")
			continue
		}
		metadata.Title = strings.TrimSpace(metadata.Title)
		metadata.Description = strings.TrimSpace(metadata.Description)
		metadata.Description = strings.ReplaceAll(metadata.Description, "实际内容以视频标题为准", "请以原视频为准")
		if !strings.Contains(metadata.Description, "字幕") {
			metadata.Description += " 中文字幕由自动翻译生成，可能有误，请以原视频为准。"
		}
		for i := range metadata.Tags {
			metadata.Tags[i] = strings.TrimSpace(metadata.Tags[i])
		}
		return &metadata, nil
	}
	return nil, fmt.Errorf("连续3次未获得有效元数据: %w", lastErr)
}
