package translator

import (
	"github.com/difyz9/ytb2bili/internal/core/types"
	"context"
	"fmt"
	"sync"
	"time"
)

// TranslatorManager 翻译器管理器
type TranslatorManager struct {
	config            *types.AppConfig
	factory           TranslatorFactory
	translators       map[string]Translator
	mutex             sync.RWMutex
	defaultProvider   string
	fallbackProviders []string
}

// NewTranslatorManager 创建翻译器管理器
func NewTranslatorManager(config *types.AppConfig) *TranslatorManager {
	factory := NewTranslatorFactory(config)

	defaultProvider := "tencent"
	fallbackProviders := []string{"microsoft", "google"}

	// 从配置中读取默认提供商和备选提供商
	if config.TranslatorConfig != nil {
		if config.TranslatorConfig.DefaultProvider != "" {
			defaultProvider = config.TranslatorConfig.DefaultProvider
		}
		if len(config.TranslatorConfig.FallbackProviders) > 0 {
			fallbackProviders = config.TranslatorConfig.FallbackProviders
		}
	}

	return &TranslatorManager{
		config:            config,
		factory:           factory,
		translators:       make(map[string]Translator),
		defaultProvider:   defaultProvider,
		fallbackProviders: fallbackProviders,
	}
}

// GetTranslator 获取翻译器实例
func (tm *TranslatorManager) GetTranslator(provider string) (Translator, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// 检查是否已存在实例
	if translator, exists := tm.translators[provider]; exists {
		return translator, nil
	}

	// 创建新的翻译器实例
	config := make(map[string]interface{})
	translator, err := tm.factory.CreateTranslator(provider, config)
	if err != nil {
		return nil, err
	}

	// 缓存实例
	tm.translators[provider] = translator
	return translator, nil
}

// GetDefaultTranslator 获取默认翻译器
func (tm *TranslatorManager) GetDefaultTranslator() (Translator, error) {
	return tm.GetTranslator(tm.defaultProvider)
}

// Translate 使用默认翻译器进行翻译
func (tm *TranslatorManager) Translate(ctx context.Context, req *TranslationRequest) (*TranslationResult, error) {
	return tm.TranslateWithProvider(ctx, tm.defaultProvider, req)
}

// TranslateWithProvider 使用指定提供商进行翻译
func (tm *TranslatorManager) TranslateWithProvider(ctx context.Context, provider string, req *TranslationRequest) (*TranslationResult, error) {
	translator, err := tm.GetTranslator(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get translator %s: %v", provider, err)
	}

	result, err := translator.Translate(ctx, req)
	if err != nil {
		// 如果主要提供商失败，尝试备选提供商
		return tm.translateWithFallback(ctx, req, err)
	}

	return result, nil
}

// translateWithFallback 使用备选提供商进行翻译
func (tm *TranslatorManager) translateWithFallback(ctx context.Context, req *TranslationRequest, originalErr error) (*TranslationResult, error) {
	for _, fallbackProvider := range tm.fallbackProviders {
		if fallbackProvider == tm.defaultProvider {
			continue // 跳过已经失败的默认提供商
		}

		translator, err := tm.GetTranslator(fallbackProvider)
		if err != nil {
			continue // 跳过无法创建的提供商
		}

		result, err := translator.Translate(ctx, req)
		if err == nil {
			return result, nil
		}
	}

	return nil, fmt.Errorf("all translators failed, original error: %v", originalErr)
}

// BatchTranslate 批量翻译
func (tm *TranslatorManager) BatchTranslate(ctx context.Context, req *BatchTranslationRequest) (*BatchTranslationResult, error) {
	return tm.BatchTranslateWithProvider(ctx, tm.defaultProvider, req)
}

// BatchTranslateWithProvider 使用指定提供商进行批量翻译
func (tm *TranslatorManager) BatchTranslateWithProvider(ctx context.Context, provider string, req *BatchTranslationRequest) (*BatchTranslationResult, error) {
	translator, err := tm.GetTranslator(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get translator %s: %v", provider, err)
	}

	return translator.BatchTranslate(ctx, req)
}

// GetSupportedLanguages 获取支持的语言列表
func (tm *TranslatorManager) GetSupportedLanguages(ctx context.Context, provider string) ([]LanguageInfo, error) {
	translator, err := tm.GetTranslator(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get translator %s: %v", provider, err)
	}

	return translator.GetSupportedLanguages(ctx)
}

// DetectLanguage 检测语言
func (tm *TranslatorManager) DetectLanguage(ctx context.Context, text string, provider string) (string, float64, error) {
	if provider == "" {
		provider = tm.defaultProvider
	}

	translator, err := tm.GetTranslator(provider)
	if err != nil {
		return "", 0, fmt.Errorf("failed to get translator %s: %v", provider, err)
	}

	return translator.DetectLanguage(ctx, text)
}

// GetProviderInfo 获取提供商信息
func (tm *TranslatorManager) GetProviderInfo(provider string) (*TranslatorInfo, error) {
	translator, err := tm.GetTranslator(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get translator %s: %v", provider, err)
	}

	return translator.GetInfo(), nil
}

// GetAllProviders 获取所有支持的提供商
func (tm *TranslatorManager) GetAllProviders() []string {
	return tm.factory.GetSupportedProviders()
}

// HealthCheck 健康检查
func (tm *TranslatorManager) HealthCheck(ctx context.Context, provider string) error {
	translator, err := tm.GetTranslator(provider)
	if err != nil {
		return fmt.Errorf("failed to get translator %s: %v", provider, err)
	}

	return translator.IsHealthy(ctx)
}

// HealthCheckAll 检查所有翻译器的健康状态
func (tm *TranslatorManager) HealthCheckAll(ctx context.Context) map[string]error {
	results := make(map[string]error)
	providers := tm.GetAllProviders()

	for _, provider := range providers {
		results[provider] = tm.HealthCheck(ctx, provider)
	}

	return results
}

// SmartTranslate 智能翻译，自动选择最佳提供商
func (tm *TranslatorManager) SmartTranslate(ctx context.Context, req *TranslationRequest) (*TranslationResult, error) {
	// 创建超时上下文
	timeout := 30 * time.Second
	if tm.config.TranslatorConfig != nil && tm.config.TranslatorConfig.Timeout > 0 {
		timeout = time.Duration(tm.config.TranslatorConfig.Timeout) * time.Second
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 首先尝试默认提供商
	result, err := tm.TranslateWithProvider(timeoutCtx, tm.defaultProvider, req)
	if err == nil {
		return result, nil
	}

	// 如果默认提供商失败，按顺序尝试备选提供商
	for _, provider := range tm.fallbackProviders {
		if provider == tm.defaultProvider {
			continue
		}

		result, err := tm.TranslateWithProvider(timeoutCtx, provider, req)
		if err == nil {
			return result, nil
		}
	}

	return nil, fmt.Errorf("all translation providers failed")
}
