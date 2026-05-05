package utils

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
)

// SubtitleValidator 字幕校验和优化器
type SubtitleValidator struct {
	logger        *zap.SugaredLogger
	apiKey        string
	maxRetries    int
	retryInterval time.Duration
}

// SubtitleEntry 字幕条目
type SubtitleEntry struct {
	Index      int
	TimeCode   string
	Original   string // 原始英文
	Translated string // 翻译中文
	Status     string // 状态: "ok", "missing", "incomplete", "error"
}

// ValidationResult 校验结果
type ValidationResult struct {
	TotalEntries   int             `json:"total_entries"`
	ValidEntries   int             `json:"valid_entries"`
	MissingEntries int             `json:"missing_entries"`
	ErrorEntries   []int           `json:"error_entries"`
	FixedEntries   []int           `json:"fixed_entries"`
	IssueDetails   map[int]string  `json:"issue_details"`
	ProcessingTime time.Duration   `json:"processing_time"`
	Entries        []SubtitleEntry `json:"entries"`
}

// NewSubtitleValidator 创建字幕校验器
func NewSubtitleValidator(logger *zap.SugaredLogger, apiKey string) *SubtitleValidator {
	return &SubtitleValidator{
		logger:        logger,
		apiKey:        apiKey,
		maxRetries:    3,
		retryInterval: 2 * time.Second,
	}
}

// ValidateAndFixSubtitles 校验并修复字幕文件
func (v *SubtitleValidator) ValidateAndFixSubtitles(originalSRTPath, translatedSRTPath, outputPath string) (*ValidationResult, error) {
	startTime := time.Now()
	v.logger.Info("🔍 开始字幕校验和优化...")

	// 1. 读取原始英文字幕
	originalEntries, err := v.parseSRTFile(originalSRTPath)
	if err != nil {
		return nil, fmt.Errorf("读取原始字幕文件失败: %v", err)
	}

	// 2. 读取翻译后的中文字幕
	translatedEntries, err := v.parseSRTFile(translatedSRTPath)
	if err != nil {
		return nil, fmt.Errorf("读取翻译字幕文件失败: %v", err)
	}

	v.logger.Infof("📊 原始字幕: %d 条，翻译字幕: %d 条", len(originalEntries), len(translatedEntries))

	// 3. 合并和分析
	entries := v.mergeAndAnalyzeEntries(originalEntries, translatedEntries)

	// 4. 创建校验结果
	result := &ValidationResult{
		TotalEntries: len(entries),
		IssueDetails: make(map[int]string),
		Entries:      entries,
	}

	// 5. 统计问题
	var problemEntries []SubtitleEntry
	for _, entry := range entries {
		switch entry.Status {
		case "ok":
			result.ValidEntries++
		case "missing", "incomplete":
			result.MissingEntries++
			problemEntries = append(problemEntries, entry)
			result.IssueDetails[entry.Index] = fmt.Sprintf("状态: %s, 内容: %s", entry.Status, entry.Translated)
		case "error":
			result.ErrorEntries = append(result.ErrorEntries, entry.Index)
			result.IssueDetails[entry.Index] = fmt.Sprintf("错误条目: %s", entry.Translated)
		}
	}

	v.logger.Infof("📋 分析结果: 有效 %d 条，问题 %d 条，错误 %d 条",
		result.ValidEntries, result.MissingEntries, len(result.ErrorEntries))

	// 6. 修复问题条目
	if len(problemEntries) > 0 {
		v.logger.Infof("🔧 开始修复 %d 个问题条目...", len(problemEntries))
		fixedEntries, err := v.fixProblemEntries(problemEntries)
		if err != nil {
			v.logger.Errorf("❌ 修复过程中发生错误: %v", err)
		} else {
			// 应用修复结果
			for _, fixed := range fixedEntries {
				for i, entry := range entries {
					if entry.Index == fixed.Index {
						entries[i] = fixed
						result.FixedEntries = append(result.FixedEntries, fixed.Index)
						break
					}
				}
			}
			v.logger.Infof("✅ 成功修复 %d 个条目", len(fixedEntries))
		}
	}

	// 7. 生成优化后的字幕文件
	if outputPath != "" {
		err = v.generateOptimizedSRT(entries, outputPath)
		if err != nil {
			v.logger.Errorf("❌ 生成优化字幕文件失败: %v", err)
		} else {
			v.logger.Infof("💾 已保存优化后的字幕: %s", outputPath)
		}
	}

	// 8. 更新统计信息
	result.ValidEntries = 0
	result.MissingEntries = 0
	result.ErrorEntries = []int{}
	for _, entry := range entries {
		switch entry.Status {
		case "ok":
			result.ValidEntries++
		case "missing", "incomplete":
			result.MissingEntries++
		case "error":
			result.ErrorEntries = append(result.ErrorEntries, entry.Index)
		}
	}

	result.ProcessingTime = time.Since(startTime)
	result.Entries = entries

	v.logger.Infof("🎉 字幕优化完成！最终统计: 有效 %d 条，剩余问题 %d 条，处理时间: %v",
		result.ValidEntries, result.MissingEntries, result.ProcessingTime)

	return result, nil
}

