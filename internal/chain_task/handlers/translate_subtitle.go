package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/difyz9/ytb2bili/internal/chain_task/base"
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/types"
	"github.com/difyz9/ytb2bili/pkg/cos"
	"github.com/difyz9/ytb2bili/pkg/utils"
	"gorm.io/gorm"
)

type TranslateSubtitle struct {
	base.BaseTask
	App         *core.AppServer
	DB          *gorm.DB
	APIKey      string
	GroupSize   int
	MaxWorkers  int // 最大并发数
	RateLimiter *adaptiveRateLimiter
}

func NewTranslateSubtitle(name string, app *core.AppServer, stateManager *manager.StateManager, client *cos.CosClient, db *gorm.DB, apiKey string) *TranslateSubtitle {
	return &TranslateSubtitle{
		BaseTask: base.BaseTask{
			Name:         name,
			StateManager: stateManager,
			Client:       client,
		},
		App:        app,
		DB:         db,
		APIKey:     "", // 不再固化API Key，运行时动态获取
		GroupSize:  15, // 每组15句，降低单次请求长度
		MaxWorkers: 5,  // worker可以较高，真实请求节奏由自适应限速器控制
	}
}

// getCurrentAPIKey 获取当前的DeepSeek API Key（实时从配置中读取）
func (t *TranslateSubtitle) getCurrentAPIKey() (string, error) {
	if t.App.Config.DeepSeekTransConfig == nil || !t.App.Config.DeepSeekTransConfig.Enabled {
		return "", fmt.Errorf("DeepSeek 翻译服务未启用")
	}

	apiKey := t.App.Config.DeepSeekTransConfig.ApiKey
	if apiKey == "" {
		return "", fmt.Errorf("DeepSeek API Key 未配置")
	}

	return apiKey, nil
}

// SRTEntry SRT字幕条目
type SRTEntry struct {
	Index    int
	TimeCode string
	Text     string
}

type adaptiveRateLimiter struct {
	mu            sync.Mutex
	interval      time.Duration
	minInterval   time.Duration
	maxInterval   time.Duration
	nextAllowed   time.Time
	successStreak int
}

func newAdaptiveRateLimiter(initial, minInterval, maxInterval time.Duration) *adaptiveRateLimiter {
	return &adaptiveRateLimiter{
		interval:    initial,
		minInterval: minInterval,
		maxInterval: maxInterval,
	}
}

func (l *adaptiveRateLimiter) Wait() {
	if l == nil {
		return
	}

	l.mu.Lock()
	now := time.Now()
	if l.nextAllowed.Before(now) {
		l.nextAllowed = now
	}
	wait := l.nextAllowed.Sub(now)
	l.nextAllowed = l.nextAllowed.Add(l.interval)
	l.mu.Unlock()

	if wait > 0 {
		time.Sleep(wait)
	}
}

func (l *adaptiveRateLimiter) ReportSuccess() time.Duration {
	if l == nil {
		return 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.successStreak++
	if l.successStreak >= 8 && l.interval > l.minInterval {
		l.interval = l.interval * 85 / 100
		if l.interval < l.minInterval {
			l.interval = l.minInterval
		}
		l.successStreak = 0
	}

	return l.interval
}

func (l *adaptiveRateLimiter) ReportFailure(err error) time.Duration {
	if l == nil || err == nil {
		return 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	errorStr := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errorStr, "429") || strings.Contains(errorStr, "rate limit"):
		l.interval *= 2
	case strings.Contains(errorStr, "timeout") ||
		strings.Contains(errorStr, "deadline exceeded") ||
		strings.Contains(errorStr, "connection") ||
		strings.Contains(errorStr, "500") ||
		strings.Contains(errorStr, "502") ||
		strings.Contains(errorStr, "503") ||
		strings.Contains(errorStr, "504"):
		l.interval = l.interval * 140 / 100
	default:
		return l.interval
	}

	if l.interval > l.maxInterval {
		l.interval = l.maxInterval
	}
	l.successStreak = 0

	if l.nextAllowed.Before(time.Now().Add(l.interval)) {
		l.nextAllowed = time.Now().Add(l.interval)
	}

	return l.interval
}

