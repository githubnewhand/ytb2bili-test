package handlers

import (
	stdcontext "context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/difyz9/ytb2bili/internal/chain_task/base"
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/services"
	"github.com/difyz9/ytb2bili/internal/core/types"
	"github.com/difyz9/ytb2bili/pkg/cos"
	"github.com/difyz9/ytb2bili/pkg/translator"
)

var chapterLineRegexp = regexp.MustCompile(`(?m)^\s*(?:[-*]\s*)?(\b(?:\d{1,2}:)?\d{2}:\d{2}\b)(?:\s+|[ \t]*[-:][ \t]*)(.+?)\s*$`)

type extractedChapter struct {
	Timestamp   string
	Description string
}

type chaptersExtractionResult struct {
	Status               string `json:"status"`
	Success              bool   `json:"success"`
	HasChapters          bool   `json:"has_chapters"`
	MatchedCount         int    `json:"matched_count"`
	ChapterCount         int    `json:"chapter_count"`
	DescriptionLength    int    `json:"description_length"`
	Provider             string `json:"provider,omitempty"`
	Message              string `json:"message"`
	Error                string `json:"error,omitempty"`
	ExistingChaptersUsed bool   `json:"existing_chapters_used"`
	Saved                bool   `json:"saved"`
}

type ExtractChaptersHandler struct {
	base.BaseTask
	App               *core.AppServer
	SavedVideoService *services.SavedVideoService
}

func NewExtractChaptersHandler(name string, app *core.AppServer, stateManager *manager.StateManager, client *cos.CosClient, savedVideoService *services.SavedVideoService) *ExtractChaptersHandler {
	return &ExtractChaptersHandler{
		BaseTask: base.BaseTask{
			Name:         name,
			StateManager: stateManager,
			Client:       client,
		},
		App:               app,
		SavedVideoService: savedVideoService,
	}
}