// parseSRTFile 解析SRT文件
func (v *SubtitleValidator) parseSRTFile(filePath string) ([]SubtitleEntry, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []SubtitleEntry
	scanner := bufio.NewScanner(file)

	var currentEntry SubtitleEntry
	var textLines []string
	stage := 0 // 0=等待序号, 1=等待时间码, 2=读取文本

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			// 空行表示一个条目结束
			if stage == 2 && len(textLines) > 0 {
				currentEntry.Translated = strings.Join(textLines, "\n")
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
				currentEntry = SubtitleEntry{Index: index}
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

	// 处理最后一个条目
	if stage == 2 && len(textLines) > 0 {
		currentEntry.Translated = strings.Join(textLines, "\n")
		entries = append(entries, currentEntry)
	}

	return entries, scanner.Err()
}

// mergeAndAnalyzeEntries 合并并分析原始和翻译字幕
func (v *SubtitleValidator) mergeAndAnalyzeEntries(original, translated []SubtitleEntry) []SubtitleEntry {
	// 创建原始字幕的映射
	originalMap := make(map[int]SubtitleEntry)
	for _, entry := range original {
		originalMap[entry.Index] = entry
	}

	var entries []SubtitleEntry

	// 基于翻译字幕进行合并分析
	for _, translatedEntry := range translated {
		entry := SubtitleEntry{
			Index:      translatedEntry.Index,
			TimeCode:   translatedEntry.TimeCode,
			Translated: translatedEntry.Translated,
		}

		// 查找对应的原始英文
		if originalEntry, exists := originalMap[translatedEntry.Index]; exists {
			entry.Original = originalEntry.Translated // 在原始文件中，Translated字段存储的是英文
		}

		// 分析翻译状态
		entry.Status = v.analyzeTranslationStatus(entry.Translated)

		entries = append(entries, entry)
	}

	return entries
}

// analyzeTranslationStatus 分析翻译状态
func (v *SubtitleValidator) analyzeTranslationStatus(text string) string {
	if text == "" {
		return "missing"
	}

	// 检测明确的翻译缺失标记
	missingPatterns := []string{
		"[翻译缺失]",
		"翻译缺失",
		"[Missing Translation]",
		"[MISSING]",
		"[缺失]",
	}

	for _, pattern := range missingPatterns {
		if strings.Contains(text, pattern) {
			return "missing"
		}
	}

	// 检测可能的不完整翻译
	incompletePatterns := []string{
		"...",
		"[",
		"]",
		"???",
		"XXX",
	}

	for _, pattern := range incompletePatterns {
		if strings.Contains(text, pattern) {
			return "incomplete"
		}
	}

	// 检测纯英文（可能翻译失败）
	if v.isPureEnglish(text) && len(text) > 10 {
		return "incomplete"
	}

	// 检测过短的翻译（可能不完整）
	if len(strings.TrimSpace(text)) < 2 {
		return "incomplete"
	}

	return "ok"
}

// isPureEnglish 检测是否为纯英文
func (v *SubtitleValidator) isPureEnglish(text string) bool {
	// 简单的中文字符检测
	chinesePattern := regexp.MustCompile(`[\p{Han}]`)
	return !chinesePattern.MatchString(text)
}

// fixProblemEntries 修复问题条目
func (v *SubtitleValidator) fixProblemEntries(problemEntries []SubtitleEntry) ([]SubtitleEntry, error) {
	if v.apiKey == "" {
		return nil, fmt.Errorf("API Key 未配置，无法进行自动修复")
	}

	var fixedEntries []SubtitleEntry
	batchSize := 10 // 每次修复10条

	for i := 0; i < len(problemEntries); i += batchSize {
		end := i + batchSize
		if end > len(problemEntries) {
			end = len(problemEntries)
		}

		batch := problemEntries[i:end]
		v.logger.Infof("🔧 修复第 %d-%d 条问题字幕...", i+1, end)

		fixedBatch, err := v.fixBatchEntries(batch)
		if err != nil {
			v.logger.Warnf("⚠️  批次修复失败: %v", err)
			// 继续处理其他批次
			continue
		}

		fixedEntries = append(fixedEntries, fixedBatch...)

		// 添加间隔避免API限制
		if end < len(problemEntries) {
			time.Sleep(v.retryInterval)
		}
	}

	return fixedEntries, nil
}

// fixBatchEntries 修复一批条目
func (v *SubtitleValidator) fixBatchEntries(entries []SubtitleEntry) ([]SubtitleEntry, error) {
	if len(entries) == 0 {
		return []SubtitleEntry{}, nil
	}

	// 准备翻译文本
	var englishTexts []string
	for _, entry := range entries {
		if entry.Original != "" {
			englishTexts = append(englishTexts, entry.Original)
		} else {
			// 如果没有原始英文，尝试从现有翻译中推断
			englishTexts = append(englishTexts, "[需要重新翻译]")
		}
	}

	// 构建修复提示
	systemPrompt := fmt.Sprintf(`你是专业的视频字幕翻译专家。现在需要重新翻译 %d 句有问题的英文字幕。

翻译要求：
1. 自然流畅：使用口语化表达，符合中文字幕习惯  
2. 准确传神：忠实原文含义，保持语气和情感
3. 简洁明了：字幕需要快速阅读，避免冗长
4. 完整输出：必须为每句英文提供完整的中文翻译
5. 数量严格：必须输出 %d 句翻译，不多不少
6. 分隔符：每句翻译用"###SENTENCE_BREAK###"分隔

注意：之前的翻译中可能有缺失或错误，请提供完整准确的重新翻译。

输出格式：只返回中文翻译，用"###SENTENCE_BREAK###"分隔，不要添加序号或其他内容。`,
		len(englishTexts), len(englishTexts))

	combinedText := strings.Join(englishTexts, "\n###SENTENCE_BREAK###\n")

	// 调用翻译API
	translatedText, err := v.callDeepSeekAPI(systemPrompt, combinedText)
	if err != nil {
		return nil, fmt.Errorf("调用翻译API失败: %v", err)
	}

	// 解析翻译结果
	translatedSentences := strings.Split(translatedText, "###SENTENCE_BREAK###")
	for i := range translatedSentences {
		translatedSentences[i] = strings.TrimSpace(translatedSentences[i])
	}

	// 确保数量匹配
	if len(translatedSentences) != len(entries) {
		v.logger.Warnf("⚠️  修复结果数量不匹配: 期望%d，实际%d", len(entries), len(translatedSentences))
		// 补齐或截断
		for len(translatedSentences) < len(entries) {
			translatedSentences = append(translatedSentences, "[修复失败]")
		}
		if len(translatedSentences) > len(entries) {
			translatedSentences = translatedSentences[:len(entries)]
		}
	}

	// 生成修复后的条目
	var fixedEntries []SubtitleEntry
	for i, entry := range entries {
		fixed := entry
		if i < len(translatedSentences) && translatedSentences[i] != "" {
			fixed.Translated = translatedSentences[i]
			fixed.Status = "ok" // 标记为已修复
		}
		fixedEntries = append(fixedEntries, fixed)
	}

	return fixedEntries, nil
}

// generateOptimizedSRT 生成优化后的SRT文件
func (v *SubtitleValidator) generateOptimizedSRT(entries []SubtitleEntry, outputPath string) error {
	// 确保输出目录存在
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %v", err)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %v", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	for _, entry := range entries {
		// 写入序号
		fmt.Fprintf(writer, "%d\n", entry.Index)

		// 写入时间码
		fmt.Fprintf(writer, "%s\n", entry.TimeCode)

		// 写入翻译内容
		fmt.Fprintf(writer, "%s\n\n", entry.Translated)
	}

	return nil
}

// GenerateValidationReport 生成校验报告
func (v *SubtitleValidator) GenerateValidationReport(result *ValidationResult, reportPath string) error {
	file, err := os.Create(reportPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	fmt.Fprintf(writer, "字幕校验报告\n")
	fmt.Fprintf(writer, "========================================\n")
	fmt.Fprintf(writer, "生成时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(writer, "处理时间: %v\n", result.ProcessingTime)
	fmt.Fprintf(writer, "\n")

	fmt.Fprintf(writer, "统计摘要:\n")
	fmt.Fprintf(writer, "- 总条目数: %d\n", result.TotalEntries)
	fmt.Fprintf(writer, "- 有效条目: %d (%.1f%%)\n", result.ValidEntries,
		float64(result.ValidEntries)/float64(result.TotalEntries)*100)
	fmt.Fprintf(writer, "- 问题条目: %d (%.1f%%)\n", result.MissingEntries,
		float64(result.MissingEntries)/float64(result.TotalEntries)*100)
	fmt.Fprintf(writer, "- 修复条目: %d\n", len(result.FixedEntries))
	fmt.Fprintf(writer, "\n")

	if len(result.IssueDetails) > 0 {
		fmt.Fprintf(writer, "问题详情:\n")
		for index, detail := range result.IssueDetails {
			fmt.Fprintf(writer, "- 条目 %d: %s\n", index, detail)
		}
		fmt.Fprintf(writer, "\n")
	}

	if len(result.FixedEntries) > 0 {
		fmt.Fprintf(writer, "修复的条目: %v\n", result.FixedEntries)
	}

	return nil
}

// DeepSeek API 相关结构体
type deepSeekRequest struct {
	Model    string            `json:"model"`
	Messages []deepSeekMessage `json:"messages"`
	Stream   bool              `json:"stream"`
	Settings *deepSeekSettings `json:"settings,omitempty"`
}

type deepSeekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type deepSeekSettings struct {
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
}

type deepSeekResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []deepSeekChoice `json:"choices"`
}

type deepSeekChoice struct {
	Index        int             `json:"index"`
	Message      deepSeekMessage `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

// callDeepSeekAPI 调用DeepSeek API
func (v *SubtitleValidator) callDeepSeekAPI(systemPrompt, userPrompt string) (string, error) {
	var lastErr error

	for attempt := 0; attempt <= v.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(v.retryInterval * time.Duration(attempt))
		}

		result, err := v.doRequest(systemPrompt, userPrompt)
		if err == nil {
			return result, nil
		}

		lastErr = err
		v.logger.Warnf("API调用失败 (尝试 %d/%d): %v", attempt+1, v.maxRetries+1, err)

		// 如果是API限制错误，延长等待时间
		if strings.Contains(err.Error(), "rate limit") || strings.Contains(err.Error(), "429") {
			time.Sleep(time.Duration(attempt+1) * 5 * time.Second)
		}
	}

	return "", fmt.Errorf("重试 %d 次后仍然失败: %v", v.maxRetries, lastErr)
}

// doRequest 执行单次API请求
func (v *SubtitleValidator) doRequest(systemPrompt, userPrompt string) (string, error) {
	request := deepSeekRequest{
		Model: "deepseek-chat",
		Messages: []deepSeekMessage{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
		Stream: false,
		Settings: &deepSeekSettings{
			Temperature: 0.3,
			MaxTokens:   4000,
		},
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %v", err)
	}

	req, err := http.NewRequest("POST", "https://api.deepseek.com/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", v.apiKey))

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API返回错误 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var response deepSeekResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("API响应中没有结果")
	}

	return response.Choices[0].Message.Content, nil
}
