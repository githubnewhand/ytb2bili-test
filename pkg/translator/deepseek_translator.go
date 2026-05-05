package translator

import (
	"github.com/difyz9/ytb2bili/internal/core/types"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DeepSeekTranslator DeepSeek翻译器实现
type DeepSeekTranslator struct {
	apiKey    string
	model     string
	endpoint  string
	timeout   int
	maxTokens int
	client    *http.Client
}

// DeepSeekRequest DeepSeek API请求结构
type DeepSeekRequest struct {
	Model     string            `json:"model"`
	Messages  []DeepSeekMessage `json:"messages"`
	Stream    bool              `json:"stream"`
	MaxTokens int               `json:"max_tokens,omitempty"`
}

// DeepSeekMessage DeepSeek消息结构
type DeepSeekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// DeepSeekResponse DeepSeek API响应结构
type DeepSeekResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []DeepSeekChoice `json:"choices"`
	Usage   DeepSeekUsage    `json:"usage"`
}

// DeepSeekChoice 选择结构
type DeepSeekChoice struct {
	Index        int             `json:"index"`
	Message      DeepSeekMessage `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

// DeepSeekUsage 使用统计
type DeepSeekUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// NewDeepSeekTranslator 创建DeepSeek翻译器实例
func NewDeepSeekTranslator(config *types.DeepSeekTransConfig) (*DeepSeekTranslator, error) {
	if config == nil {
		return nil, fmt.Errorf("deepseek translator config is nil")
	}

	if !config.Enabled {
		return nil, fmt.Errorf("deepseek translator is not enabled")
	}

	if config.ApiKey == "" {
		return nil, fmt.Errorf("deepseek API key is required")
	}

	// 设置默认值
	endpoint := config.Endpoint
	if endpoint == "" {
		endpoint = "https://api.deepseek.com"
	}

	model := config.Model
	if model == "" {
		model = "deepseek-chat"
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 60 // 默认60秒超时
	}

	maxTokens := config.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4000 // 默认4000 tokens
	}

	return &DeepSeekTranslator{
		apiKey:    config.ApiKey,
		model:     model,
		endpoint:  endpoint,
		timeout:   timeout,
		maxTokens: maxTokens,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}, nil
}

// Translate 单个文本翻译
func (d *DeepSeekTranslator) Translate(ctx context.Context, req *TranslationRequest) (*TranslationResult, error) {
	if req.Text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	startTime := time.Now()

	// 构建翻译提示词
	systemPrompt := d.buildSystemPrompt(req.SourceLang, req.TargetLang, req.TextType, req.Domain)
	userPrompt := req.Text

	// 调用DeepSeek API
	response, err := d.callDeepSeekAPI(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("deepseek API call failed: %w", err)
	}

	duration := time.Since(startTime).Milliseconds()

	// 检测源语言（如果未指定）
	sourceLang := req.SourceLang
	if sourceLang == "" {
		sourceLang = "auto" // DeepSeek会自动检测
	}

	// 构建使用统计
	usage := &Usage{
		InputTokens:  response.Usage.PromptTokens,
		OutputTokens: response.Usage.CompletionTokens,
		TotalTokens:  response.Usage.TotalTokens,
		Characters:   len(req.Text),
		Duration:     duration,
	}

	return &TranslationResult{
		OriginalText:   req.Text,
		TranslatedText: response.Choices[0].Message.Content,
		SourceLang:     sourceLang,
		TargetLang:     req.TargetLang,
		Provider:       "deepseek",
		Model:          d.model,
		Confidence:     0.95, // DeepSeek质量较高，设置较高置信度
		Usage:          usage,
	}, nil
}

// BatchTranslate 批量翻译
func (d *DeepSeekTranslator) BatchTranslate(ctx context.Context, req *BatchTranslationRequest) (*BatchTranslationResult, error) {
	if len(req.Texts) == 0 {
		return nil, fmt.Errorf("texts cannot be empty")
	}

	startTime := time.Now()
	results := make([]*TranslationResult, 0, len(req.Texts))
	totalUsage := &Usage{}

	// DeepSeek支持批量处理，我们将多个文本组合到一个请求中
	// 如果文本数量太多，分批处理
	const maxBatchSize = 20

	for i := 0; i < len(req.Texts); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(req.Texts) {
			end = len(req.Texts)
		}

		batchTexts := req.Texts[i:end]
		batchResults, err := d.translateBatch(ctx, batchTexts, req.SourceLang, req.TargetLang, req.TextType, req.Domain)
		if err != nil {
			return nil, fmt.Errorf("batch translation failed: %w", err)
		}

		results = append(results, batchResults...)

		// 累加使用统计
		for _, result := range batchResults {
			if result.Usage != nil {
				totalUsage.InputTokens += result.Usage.InputTokens
				totalUsage.OutputTokens += result.Usage.OutputTokens
				totalUsage.TotalTokens += result.Usage.TotalTokens
				totalUsage.Characters += result.Usage.Characters
			}
		}
	}

	totalUsage.Duration = time.Since(startTime).Milliseconds()

	return &BatchTranslationResult{
		Results:  results,
		Provider: "deepseek",
		Usage:    totalUsage,
	}, nil
}

// translateBatch 翻译一批文本
func (d *DeepSeekTranslator) translateBatch(ctx context.Context, texts []string, sourceLang, targetLang, textType, domain string) ([]*TranslationResult, error) {
	// 构建批量翻译提示词
	systemPrompt := d.buildBatchSystemPrompt(sourceLang, targetLang, textType, domain)

	// 将文本组合成编号格式
	var userPrompt strings.Builder
	userPrompt.WriteString("请翻译以下文本，保持原有的编号格式：\n\n")
	for i, text := range texts {
		userPrompt.WriteString(fmt.Sprintf("%d. %s\n", i+1, text))
	}

	// 调用DeepSeek API
	response, err := d.callDeepSeekAPI(ctx, systemPrompt, userPrompt.String())
	if err != nil {
		return nil, err
	}

	// 解析批量翻译结果
	translatedTexts := d.parseBatchResponse(response.Choices[0].Message.Content, len(texts))

	// 确保翻译结果数量匹配
	if len(translatedTexts) != len(texts) {
		// 如果批量翻译失败，降级为逐个翻译
		return d.fallbackToIndividualTranslation(ctx, texts, sourceLang, targetLang, textType, domain)
	}

	// 构建结果
	results := make([]*TranslationResult, len(texts))
	avgUsage := d.distributeUsage(response.Usage, len(texts))

	for i, text := range texts {
		results[i] = &TranslationResult{
			OriginalText:   text,
			TranslatedText: translatedTexts[i],
			SourceLang:     sourceLang,
			TargetLang:     targetLang,
			Provider:       "deepseek",
			Model:          d.model,
			Confidence:     0.95,
			Usage:          avgUsage,
		}
	}

	return results, nil
}

// fallbackToIndividualTranslation 降级为逐个翻译
func (d *DeepSeekTranslator) fallbackToIndividualTranslation(ctx context.Context, texts []string, sourceLang, targetLang, textType, domain string) ([]*TranslationResult, error) {
	results := make([]*TranslationResult, len(texts))

	for i, text := range texts {
		req := &TranslationRequest{
			Text:       text,
			SourceLang: sourceLang,
			TargetLang: targetLang,
			TextType:   textType,
			Domain:     domain,
		}

		result, err := d.Translate(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("failed to translate text %d: %w", i+1, err)
		}

		results[i] = result
	}

	return results, nil
}

// GetSupportedLanguages 获取支持的语言列表
func (d *DeepSeekTranslator) GetSupportedLanguages(ctx context.Context) ([]LanguageInfo, error) {
	// DeepSeek支持主流语言
	languages := []LanguageInfo{
		{Code: "zh", Name: "Chinese", NativeName: "中文", Direction: "ltr", IsSupported: true},
		{Code: "zh-cn", Name: "Chinese Simplified", NativeName: "简体中文", Direction: "ltr", IsSupported: true},
		{Code: "zh-tw", Name: "Chinese Traditional", NativeName: "繁體中文", Direction: "ltr", IsSupported: true},
		{Code: "en", Name: "English", NativeName: "English", Direction: "ltr", IsSupported: true},
		{Code: "ja", Name: "Japanese", NativeName: "日本語", Direction: "ltr", IsSupported: true},
		{Code: "ko", Name: "Korean", NativeName: "한국어", Direction: "ltr", IsSupported: true},
		{Code: "es", Name: "Spanish", NativeName: "Español", Direction: "ltr", IsSupported: true},
		{Code: "fr", Name: "French", NativeName: "Français", Direction: "ltr", IsSupported: true},
		{Code: "de", Name: "German", NativeName: "Deutsch", Direction: "ltr", IsSupported: true},
		{Code: "ru", Name: "Russian", NativeName: "Русский", Direction: "ltr", IsSupported: true},
		{Code: "it", Name: "Italian", NativeName: "Italiano", Direction: "ltr", IsSupported: true},
		{Code: "pt", Name: "Portuguese", NativeName: "Português", Direction: "ltr", IsSupported: true},
		{Code: "ar", Name: "Arabic", NativeName: "العربية", Direction: "rtl", IsSupported: true},
		{Code: "hi", Name: "Hindi", NativeName: "हिन्दी", Direction: "ltr", IsSupported: true},
		{Code: "th", Name: "Thai", NativeName: "ไทย", Direction: "ltr", IsSupported: true},
		{Code: "vi", Name: "Vietnamese", NativeName: "Tiếng Việt", Direction: "ltr", IsSupported: true},
	}
	return languages, nil
}

// DetectLanguage 检测语言
func (d *DeepSeekTranslator) DetectLanguage(ctx context.Context, text string) (string, float64, error) {
	if text == "" {
		return "", 0, fmt.Errorf("text cannot be empty")
	}

	systemPrompt := "你是一个语言检测专家。请检测给定文本的语言，并返回ISO 639-1语言代码（如'en'、'zh'、'ja'等）。只返回语言代码，不要其他说明。"

	response, err := d.callDeepSeekAPI(ctx, systemPrompt, text)
	if err != nil {
		return "", 0, fmt.Errorf("language detection failed: %w", err)
	}

	// 解析语言代码
	langCode := strings.TrimSpace(strings.ToLower(response.Choices[0].Message.Content))

	// 简单验证语言代码格式
	if len(langCode) < 2 || len(langCode) > 5 {
		langCode = "auto"
	}

	return langCode, 0.9, nil
}

// GetInfo 获取翻译器信息
func (d *DeepSeekTranslator) GetInfo() *TranslatorInfo {
	return &TranslatorInfo{
		Name:          fmt.Sprintf("DeepSeek AI Translator (%s)", d.model),
		Provider:      "deepseek",
		Version:       "1.0.0",
		MaxTextLength: 32000, // DeepSeek支持较长文本
		SupportedLanguages: []string{
			"zh", "zh-cn", "zh-tw", "en", "ja", "ko", "es", "fr", "de", "ru", "it", "pt", "ar", "hi", "th", "vi",
		},
		Features: []string{"translate", "batch_translate", "detect_language", "srt_translate"},
		IsOnline: true,
	}
}

// IsHealthy 健康检查
func (d *DeepSeekTranslator) IsHealthy(ctx context.Context) error {
	// 简单的健康检查：发送一个简短的翻译请求
	testReq := &TranslationRequest{
		Text:       "Hello",
		SourceLang: "en",
		TargetLang: "zh",
	}

	_, err := d.Translate(ctx, testReq)
	if err != nil {
		return fmt.Errorf("deepseek health check failed: %w", err)
	}

	return nil
}

// callDeepSeekAPI 调用DeepSeek API
func (d *DeepSeekTranslator) callDeepSeekAPI(ctx context.Context, systemPrompt, userPrompt string) (*DeepSeekResponse, error) {
	// 构建请求
	reqBody := DeepSeekRequest{
		Model: d.model,
		Messages: []DeepSeekMessage{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
		Stream:    false,
		MaxTokens: d.maxTokens,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建HTTP请求
	url := fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(d.endpoint, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+d.apiKey)

	// 发送请求
	resp, err := d.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deepseek API returned status %d", resp.StatusCode)
	}

	// 解析响应
	var response DeepSeekResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("no translation result in response")
	}

	return &response, nil
}

// buildSystemPrompt 构建系统提示词
func (d *DeepSeekTranslator) buildSystemPrompt(sourceLang, targetLang, textType, domain string) string {
	var prompt strings.Builder

	prompt.WriteString("你是一位专业的翻译专家。请将给定的文本进行准确、自然的翻译。\n\n")
	prompt.WriteString("翻译要求：\n")
	prompt.WriteString("1. 保持原文的意思和语调\n")
	prompt.WriteString("2. 使用自然流畅的目标语言表达\n")
	prompt.WriteString("3. 保留原文的格式和结构\n")
	prompt.WriteString("4. 对于专业术语，使用准确的对应词汇\n")

	if sourceLang != "" && sourceLang != "auto" {
		prompt.WriteString(fmt.Sprintf("5. 源语言：%s\n", d.getLanguageName(sourceLang)))
	}

	if targetLang != "" {
		prompt.WriteString(fmt.Sprintf("6. 目标语言：%s\n", d.getLanguageName(targetLang)))
	}

	if textType != "" {
		prompt.WriteString(fmt.Sprintf("7. 文本类型：%s\n", textType))
	}

	if domain != "" {
		prompt.WriteString(fmt.Sprintf("8. 领域：%s\n", domain))
	}

	prompt.WriteString("\n请直接返回翻译结果，不要包含任何解释或其他内容。")

	return prompt.String()
}

// buildBatchSystemPrompt 构建批量翻译系统提示词
func (d *DeepSeekTranslator) buildBatchSystemPrompt(sourceLang, targetLang, textType, domain string) string {
	var prompt strings.Builder

	prompt.WriteString("你是一位专业的翻译专家。请将以下编号的文本逐条翻译，保持相同的编号格式。\n\n")
	prompt.WriteString("翻译要求：\n")
	prompt.WriteString("1. 保持原文的意思和语调\n")
	prompt.WriteString("2. 使用自然流畅的目标语言表达\n")
	prompt.WriteString("3. 保留编号格式：1. 翻译内容\n")
	prompt.WriteString("4. 每个编号对应一行翻译结果\n")

	if sourceLang != "" && sourceLang != "auto" {
		prompt.WriteString(fmt.Sprintf("5. 源语言：%s\n", d.getLanguageName(sourceLang)))
	}

	if targetLang != "" {
		prompt.WriteString(fmt.Sprintf("6. 目标语言：%s\n", d.getLanguageName(targetLang)))
	}

	if textType != "" {
		prompt.WriteString(fmt.Sprintf("7. 文本类型：%s\n", textType))
	}

	if domain != "" {
		prompt.WriteString(fmt.Sprintf("8. 领域：%s\n", domain))
	}

	return prompt.String()
}

// getLanguageName 获取语言名称
func (d *DeepSeekTranslator) getLanguageName(code string) string {
	languageNames := map[string]string{
		"zh":    "中文",
		"zh-cn": "简体中文",
		"zh-tw": "繁体中文",
		"en":    "英语",
		"ja":    "日语",
		"ko":    "韩语",
		"es":    "西班牙语",
		"fr":    "法语",
		"de":    "德语",
		"ru":    "俄语",
		"it":    "意大利语",
		"pt":    "葡萄牙语",
		"ar":    "阿拉伯语",
		"hi":    "印地语",
		"th":    "泰语",
		"vi":    "越南语",
	}

	if name, exists := languageNames[code]; exists {
		return name
	}
	return code
}

// parseBatchResponse 解析批量翻译响应
func (d *DeepSeekTranslator) parseBatchResponse(response string, expectedCount int) []string {
	lines := strings.Split(strings.TrimSpace(response), "\n")
	results := make([]string, 0, expectedCount)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 尝试匹配编号格式：1. 内容
		parts := strings.SplitN(line, ". ", 2)
		if len(parts) == 2 {
			results = append(results, strings.TrimSpace(parts[1]))
		} else {
			// 如果没有编号，直接添加内容
			results = append(results, line)
		}
	}

	return results
}

// distributeUsage 分配使用统计
func (d *DeepSeekTranslator) distributeUsage(usage DeepSeekUsage, count int) *Usage {
	if count <= 0 {
		count = 1
	}

	return &Usage{
		InputTokens:  usage.PromptTokens / count,
		OutputTokens: usage.CompletionTokens / count,
		TotalTokens:  usage.TotalTokens / count,
	}
}
