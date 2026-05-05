package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAICompatibleClient OpenAI兼容API客户端
// 支持任何兼容OpenAI API格式的服务，如：
// - OpenAI (https://api.openai.com/v1)
// - DeepSeek (https://api.deepseek.com/v1)
// - 通义千问 (https://dashscope.aliyuncs.com/compatible-mode/v1)
// - 智谱AI (https://open.bigmodel.cn/api/paas/v4/)
// - one-api/new-api 代理
// - Gemini代理 (如 clawcloudrun.com)
type OpenAICompatibleClient struct {
	APIKey      string
	BaseURL     string
	Model       string
	Client      *http.Client
	MaxRetries  int
	RetryDelay  time.Duration
	Temperature float64
	MaxTokens   int
}

// OpenAIRequest OpenAI格式请求结构
type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Stream      bool            `json:"stream"`
	Temperature float64         `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
}

// OpenAIMessage 消息结构
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIResponse OpenAI格式响应结构
type OpenAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
	Error   *OpenAIError   `json:"error,omitempty"`
}

// OpenAIChoice 选择结构
type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

// OpenAIUsage 使用量统计
type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAIError 错误结构
type OpenAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// OpenAIClientConfig 客户端配置
type OpenAIClientConfig struct {
	APIKey      string
	BaseURL     string
	Model       string
	Timeout     int
	MaxRetries  int
	Temperature float64
	MaxTokens   int
}

// NewOpenAICompatibleClient 创建OpenAI兼容客户端
func NewOpenAICompatibleClient(config *OpenAIClientConfig) *OpenAICompatibleClient {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.openai.com/v1"
	}
	if config.Model == "" {
		config.Model = "gpt-3.5-turbo"
	}
	if config.Timeout <= 0 {
		config.Timeout = 60
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.Temperature <= 0 {
		config.Temperature = 0.7
	}
	if config.MaxTokens <= 0 {
		config.MaxTokens = 4000
	}

	// 确保BaseURL以/v1结尾（如果不是完整的chat/completions路径）
	baseURL := strings.TrimSuffix(config.BaseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1") && !strings.Contains(baseURL, "/chat/completions") {
		baseURL = baseURL + "/v1"
	}

	return &OpenAICompatibleClient{
		APIKey:      config.APIKey,
		BaseURL:     baseURL,
		Model:       config.Model,
		MaxRetries:  config.MaxRetries,
		RetryDelay:  2 * time.Second,
		Temperature: config.Temperature,
		MaxTokens:   config.MaxTokens,
		Client: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}
}

// ChatCompletion 执行对话补全（带重试机制）
func (c *OpenAICompatibleClient) ChatCompletion(systemPrompt, userPrompt string) (string, error) {
	var lastErr error

	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(c.RetryDelay * time.Duration(attempt))
		}

		result, err := c.doRequest(systemPrompt, userPrompt)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// 如果是API限制错误，延长等待时间
		if strings.Contains(err.Error(), "rate limit") || strings.Contains(err.Error(), "429") {
			time.Sleep(time.Duration(attempt+1) * 5 * time.Second)
		}
	}

	return "", fmt.Errorf("重试 %d 次后仍然失败: %v", c.MaxRetries, lastErr)
}

// ChatCompletionWithUsage 执行对话补全并返回使用量统计
func (c *OpenAICompatibleClient) ChatCompletionWithUsage(systemPrompt, userPrompt string) (string, *OpenAIUsage, error) {
	messages := []OpenAIMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	response, err := c.doRequestWithMessages(messages)
	if err != nil {
		return "", nil, err
	}

	if len(response.Choices) == 0 {
		return "", nil, fmt.Errorf("API响应中没有结果")
	}

	return response.Choices[0].Message.Content, &response.Usage, nil
}

// TestConnection 测试API连接
func (c *OpenAICompatibleClient) TestConnection() error {
	_, err := c.ChatCompletion("You are a helpful assistant.", "Say 'OK' if you can hear me.")
	return err
}

// doRequest 执行单次API请求
func (c *OpenAICompatibleClient) doRequest(systemPrompt, userPrompt string) (string, error) {
	messages := []OpenAIMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	response, err := c.doRequestWithMessages(messages)
	if err != nil {
		return "", err
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("API响应中没有结果")
	}

	return response.Choices[0].Message.Content, nil
}

// doRequestWithMessages 执行带消息列表的请求
func (c *OpenAICompatibleClient) doRequestWithMessages(messages []OpenAIMessage) (*OpenAIResponse, error) {
	request := OpenAIRequest{
		Model:       c.Model,
		Messages:    messages,
		Stream:      false,
		Temperature: c.Temperature,
		MaxTokens:   c.MaxTokens,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %v", err)
	}

	// 构建完整的API URL
	apiURL := c.BaseURL
	if !strings.Contains(apiURL, "/chat/completions") {
		apiURL = strings.TrimSuffix(apiURL, "/") + "/chat/completions"
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	// 尝试解析响应
	var response OpenAIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v, 原始响应: %s", err, string(body))
	}

	// 检查API错误
	if response.Error != nil {
		return nil, fmt.Errorf("API错误 [%s]: %s", response.Error.Type, response.Error.Message)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API返回错误 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	return &response, nil
}

// GetSupportedProviders 获取支持的提供商列表及其默认配置
func GetSupportedProviders() map[string]OpenAIClientConfig {
	return map[string]OpenAIClientConfig{
		"openai": {
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-3.5-turbo",
		},
		"deepseek": {
			BaseURL: "https://api.deepseek.com/v1",
			Model:   "deepseek-chat",
		},
		"qwen": {
			BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
			Model:   "qwen-turbo",
		},
		"zhipu": {
			BaseURL: "https://open.bigmodel.cn/api/paas/v4/",
			Model:   "glm-4-flash",
		},
		"gemini": {
			BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai",
			Model:   "gemini-2.0-flash",
		},
		"custom": {
			BaseURL: "",
			Model:   "",
		},
	}
}