func (t *TranslateSubtitle) Execute(context map[string]interface{}) bool {
	t.App.Logger.Info("========================================")
	t.App.Logger.Infof("开始翻译字幕: VideoID=%s", t.StateManager.VideoID)
	t.App.Logger.Info("========================================")
	types.ReportTaskProgress(context, 5, "检查翻译配置")

	// 0. 动态获取最新的API Key配置
	currentAPIKey, err := t.getCurrentAPIKey()
	if err != nil {
		t.App.Logger.Errorf("❌ %v", err)
		context["error"] = t.getTranslationError(err)
		return false
	}

	t.App.Logger.Infof("🔑 使用DeepSeek API Key: %s", maskAPIKey(currentAPIKey))
	// 更新当前使用的API Key
	t.APIKey = currentAPIKey
	t.RateLimiter = newAdaptiveRateLimiter(800*time.Millisecond, 350*time.Millisecond, 6*time.Second)
	types.ReportTaskProgress(context, 10, "读取字幕文件")

	// 1. 检查英文字幕文件是否存在（由 GenerateSubtitles 任务生成）
	enSRTPath := filepath.Join(t.StateManager.CurrentDir, fmt.Sprintf("%s.srt", t.StateManager.VideoID))
	if _, err := os.Stat(enSRTPath); os.IsNotExist(err) {
		t.App.Logger.Error("❌ 英文字幕文件不存在，无法翻译")
		context["error"] = "翻译失败：缺少英文字幕文件，请先完成生成字幕"
		return false
	}

	// 2. 读取并解析英文字幕文件
	srtContent, err := os.ReadFile(enSRTPath)
	if err != nil {
		t.App.Logger.Errorf("❌ 读取英文字幕文件失败: %v", err)
		context["error"] = "字幕文件读取失败，请确认字幕生成步骤已完成"
		return false
	}

	srtEntries, err := t.parseSRTContent(string(srtContent))
	if err != nil {
		t.App.Logger.Errorf("❌ 解析SRT文件失败: %v", err)
		context["error"] = "字幕文件格式错误，无法解析SRT内容"
		return false
	}

	if len(srtEntries) == 0 {
		t.App.Logger.Error("❌ 字幕内容为空，无法翻译")
		context["error"] = "翻译失败：英文字幕内容为空"
		return false
	}

	t.App.Logger.Infof("📝 找到 %d 条字幕", len(srtEntries))
	types.ReportTaskProgress(context, 15, fmt.Sprintf("解析到%d条字幕", len(srtEntries)))

	// 3. 提取文本进行翻译
	var texts []string
	for _, entry := range srtEntries {
		texts = append(texts, entry.Text)
	}

	// 4. 执行并发翻译
	totalGroups := len(t.buildTranslationGroups(texts))
	t.App.Logger.Infof("🚀 开始并发翻译，每组最多 %d 句，共 %d 组，并发数: %d", t.GroupSize, totalGroups, t.MaxWorkers)

	translatedTexts, err := t.translateTextsInGroupsConcurrent(texts, context)
	if err != nil {
		t.App.Logger.Errorf("❌ 翻译失败: %v", err)
		context["error"] = t.getTranslationError(err)
		return false
	}

	// 5. 生成中文字幕SRT
	types.ReportTaskProgress(context, 88, "生成中文字幕")
	translatedSRT := t.generateTranslatedSRTContent(srtEntries, translatedTexts)

	// 6. 保存中文字幕文件
	types.ReportTaskProgress(context, 92, "保存中文字幕文件")
	zhSRTPath := filepath.Join(t.StateManager.CurrentDir, "zh.srt")
	if err := os.WriteFile(zhSRTPath, []byte(translatedSRT), 0644); err != nil {
		t.App.Logger.Errorf("❌ 保存中文字幕失败: %v", err)
		context["error"] = "保存翻译字幕文件失败，请检查磁盘空间和文件权限"
		return false
	}

	// 7. 字幕质量校验和优化
	types.ReportTaskProgress(context, 95, "校验字幕质量")
	optimizedPath, validationResult, err := t.validateAndOptimizeSubtitles(enSRTPath, zhSRTPath)
	if err != nil {
		t.App.Logger.Warnf("⚠️  字幕校验失败，使用原始翻译: %v", err)
	} else {
		if validationResult.MissingEntries > 0 {
			t.App.Logger.Infof("🔧 检测到 %d 个问题条目，已尝试修复 %d 个",
				validationResult.MissingEntries, len(validationResult.FixedEntries))

			if optimizedPath != "" {
				// 使用优化后的文件替换原文件
				if err := os.Rename(optimizedPath, zhSRTPath); err == nil {
					t.App.Logger.Info("✨ 已应用字幕优化结果")
				}
			}
		}
	}

	// 8. 保存文件路径到 context
	context["en_srt_path"] = enSRTPath
	context["zh_srt_path"] = zhSRTPath
	context["translated_count"] = len(translatedTexts)

	// 添加校验结果信息
	if validationResult != nil {
		context["validation_result"] = map[string]interface{}{
			"total_entries":   validationResult.TotalEntries,
			"valid_entries":   validationResult.ValidEntries,
			"missing_entries": validationResult.MissingEntries,
			"fixed_entries":   len(validationResult.FixedEntries),
		}
	}

	t.App.Logger.Infof("✓ 中文字幕已保存: %s", zhSRTPath)
	t.App.Logger.Infof("✓ 翻译完成: %d/%d 条字幕", len(translatedTexts), len(texts))
	t.App.Logger.Info("========================================")
	types.ReportTaskProgress(context, 98, "翻译步骤收尾")

	return true
}