func (t *ExtractChaptersHandler) Execute(context map[string]interface{}) bool {
	t.App.Logger.Info("========================================")
	t.App.Logger.Infof("开始提取视频时间轴章节: VideoID=%s", t.StateManager.VideoID)
	t.App.Logger.Info("========================================")
	types.ReportTaskProgress(context, 5, "读取视频简介")

	if t.SavedVideoService == nil {
		message := "SavedVideoService 未初始化，跳过章节提取"
		t.App.Logger.Warnf("⚠️ %s，不阻断上传", message)
		context["bili_chapters"] = ""
		t.setChaptersResult(context, chaptersExtractionResult{
			Status:  "skipped",
			Message: message,
		})
		types.ReportTaskProgress(context, 100, "跳过章节提取")
		return true
	}

	savedVideo, err := t.SavedVideoService.GetVideoByVideoID(t.StateManager.VideoID)
	if err != nil {
		message := "获取视频记录失败，跳过章节提取"
		t.App.Logger.Warnf("⚠️ %s，不阻断上传: %v", message, err)
		context["bili_chapters"] = ""
		t.setChaptersResult(context, chaptersExtractionResult{
			Status:  "skipped",
			Message: message,
			Error:   err.Error(),
		})
		types.ReportTaskProgress(context, 100, "跳过章节提取")
		return true
	}

	existingChapters := strings.TrimSpace(savedVideo.Chapters)
	description := strings.TrimSpace(savedVideo.Description)
	descriptionLength := len([]rune(description))
	if description == "" {
		message := "视频 Description 为空，跳过章节提取"
		t.App.Logger.Infof("ℹ️ %s，已有章节=%t", message, existingChapters != "")
		context["bili_chapters"] = existingChapters
		t.setChaptersResult(context, chaptersExtractionResult{
			Status:               statusForExistingChapters(existingChapters, "skipped"),
			Success:              existingChapters != "",
			HasChapters:          existingChapters != "",
			ChapterCount:         countChapterLines(existingChapters),
			DescriptionLength:    descriptionLength,
			Message:              message,
			ExistingChaptersUsed: existingChapters != "",
		})
		types.ReportTaskProgress(context, 100, "章节提取跳过：描述为空")
		return true
	}

	t.App.Logger.Infof("📄 Description 长度: %d 字符", descriptionLength)
	chapters := t.extractChapters(description)
	if len(chapters) == 0 {
		message := "未在 Description 中匹配到时间轴章节"
		t.App.Logger.Infof("ℹ️ %s，已有章节=%t", message, existingChapters != "")
		context["bili_chapters"] = existingChapters
		t.setChaptersResult(context, chaptersExtractionResult{
			Status:               statusForExistingChapters(existingChapters, "no_match"),
			Success:              existingChapters != "",
			HasChapters:          existingChapters != "",
			ChapterCount:         countChapterLines(existingChapters),
			DescriptionLength:    descriptionLength,
			Message:              message,
			ExistingChaptersUsed: existingChapters != "",
		})
		types.ReportTaskProgress(context, 100, "未匹配到章节，继续上传")
		return true
	}

	t.App.Logger.Infof("📝 匹配到 %d 条时间轴章节", len(chapters))
	for i, chapter := range chapters {
		if i >= 3 {
			break
		}
		t.App.Logger.Infof("   章节样例 %d: %s %s", i+1, chapter.Timestamp, chapter.Description)
	}
	types.ReportTaskProgress(context, 30, fmt.Sprintf("匹配到%d条章节", len(chapters)))

	descriptions := make([]string, 0, len(chapters))
	for _, chapter := range chapters {
		descriptions = append(descriptions, chapter.Description)
	}

	t.App.Logger.Info("🌐 开始批量翻译章节描述为中文")
	types.ReportTaskProgress(context, 55, "翻译章节描述")
	translatedDescriptions, provider, err := t.translateChapterDescriptions(descriptions)
	if err != nil {
		context["bili_chapters"] = existingChapters
		message := "章节翻译失败"
		if existingChapters != "" {
			t.App.Logger.Warnf("⚠️ %s，将沿用数据库中已有章节，不阻断上传: %v", message, err)
		} else {
			t.App.Logger.Warnf("⚠️ %s，跳过章节评论，不阻断上传: %v", message, err)
		}
		t.setChaptersResult(context, chaptersExtractionResult{
			Status:               statusForExistingChapters(existingChapters, "translation_failed"),
			Success:              existingChapters != "",
			HasChapters:          existingChapters != "",
			MatchedCount:         len(chapters),
			ChapterCount:         countChapterLines(existingChapters),
			DescriptionLength:    descriptionLength,
			Message:              message,
			Error:                err.Error(),
			ExistingChaptersUsed: existingChapters != "",
		})
		t.App.Logger.Info("========================================")
		types.ReportTaskProgress(context, 100, "章节翻译不可用，继续上传")
		return true
	}
	t.App.Logger.Infof("✅ 章节描述翻译完成，provider=%s", provider)

	chaptersText := t.buildChaptersText(chapters, translatedDescriptions)
	if chaptersText == "" {
		message := "章节翻译结果为空，跳过章节评论"
		t.App.Logger.Warnf("⚠️ %s，不阻断上传", message)
		context["bili_chapters"] = existingChapters
		t.setChaptersResult(context, chaptersExtractionResult{
			Status:               statusForExistingChapters(existingChapters, "empty_result"),
			Success:              existingChapters != "",
			HasChapters:          existingChapters != "",
			MatchedCount:         len(chapters),
			ChapterCount:         countChapterLines(existingChapters),
			DescriptionLength:    descriptionLength,
			Provider:             provider,
			Message:              message,
			ExistingChaptersUsed: existingChapters != "",
		})
		t.App.Logger.Info("========================================")
		types.ReportTaskProgress(context, 100, "章节结果为空，继续上传")
		return true
	}

	saved := true
	savedVideo.Chapters = chaptersText
	if err := t.SavedVideoService.UpdateVideo(savedVideo); err != nil {
		saved = false
		t.App.Logger.Warnf("⚠️ 保存章节到数据库失败，不阻断上传: %v", err)
	}

	context["bili_chapters"] = chaptersText
	message := fmt.Sprintf("已提取 %d 条章节", len(chapters))
	status := "extracted"
	if saved {
		t.App.Logger.Infof("💾 已保存视频时间轴章节，共 %d 条，provider=%s", len(chapters), provider)
	} else {
		status = "save_failed"
		message = fmt.Sprintf("已提取 %d 条章节，但保存数据库失败", len(chapters))
		t.App.Logger.Warnf("⚠️ 已生成章节但未能保存到数据库，共 %d 条，provider=%s", len(chapters), provider)
	}
	t.setChaptersResult(context, chaptersExtractionResult{
		Status:            status,
		Success:           true,
		HasChapters:       true,
		MatchedCount:      len(chapters),
		ChapterCount:      countChapterLines(chaptersText),
		DescriptionLength: descriptionLength,
		Provider:          provider,
		Message:           message,
		Saved:             saved,
	})
	t.App.Logger.Info("========================================")
	types.ReportTaskProgress(context, 100, "章节提取完成")

	return true
}

