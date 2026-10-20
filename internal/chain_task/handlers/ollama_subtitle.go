package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/difyz9/ytb2bili/internal/core/types"
)

func subtitleTranslationProvider(config *types.AppConfig) string {
	if config == nil || config.SubtitleTranslationConfig == nil || config.SubtitleTranslationConfig.Provider == "" {
		return "deepseek"
	}
	return strings.ToLower(strings.TrimSpace(config.SubtitleTranslationConfig.Provider))
}

func ollamaSubtitleSettings(config *types.AppConfig) (string, string, time.Duration, error) {
	endpoint := "http://127.0.0.1:11434"
	model := "qwen3:4b-instruct"
	timeout := 180 * time.Second
	if config != nil && config.SubtitleTranslationConfig != nil {
		c := config.SubtitleTranslationConfig
		if c.OllamaURL != "" {
			endpoint = strings.TrimRight(c.OllamaURL, "/")
		}
		if c.OllamaModel != "" {
			model = c.OllamaModel
		}
		if c.Timeout > 0 {
			timeout = time.Duration(c.Timeout) * time.Second
		}
	}
	return validatedOllamaSettings(endpoint, model, timeout)
}

func ollamaMetadataSettings(config *types.AppConfig) (string, string, time.Duration, error) {
	endpoint := "http://127.0.0.1:11434"
	model := "qwen3:4b-instruct"
	timeout := 180 * time.Second
	if config != nil && config.MetadataGenerationConfig != nil {
		c := config.MetadataGenerationConfig
		if c.OllamaURL != "" {
			endpoint = strings.TrimRight(c.OllamaURL, "/")
		}
		if c.OllamaModel != "" {
			model = c.OllamaModel
		}
		if c.Timeout > 0 {
			timeout = time.Duration(c.Timeout) * time.Second
		}
	}
	return validatedOllamaSettings(endpoint, model, timeout)
}

func validatedOllamaSettings(endpoint, model string, timeout time.Duration) (string, string, time.Duration, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" {
		return "", "", 0, fmt.Errorf("Ollama 地址必须是本机 HTTP 根地址")
	}
	if ip := net.ParseIP(parsed.Hostname()); parsed.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return "", "", 0, fmt.Errorf("Ollama 地址必须指向本机，避免内容发往外部服务")
	}
	return endpoint, model, timeout, nil
}

// checkOllamaReady fails before work is queued when the local service or model is unavailable.
func checkOllamaReady(config *types.AppConfig) error {
	endpoint, model, _, err := ollamaSubtitleSettings(config)
	if err != nil {
		return err
	}
	return checkOllamaReadyAt(endpoint, model)
}

func checkOllamaReadyAt(endpoint, model string) error {
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(endpoint + "/api/tags")
	if err != nil {
		return fmt.Errorf("本机 Ollama 未运行: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("本机 Ollama 状态异常: HTTP %d", resp.StatusCode)
	}
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tags); err != nil {
		return fmt.Errorf("读取 Ollama 模型列表失败: %w", err)
	}
	for _, entry := range tags.Models {
		if entry.Name == model {
			return nil
		}
	}
	return fmt.Errorf("本机 Ollama 缺少模型 %s，请先下载", model)
}

func (t *TranslateSubtitle) callSubtitleTranslationAPI(systemPrompt, userPrompt string) (string, error) {
	switch subtitleTranslationProvider(t.App.Config) {
	case "deepseek":
		return t.callDeepSeekAPI(systemPrompt, userPrompt)
	case "ollama":
		return t.callOllamaAPI(systemPrompt, userPrompt)
	default:
		return "", fmt.Errorf("未知字幕翻译服务：%s", subtitleTranslationProvider(t.App.Config))
	}
}

func (t *TranslateSubtitle) callOllamaAPI(systemPrompt, userPrompt string) (string, error) {
	endpoint, model, timeout, err := ollamaSubtitleSettings(t.App.Config)
	if err != nil {
		return "", err
	}
	return callOllamaJSON(endpoint, model, timeout, systemPrompt, userPrompt)
}

func callOllamaJSON(endpoint, model string, timeout time.Duration, systemPrompt, userPrompt string) (string, error) {
	payload := map[string]interface{}{
		"model":      model,
		"stream":     false,
		"think":      false,
		"format":     "json",
		"keep_alive": "5m",
		"options":    map[string]interface{}{"temperature": 0, "num_ctx": 4096},
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	// This client never reads HTTP(S)_PROXY or the project's external proxy.
	client := &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("连接本机 Ollama 失败: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ollama HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return "", fmt.Errorf("解析 Ollama 响应失败: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("Ollama: %s", result.Error)
	}
	if strings.TrimSpace(result.Message.Content) == "" {
		return "", fmt.Errorf("Ollama 返回空译文")
	}
	return result.Message.Content, nil
}