// parseSRTContent 解析SRT文件内容
func (t *TranslateSubtitle) parseSRTContent(content string) ([]SRTEntry, error) {
	lines := strings.Split(content, "\n")
	var entries []SRTEntry
	var currentEntry SRTEntry
	var textLines []string
	stage := 0 // 0=等待序号, 1=等待时间码, 2=读取文本

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" {
			// 空行表示一个条目结束
			if stage == 2 && len(textLines) > 0 {
				currentEntry.Text = strings.Join(textLines, "\n")
				entries = append(entries, currentEntry)
				textLines = nil
				stage = 0
			}
			continue
		}

		switch stage {
		case 0: // 读取序号
			var index int
			if _, err := fmt.Sscanf(line, "%d", &index); err == nil {
				currentEntry = SRTEntry{Index: index}
				stage = 1
			}
		case 1: // 读取时间码
			if strings.Contains(line, "-->") {
				currentEntry.TimeCode = line
				stage = 2
			}
		case 2: // 读取文本
			textLines = append(textLines, line)
		}
	}

	// 处理最后一个条目（如果文件末尾没有空行）
	if stage == 2 && len(textLines) > 0 {
		currentEntry.Text = strings.Join(textLines, "\n")
		entries = append(entries, currentEntry)
	}

	return entries, nil
}

// generateTranslatedSRTContent 生成翻译后的SRT内容（保持原时间轴）
func (t *TranslateSubtitle) generateTranslatedSRTContent(entries []SRTEntry, translatedTexts []string) string {
	var builder strings.Builder

	for i, entry := range entries {
		builder.WriteString(fmt.Sprintf("%d\n", entry.Index))
		builder.WriteString(fmt.Sprintf("%s\n", entry.TimeCode))

		if i < len(translatedTexts) {
			builder.WriteString(fmt.Sprintf("%s\n\n", translatedTexts[i]))
		} else {
			builder.WriteString(fmt.Sprintf("%s\n\n", entry.Text))
		}
	}

	return builder.String()
}

