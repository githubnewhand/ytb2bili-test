package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/difyz9/ytb2bili/internal/core/types"
)

// DeepSeekClient DeepSeek API客户端
type DeepSeekClient struct {
	APIKey     string
	BaseURL    string
	Model      string
	MaxTokens  int
	Client     *http.Client
	MaxRetries int
	RetryDelay time.Duration
}

// DeepSeekRequest API请求结构
type DeepSeekRequest struct {
	Model       string            `json:"model"`
	Messages    []DeepSeekMessage `json:"messages"`
	Stream      bool              `json:"stream"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature float64           `json:"temperature,omitempty"`
}

// DeepSeekMessage 消息结构
type DeepSeekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// DeepSeekSettings API设置
type DeepSeekSettings struct {
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
}

// DeepSeekResponse API响应结构
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

// DeepSeekUsage 使用量统计
type DeepSeekUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// NewDeepSeekClient 创建DeepSeek客户端
func NewDeepSeekClient(apiKey string) *DeepSeekClient {
	return NewDeepSeekClientWithConfig(apiKey, nil)
}

// NewDeepSeekClientWithConfig 创建DeepSeek客户端，并应用应用配置中的代理/端点/超时等设置
func NewDeepSeekClientWithConfig(apiKey string, config *types.AppConfig) *DeepSeekClient {
	baseURL := "https://api.deepseek.com/v1/chat/completions"
	model := "deepseek-chat"
	timeout := 60 * time.Second
	maxTokens := 4000

	if config != nil && config.DeepSeekTransConfig != nil {
		deepSeekConfig := config.DeepSeekTransConfig
		if deepSeekConfig.Endpoint != "" {
			baseURL = strings.TrimRight(deepSeekConfig.Endpoint, "/") + "/v1/chat/completions"
		}
		if deepSeekConfig.Model != "" {
			model = deepSeekConfig.Model
		}
		if deepSeekConfig.Timeout > 0 {
			timeout = time.Duration(deepSeekConfig.Timeout) * time.Second
		}
		if deepSeekConfig.MaxTokens > 0 {
			maxTokens = deepSeekConfig.MaxTokens
		}
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if config != nil && config.ProxyConfig != nil && config.ProxyConfig.UseProxy && config.ProxyConfig.ProxyHost != "" {
		if proxyURL, err := url.Parse(config.ProxyConfig.ProxyHost); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &DeepSeekClient{
		APIKey:     apiKey,
		BaseURL:    baseURL,
		Model:      model,
		MaxTokens:  maxTokens,
		MaxRetries: 3,
		RetryDelay: 2 * time.Second,
		Client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

// ChatCompletion 执行对话补全（带重试机制）
func (c *DeepSeekClient) ChatCompletion(systemPrompt, userPrompt string) (string, error) {
	var lastErr error

	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(c.retryDelayForError(lastErr, attempt))
		}

		result, err := c.doRequest(systemPrompt, userPrompt)
		if err == nil {
			return result, nil
		}

		lastErr = err

		if !c.shouldRetry(err) {
			return "", err
		}
	}

	return "", fmt.Errorf("重试 %d 次后仍然失败: %v", c.MaxRetries, lastErr)
}

func (c *DeepSeekClient) shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	errorStr := strings.ToLower(err.Error())

	nonRetryableMarkers := []string{
		"401",
		"403",
		"unauthorized",
		"forbidden",
		"invalid_api_key",
		"insufficient_quota",
		"quota",
		"context_length_exceeded",
		"invalid_request",
		"max_tokens",
	}
	for _, marker := range nonRetryableMarkers {
		if strings.Contains(errorStr, marker) {
			return false
		}
	}

	retryableMarkers := []string{
		"429",
		"rate limit",
		"500",
		"502",
		"503",
		"504",
		"timeout",
		"deadline exceeded",
		"connection",
		"connection reset",
		"temporary",
	}
	for _, marker := range retryableMarkers {
		if strings.Contains(errorStr, marker) {
			return true
		}
	}

	return false
}

func (c *DeepSeekClient) retryDelayForError(err error, attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	errorStr := ""
	if err != nil {
		errorStr = strings.ToLower(err.Error())
	}

	if strings.Contains(errorStr, "429") || strings.Contains(errorStr, "rate limit") {
		return time.Duration(attempt*attempt) * 10 * time.Second
	}

	delay := c.RetryDelay * time.Duration(1<<(attempt-1))
	maxDelay := 30 * time.Second
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

// doRequest 执行单次API请求
func (c *DeepSeekClient) doRequest(systemPrompt, userPrompt string) (string, error) {
	request := DeepSeekRequest{
		Model: c.Model,
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
		Stream:      false,
		Temperature: 0.3,
		MaxTokens:   c.MaxTokens,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %v", err)
	}

	req, err := http.NewRequest("POST", c.BaseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))

	resp, err := c.Client.Do(req)
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

	var response DeepSeekResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("API响应中没有结果")
	}

	return response.Choices[0].Message.Content, nil
}

// ChatCompletionWithUsage 执行对话补全并返回使用量统计
func (c *DeepSeekClient) ChatCompletionWithUsage(systemPrompt, userPrompt string) (string, *DeepSeekUsage, error) {
	request := DeepSeekRequest{
		Model: c.Model,
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
		MaxTokens: c.MaxTokens,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", nil, fmt.Errorf("序列化请求失败: %v", err)
	}

	req, err := http.NewRequest("POST", c.BaseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", nil, fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))

	resp, err := c.Client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("读取响应失败: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("API返回错误 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var response DeepSeekResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if len(response.Choices) == 0 {
		return "", nil, fmt.Errorf("API响应中没有结果")
	}

	return response.Choices[0].Message.Content, &response.Usage, nil
}