func (t *ExtractChaptersHandler) extractChapters(description string) []extractedChapter {
	matches := chapterLineRegexp.FindAllStringSubmatch(description, -1)
	chapters := make([]extractedChapter, 0, len(matches))

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		timestamp := strings.TrimSpace(match[1])
		chapterDescription := strings.TrimSpace(match[2])
		chapterDescription = strings.Trim(chapterDescription, "-: \t")
		if timestamp == "" || chapterDescription == "" {
			continue
		}

		chapters = append(chapters, extractedChapter{
			Timestamp:   timestamp,
			Description: chapterDescription,
		})
	}

	return chapters
}

func (t *ExtractChaptersHandler) translateChapterDescriptions(descriptions []string) ([]string, string, error) {
	if len(descriptions) == 0 {
		return nil, "", nil
	}

	timeout := 60 * time.Second
	if t.App != nil && t.App.Config != nil && t.App.Config.TranslatorConfig != nil && t.App.Config.TranslatorConfig.Timeout > 0 {
		timeout = time.Duration(t.App.Config.TranslatorConfig.Timeout) * time.Second
	}

	ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), timeout)
	defer cancel()

	translationManager := translator.NewTranslatorManager(t.App.Config)
	req := &translator.BatchTranslationRequest{
		Texts:      descriptions,
		SourceLang: "en",
		TargetLang: "zh",
		TextType:   "plain",
		Domain:     "general",
	}

	if result, err := translationManager.BatchTranslate(ctx, req); err == nil {
		translated, parseErr := collectTranslatedDescriptions(result, len(descriptions))
		if parseErr == nil {
			return translated, result.Provider, nil
		}
		t.App.Logger.Warnf("⚠️ 默认翻译服务返回结果不可用: %v", parseErr)
	} else {
		t.App.Logger.Warnf("⚠️ 默认翻译服务批量翻译失败: %v", err)
	}

	for _, provider := range []string{"deepseek", "baidu"} {
		result, err := translationManager.BatchTranslateWithProvider(ctx, provider, req)
		if err != nil {
			t.App.Logger.Warnf("⚠️ %s 批量翻译章节失败: %v", provider, err)
			continue
		}

		translated, parseErr := collectTranslatedDescriptions(result, len(descriptions))
		if parseErr != nil {
			t.App.Logger.Warnf("⚠️ %s 翻译结果不可用: %v", provider, parseErr)
			continue
		}

		if result.Provider != "" {
			provider = result.Provider
		}
		return translated, provider, nil
	}

	return nil, "", fmt.Errorf("所有章节翻译服务均不可用")
}

func collectTranslatedDescriptions(result *translator.BatchTranslationResult, expected int) ([]string, error) {
	if result == nil {
		return nil, fmt.Errorf("翻译结果为空")
	}
	if len(result.Results) != expected {
		return nil, fmt.Errorf("翻译结果数量不匹配: got=%d want=%d", len(result.Results), expected)
	}

	translated := make([]string, 0, expected)
	for i, item := range result.Results {
		if item == nil {
			return nil, fmt.Errorf("第 %d 条翻译结果为空", i+1)
		}

		text := strings.TrimSpace(item.TranslatedText)
		if text == "" {
			return nil, fmt.Errorf("第 %d 条翻译文本为空", i+1)
		}
		translated = append(translated, text)
	}

	return translated, nil
}

func (t *ExtractChaptersHandler) buildChaptersText(chapters []extractedChapter, translatedDescriptions []string) string {
	if len(chapters) == 0 || len(chapters) != len(translatedDescriptions) {
		return ""
	}

	lines := make([]string, 0, len(chapters))
	for i, chapter := range chapters {
		lines = append(lines, fmt.Sprintf("%s %s", chapter.Timestamp, strings.TrimSpace(translatedDescriptions[i])))
	}

	return strings.Join(lines, "\n")
}

func (t *ExtractChaptersHandler) setChaptersResult(context map[string]interface{}, result chaptersExtractionResult) {
	if context == nil {
		return
	}

	context["chapters_result"] = result
	context["chapters_status"] = result.Status
	context["chapters_success"] = result.Success
	context["chapters_count"] = result.ChapterCount
	context["chapters_message"] = result.Message

	t.App.Logger.Infof(
		"📌 章节提取结果: status=%s success=%t matched=%d chapters=%d saved=%t existing_used=%t message=%s",
		result.Status,
		result.Success,
		result.MatchedCount,
		result.ChapterCount,
		result.Saved,
		result.ExistingChaptersUsed,
		result.Message,
	)
}

func statusForExistingChapters(existingChapters, fallback string) string {
	if strings.TrimSpace(existingChapters) != "" {
		return "used_existing"
	}
	return fallback
}

func countChapterLines(chaptersText string) int {
	count := 0
	for _, line := range strings.Split(chaptersText, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}