// translateTextsInGroupsConcurrent 并发分组翻译文本
func (t *TranslateSubtitle) translateTextsInGroupsConcurrent(texts []string, context map[string]interface{}) ([]string, error) {
	groups := t.buildTranslationGroups(texts)
	totalGroups := len(groups)
	results := make([][]string, totalGroups)
	if totalGroups == 0 {
		return []string{}, nil
	}
	types.ReportTaskProgress(context, 20, fmt.Sprintf("开始翻译%d组字幕", totalGroups))

	// 创建工作池
	type translateTask struct {
		groupIndex int
		texts      []string
	}

	taskChannel := make(chan translateTask, totalGroups)
	resultChannel := make(chan struct {
		groupIndex int
		result     []string
		err        error
	}, totalGroups)

	// 启动工作者
	var wg sync.WaitGroup
	workerCount := t.MaxWorkers
	if workerCount > totalGroups {
		workerCount = totalGroups
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			t.App.Logger.Debugf("🔧 启动翻译工作者 %d", workerID)

			for task := range taskChannel {
				t.App.Logger.Infof("⏳ 工作者 %d 处理第 %d/%d 组 (%d句)",
					workerID, task.groupIndex+1, totalGroups, len(task.texts))

				// 优先批量翻译；失败时自动拆小组，避免单组失败拖垮整条任务链。
				translated, err := t.translateGroupResilient(task.texts)

				resultChannel <- struct {
					groupIndex int
					result     []string
					err        error
				}{
					groupIndex: task.groupIndex,
					result:     translated,
					err:        err,
				}
			}
		}(i)
	}

	// 分发任务
	go func() {
		for i, groupTexts := range groups {
			taskChannel <- translateTask{
				groupIndex: i,
				texts:      groupTexts,
			}
		}
		close(taskChannel)
	}()

	// 收集结果
	go func() {
		wg.Wait()
		close(resultChannel)
	}()

	// 处理结果
	completedGroups := 0
	failedGroups := 0
	var fatalErr error
	for result := range resultChannel {
		if result.err != nil {
			if len(result.result) == 0 {
				fatalErr = result.err
				t.App.Logger.Errorf("❌ 第 %d 组翻译失败: %v", result.groupIndex+1, result.err)
				continue
			}
			t.App.Logger.Warnf("⚠️  第 %d 组翻译降级完成: %v", result.groupIndex+1, result.err)
			failedGroups++
		}
		results[result.groupIndex] = result.result
		completedGroups++
		progress := 20 + (completedGroups * 65 / totalGroups)
		if progress > 85 {
			progress = 85
		}
		types.ReportTaskProgress(context, progress, fmt.Sprintf("已翻译 %d/%d 组", completedGroups, totalGroups))
	}

	if fatalErr != nil {
		return nil, fatalErr
	}

	// 合并结果
	var allTranslated []string
	for i, groupResult := range results {
		if len(groupResult) == 0 {
			return nil, fmt.Errorf("第 %d 组翻译没有结果", i+1)
		}
		allTranslated = append(allTranslated, groupResult...)
	}

	if failedGroups > 0 {
		t.App.Logger.Warnf("⚠️  有 %d 组触发降级翻译，字幕文件已尽量保留完整", failedGroups)
	}

	return allTranslated, nil
}

// buildTranslationGroups 按句数和字符数切分字幕，避免单次请求过长。
func (t *TranslateSubtitle) buildTranslationGroups(texts []string) [][]string {
	const maxGroupChars = 3000

	var groups [][]string
	var current []string
	currentChars := 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		group := make([]string, len(current))
		copy(group, current)
		groups = append(groups, group)
		current = nil
		currentChars = 0
	}

	groupSize := t.GroupSize
	if groupSize <= 0 {
		groupSize = 15
	}

	for _, text := range texts {
		textChars := len([]rune(text))
		if len(current) > 0 && (len(current) >= groupSize || currentChars+textChars > maxGroupChars) {
			flush()
		}
		current = append(current, text)
		currentChars += textChars
	}
	flush()

	return groups
}

// translateGroupResilient 批量失败后自动拆小组，最后逐条兜底，尽量产出完整字幕。
func (t *TranslateSubtitle) translateGroupResilient(texts []string) ([]string, error) {
	translated, err := t.translateGroupSimple(texts)
	if err == nil {
		return translated, nil
	}

	if t.isFatalTranslationError(err) {
		return nil, err
	}

	t.App.Logger.Warnf("⚠️  批量翻译失败，准备降级处理 (%d句): %v", len(texts), err)
	if len(texts) == 1 {
		return []string{fmt.Sprintf("[翻译失败] %s", texts[0])}, err
	}

	const fallbackGroupSize = 5
	if len(texts) > fallbackGroupSize {
		var allTranslated []string
		var lastErr error
		for i := 0; i < len(texts); i += fallbackGroupSize {
			end := i + fallbackGroupSize
			if end > len(texts) {
				end = len(texts)
			}
			partTranslated, partErr := t.translateGroupResilient(texts[i:end])
			if partErr != nil {
				lastErr = partErr
			}
			allTranslated = append(allTranslated, partTranslated...)
		}
		return allTranslated, lastErr
	}

	var allTranslated []string
	var lastErr error
	for _, text := range texts {
		itemTranslated, itemErr := t.translateGroupResilient([]string{text})
		if itemErr != nil {
			lastErr = itemErr
		}
		allTranslated = append(allTranslated, itemTranslated...)
	}

	return allTranslated, lastErr
}

