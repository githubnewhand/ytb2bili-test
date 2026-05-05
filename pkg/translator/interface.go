package translator

import "context"

// TranslationRequest 翻译请求
type TranslationRequest struct {
	Text       string `json:"text" binding:"required"`       // 要翻译的文本
	SourceLang string `json:"sourceLang,omitempty"`          // 源语言，可选，auto为自动检测
	TargetLang string `json:"targetLang" binding:"required"` // 目标语言
	TextType   string `json:"textType,omitempty"`            // 文本类型：plain, html, markdown等
	Domain     string `json:"domain,omitempty"`              // 领域：general, medical, legal等
	ProjectId  string `json:"projectId,omitempty"`           // 项目ID（某些服务需要）
	Model      string `json:"model,omitempty"`               // 使用的模型（如Ollama）
}

// BatchTranslationRequest 批量翻译请求
type BatchTranslationRequest struct {
	Texts      []string `json:"texts" binding:"required"`      // 要翻译的文本列表
	SourceLang string   `json:"sourceLang,omitempty"`          // 源语言
	TargetLang string   `json:"targetLang" binding:"required"` // 目标语言
	TextType   string   `json:"textType,omitempty"`            // 文本类型
	Domain     string   `json:"domain,omitempty"`              // 领域
	ProjectId  string   `json:"projectId,omitempty"`           // 项目ID
	Model      string   `json:"model,omitempty"`               // 使用的模型
}

// TranslationResult 翻译结果
type TranslationResult struct {
	OriginalText   string  `json:"originalText"`        // 原文
	TranslatedText string  `json:"translatedText"`      // 译文
	SourceLang     string  `json:"sourceLang"`          // 检测到的源语言
	TargetLang     string  `json:"targetLang"`          // 目标语言
	Confidence     float64 `json:"confidence,omitempty"` // 置信度
	Provider       string  `json:"provider"`             // 翻译服务提供商
	Model          string  `json:"model,omitempty"`      // 使用的模型
	Usage          *Usage  `json:"usage,omitempty"`      // 使用统计
}

type TranslationResultDto struct {
	OriginalText   string  `json:"originalText"`        // 原文
	TranslatedText string  `json:"translatedText"`      // 译文
	SourceLang     string  `json:"sourceLang"`          // 检测到的源语言
	TargetLang     string  `json:"targetLang"`          // 目标语言
	Confidence     float64 `json:"confidence,omitempty"` // 置信度
}

func (t *TranslationResult) ToDto() TranslationResultDto {
	return TranslationResultDto{
		OriginalText:   t.OriginalText,
		TranslatedText: t.TranslatedText,
		SourceLang:     t.SourceLang,
		TargetLang:     t.TargetLang,
		Confidence:     t.Confidence,
	}
}

// BatchTranslationResult 批量翻译结果
type BatchTranslationResult struct {
	Results  []*TranslationResult `json:"results"`         // 翻译结果列表
	Provider string               `json:"provider"`        // 翻译服务提供商
	Usage    *Usage               `json:"usage,omitempty"` // 使用统计
}

// Usage 使用统计
type Usage struct {
	InputTokens  int     `json:"inputTokens,omitempty"`  // 输入token数
	OutputTokens int     `json:"outputTokens,omitempty"` // 输出token数
	TotalTokens  int     `json:"totalTokens,omitempty"`  // 总token数
	Characters   int     `json:"characters,omitempty"`    // 字符数
	Cost         float64 `json:"cost,omitempty"`          // 费用
	Duration     int64   `json:"duration,omitempty"`      // 耗时(毫秒)
}

// LanguageInfo 语言信息
type LanguageInfo struct {
	Code        string `json:"code"`        // 语言代码
	Name        string `json:"name"`        // 语言名称
	NativeName  string `json:"nativeName"`  // 本地名称
	Direction   string `json:"direction"`   // 文字方向：ltr, rtl
	IsSupported bool   `json:"isSupported"` // 是否支持
}

// TranslatorInfo 翻译器信息
type TranslatorInfo struct {
	Name               string   `json:"name"`               // 翻译器名称
	Provider           string   `json:"provider"`           // 提供商
	Version            string   `json:"version"`            // 版本
	MaxTextLength      int      `json:"maxTextLength"`      // 最大文本长度
	SupportedLanguages []string `json:"supportedLanguages"` // 支持的语言列表
	Features           []string `json:"features"`           // 支持的功能
	IsOnline           bool     `json:"isOnline"`           // 是否在线服务
}

// Translator 翻译器接口
type Translator interface {
	// Translate 单个文本翻译
	Translate(ctx context.Context, req *TranslationRequest) (*TranslationResult, error)

	// BatchTranslate 批量翻译
	BatchTranslate(ctx context.Context, req *BatchTranslationRequest) (*BatchTranslationResult, error)

	// GetSupportedLanguages 获取支持的语言列表
	GetSupportedLanguages(ctx context.Context) ([]LanguageInfo, error)

	// DetectLanguage 检测语言
	DetectLanguage(ctx context.Context, text string) (string, float64, error)

	// GetInfo 获取翻译器信息
	GetInfo() *TranslatorInfo

	// IsHealthy 健康检查
	IsHealthy(ctx context.Context) error
}

// TranslatorFactory 翻译器工厂接口
type TranslatorFactory interface {
	// CreateTranslator 创建翻译器实例
	CreateTranslator(provider string, config map[string]interface{}) (Translator, error)

	// GetSupportedProviders 获取支持的提供商列表
	GetSupportedProviders() []string
}
