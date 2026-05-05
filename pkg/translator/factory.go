package translator

import (
	"github.com/difyz9/ytb2bili/internal/core/types"
	"fmt"
)

// Factory 翻译器工厂实现
type Factory struct {
	config *types.AppConfig
}

// NewTranslatorFactory 创建翻译器工厂
func NewTranslatorFactory(config *types.AppConfig) *Factory {
	return &Factory{
		config: config,
	}
}

// CreateTranslator 创建翻译器实例
func (f *Factory) CreateTranslator(provider string, config map[string]interface{}) (Translator, error) {
	switch provider {

	case "baidu":
		return f.createBaiduTranslator(config)
	case "deepseek":
		return f.createDeepSeekTranslator(config)
	default:
		return nil, fmt.Errorf("unsupported translator provider: %s", provider)
	}
}

// GetSupportedProviders 获取支持的提供商列表
func (f *Factory) GetSupportedProviders() []string {
	return []string{
		"tencent",
		"microsoft",
		"alibaba",
		"ollama",
		"google",
		"baidu",
		"deepseek",
	}
}

// createBaiduTranslator 创建百度翻译器
func (f *Factory) createBaiduTranslator(config map[string]interface{}) (Translator, error) {
	// 优先使用传入的配置，其次使用应用配置
	baiduConfig := f.config.BaiduTransConfig
	if baiduConfig == nil || !baiduConfig.Enabled {
		return nil, fmt.Errorf("baidu translator not enabled or config not found")
	}

	// 创建配置映射
	configMap := make(map[string]interface{})
	configMap["app_id"] = baiduConfig.AppId
	configMap["secret_key"] = baiduConfig.SecretKey
	configMap["endpoint"] = baiduConfig.Endpoint

	// 覆盖配置
	if appId, ok := config["app_id"].(string); ok && appId != "" {
		configMap["app_id"] = appId
	}
	if secretKey, ok := config["secret_key"].(string); ok && secretKey != "" {
		configMap["secret_key"] = secretKey
	}
	if endpoint, ok := config["endpoint"].(string); ok && endpoint != "" {
		configMap["endpoint"] = endpoint
	}

	return NewBaiduTranslator(configMap)
}

// createDeepSeekTranslator 创建DeepSeek翻译器
func (f *Factory) createDeepSeekTranslator(config map[string]interface{}) (Translator, error) {
	// 优先使用传入的配置，其次使用应用配置
	deepseekConfig := f.config.DeepSeekTransConfig
	if deepseekConfig == nil || !deepseekConfig.Enabled {
		return nil, fmt.Errorf("deepseek translator not enabled or config not found")
	}

	// 创建配置副本
	configCopy := *deepseekConfig

	// 覆盖配置
	if apiKey, ok := config["api_key"].(string); ok && apiKey != "" {
		configCopy.ApiKey = apiKey
	}
	if model, ok := config["model"].(string); ok && model != "" {
		configCopy.Model = model
	}
	if endpoint, ok := config["endpoint"].(string); ok && endpoint != "" {
		configCopy.Endpoint = endpoint
	}
	if timeout, ok := config["timeout"].(int); ok && timeout > 0 {
		configCopy.Timeout = timeout
	}
	if maxTokens, ok := config["max_tokens"].(int); ok && maxTokens > 0 {
		configCopy.MaxTokens = maxTokens
	}

	return NewDeepSeekTranslator(&configCopy)
}