func (t *TranslateSubtitle) isFatalTranslationError(err error) bool {
	if err == nil {
		return false
	}
	errorStr := strings.ToLower(err.Error())
	fatalMarkers := []string{
		"api key",
		"未配置",
		"unauthorized",
		"401",
		"403",
		"insufficient_quota",
		"quota",
	}
	for _, marker := range fatalMarkers {
		if strings.Contains(errorStr, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

// translateGroupSimple 简化的组翻译（无上下文，更快速）
func (t *TranslateSubtitle) translateGroupSimple(texts []string) ([]string, error) {
	if len(texts) == 0 {
		return []string{}, nil
	}

	// 直接组合文本
	combinedText := strings.Join(texts, "\n###SENTENCE_BREAK###\n")

	// 简化的系统提示
	systemPrompt := fmt.Sprintf(`你是一个专业的视频字幕翻译专家。将给出的 %d 句英文字幕翻译成中文。

翻译要求：
1. 自然流畅：使用口语化表达，符合中文字幕习惯
2. 准确传神：忠实原文含义，保持语气和情感
3. 简洁明了：字幕需要快速阅读，避免冗长
4. 数量严格：必须输出 %d 句翻译，不多不少
5. 分隔符：每句翻译用"###SENTENCE_BREAK###"分隔

输入格式：句子用"###SENTENCE_BREAK###"分隔
输出格式：只返回中文翻译，用"###SENTENCE_BREAK###"分隔

注意：只返回翻译的中文文本，不要添加序号、解释或其他内容。`, len(texts), len(texts))

	translatedText, err := t.callDeepSeekAPI(systemPrompt, combinedText)
	if err != nil {
		return nil, err
	}

	translatedSentences := strings.Split(translatedText, "###SENTENCE_BREAK###")

	// 清理和验证
	for i := range translatedSentences {
		translatedSentences[i] = strings.TrimSpace(translatedSentences[i])
	}

	// 确保数量匹配
	if len(translatedSentences) != len(texts) {
		t.App.Logger.Warnf("⚠️  翻译结果数量不匹配: 期望%d句，实际%d句，正在修正...", len(texts), len(translatedSentences))
		for len(translatedSentences) < len(texts) {
			translatedSentences = append(translatedSentences, "[翻译缺失]")
		}
		if len(translatedSentences) > len(texts) {
			translatedSentences = translatedSentences[:len(texts)]
		}
	}

	return translatedSentences, nil
}

// translateTextsInGroups 分组翻译文本（带上下文）- 保留原方法作为备用
func (t *TranslateSubtitle) translateTextsInGroups(texts []string) ([]string, error) {
	var translatedTexts []string
	totalGroups := (len(texts) + t.GroupSize - 1) / t.GroupSize

	for i := 0; i < len(texts); i += t.GroupSize {
		groupNum := (i / t.GroupSize) + 1
		end := i + t.GroupSize
		if end > len(texts) {
			end = len(texts)
		}

		currentGroup := texts[i:end]

		// 准备上下文窗口
		var prevContext, nextContext []string
		contextSize := 2 // 前后各取2句作为上下文

		// 获取前置上下文
		if i > 0 {
			prevStart := i - contextSize
			if prevStart < 0 {
				prevStart = 0
			}
			prevContext = texts[prevStart:i]
		}

		// 获取后置上下文
		if end < len(texts) {
			nextEnd := end + contextSize
			if nextEnd > len(texts) {
				nextEnd = len(texts)
			}
			nextContext = texts[end:nextEnd]
		}

		t.App.Logger.Infof("⏳ 翻译第 %d/%d 组 (上下文: 前%d句, 当前%d句, 后%d句)",
			groupNum, totalGroups, len(prevContext), len(currentGroup), len(nextContext))

		// 带上下文翻译
		groupTranslated, err := t.translateGroupWithContext(currentGroup, prevContext, nextContext)
		if err != nil {
			return nil, fmt.Errorf("翻译第 %d 组失败: %v", groupNum, err)
		}

		translatedTexts = append(translatedTexts, groupTranslated...)

		// 移除组间延迟，改为根据需要动态调整
		// 如果遇到API限制，可以在错误处理中添加重试和延迟
	}

	return translatedTexts, nil
}

// translateGroupWithContext 带上下文翻译一组文本
func (t *TranslateSubtitle) translateGroupWithContext(texts []string, prevContext []string, nextContext []string) ([]string, error) {
	// 构建包含上下文的完整文本
	var fullTexts []string
	targetStartIndex := 0

	// 添加前置上下文
	if len(prevContext) > 0 {
		fullTexts = append(fullTexts, prevContext...)
		targetStartIndex = len(fullTexts)
	}

	// 添加目标翻译文本
	fullTexts = append(fullTexts, texts...)
	targetEndIndex := len(fullTexts)

	// 添加后置上下文
	if len(nextContext) > 0 {
		fullTexts = append(fullTexts, nextContext...)
	}

	combinedText := strings.Join(fullTexts, "\n###SENTENCE_BREAK###\n")

	// 构建系统提示
	contextInfo := ""
	if len(prevContext) > 0 || len(nextContext) > 0 {
		contextInfo = fmt.Sprintf(`

上下文信息：
- 前置上下文：%d 句（仅供参考，不需要翻译）
- 目标翻译：%d 句（位于第 %d-%d 句，需要全部翻译）
- 后置上下文：%d 句（仅供参考，不需要翻译）

请只翻译目标部分（第 %d-%d 句），但要充分考虑前后文的连贯性。`,
			len(prevContext), len(texts), targetStartIndex+1, targetEndIndex,
			len(nextContext), targetStartIndex+1, targetEndIndex)
	}

	systemPrompt := fmt.Sprintf(`你是一个专业的视频字幕翻译专家。我将给你一段连续的英文字幕，其中包含 %d 句需要翻译的内容。%s

翻译要求：
1. 自然流畅：使用口语化表达，符合中文字幕习惯
2. 上下文连贯：理解整体语境，确保翻译前后呼应
3. 准确传神：忠实原文含义，保持语气和情感
4. 简洁明了：字幕需要快速阅读，避免冗长
5. 数量严格：必须输出 %d 句翻译，不多不少
6. 分隔符：每句翻译用"###SENTENCE_BREAK###"分隔

输入格式：句子用"###SENTENCE_BREAK###"分隔
输出格式：只返回目标部分的中文翻译，用"###SENTENCE_BREAK###"分隔

注意：只返回翻译的中文文本，不要添加序号、解释或其他内容。`, len(texts), contextInfo, len(texts))

	translatedText, err := t.callDeepSeekAPI(systemPrompt, combinedText)
	if err != nil {
		return nil, err
	}

	translatedSentences := strings.Split(translatedText, "###SENTENCE_BREAK###")

	// 清理和验证
	for i := range translatedSentences {
		translatedSentences[i] = strings.TrimSpace(translatedSentences[i])
	}

	// 确保数量匹配
	if len(translatedSentences) != len(texts) {
		t.App.Logger.Warnf("⚠️  翻译结果数量不匹配: 期望%d句，实际%d句，正在修正...", len(texts), len(translatedSentences))
		for len(translatedSentences) < len(texts) {
			translatedSentences = append(translatedSentences, "[翻译缺失]")
		}
		if len(translatedSentences) > len(texts) {
			translatedSentences = translatedSentences[:len(texts)]
		}
	}

	return translatedSentences, nil
}

// callDeepSeekAPI 调用DeepSeek API（实时获取最新的API Key）
func (t *TranslateSubtitle) callDeepSeekAPI(systemPrompt, userPrompt string) (string, error) {
	// 实时从配置中获取最新的API Key
	currentAPIKey, err := t.getCurrentAPIKey()
	if err != nil {
		return "", err
	}

	// 添加调试日志，显示当前使用的API Key（用于验证热更新是否生效）
	t.App.Logger.Debugf("🔑 当前使用API Key: %s", maskAPIKey(currentAPIKey))

	if t.RateLimiter == nil {
		t.RateLimiter = newAdaptiveRateLimiter(800*time.Millisecond, 350*time.Millisecond, 6*time.Second)
	}
	t.RateLimiter.Wait()

	client := NewDeepSeekClientWithConfig(currentAPIKey, t.App.Config)
	response, err := client.ChatCompletion(systemPrompt, userPrompt)
	if err != nil {
		interval := t.RateLimiter.ReportFailure(err)
		if interval > 0 {
			t.App.Logger.Warnf("⚠️  DeepSeek请求失败，限速间隔调整为 %v: %v", interval, err)
		}
		return "", fmt.Errorf("调用DeepSeek API失败: %v", err)
	}
	interval := t.RateLimiter.ReportSuccess()
	t.App.Logger.Debugf("✅ DeepSeek请求成功，当前限速间隔: %v", interval)

	return response, nil
}

// getTranslationError 将翻译错误转换为用户友好的错误信息
func (t *TranslateSubtitle) getTranslationError(err error) string {
	errorStr := err.Error()

	if strings.Contains(errorStr, "DeepSeek API Key 未配置") {
		return "翻译失败：DeepSeek API Key未配置，请在设置中配置API Key"
	}

	if strings.Contains(errorStr, "401") || strings.Contains(errorStr, "unauthorized") {
		return "翻译失败：DeepSeek API Key无效或已过期，请检查API Key设置"
	}

	if strings.Contains(errorStr, "429") || strings.Contains(errorStr, "rate limit") {
		return "翻译失败：API调用频率过快，请稍后重试"
	}

	if strings.Contains(errorStr, "insufficient_quota") || strings.Contains(errorStr, "quota") {
		return "翻译失败：DeepSeek账户余额不足，请充值后重试"
	}

	if strings.Contains(errorStr, "timeout") || strings.Contains(errorStr, "deadline exceeded") {
		return "翻译失败：网络超时，请检查网络连接后重试"
	}

	if strings.Contains(errorStr, "connection") {
		return "翻译失败：网络连接异常，请检查网络状态"
	}

	if strings.Contains(errorStr, "max_tokens") {
		return "翻译失败：字幕内容过长，请尝试分段处理"
	}

	if strings.Contains(errorStr, "context_length_exceeded") {
		return "翻译失败：单次翻译内容过多，系统将自动分批重试"
	}

	if strings.Contains(errorStr, "API Key") {
		return "翻译失败：API Key配置问题，请检查设置"
	}

	// 通用翻译错误
	return "翻译失败：AI翻译服务暂时不可用，请稍后重试"
}

// maskAPIKey 隐藏API Key的敏感信息用于日志显示
func maskAPIKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}
	if len(apiKey) > 10 {
		return apiKey[:6] + "..." + apiKey[len(apiKey)-4:]
	}
	return "***"
}

// validateAndOptimizeSubtitles 校验和优化字幕质量
func (t *TranslateSubtitle) validateAndOptimizeSubtitles(originalPath, translatedPath string) (string, *utils.ValidationResult, error) {
	// 获取当前API Key用于修复
	apiKey, err := t.getCurrentAPIKey()
	if err != nil {
		return "", nil, fmt.Errorf("无法获取API Key进行校验: %v", err)
	}

	// 创建校验器
	validator := utils.NewSubtitleValidator(t.App.Logger, apiKey)

	// 生成优化后的文件路径
	optimizedPath := filepath.Join(t.StateManager.CurrentDir, "zh_optimized.srt")

	// 执行校验和修复
	result, err := validator.ValidateAndFixSubtitles(originalPath, translatedPath, optimizedPath)
	if err != nil {
		return "", nil, err
	}

	// 如果有修复，返回优化文件路径
	if len(result.FixedEntries) > 0 {
		return optimizedPath, result, nil
	}

	// 没有问题或无法修复，返回空路径
	return "", result, nil
}
