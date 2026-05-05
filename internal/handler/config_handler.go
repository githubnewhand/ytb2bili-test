package handler

import (
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/types"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ConfigHandler struct {
	BaseHandler
}

func NewConfigHandler(app *core.AppServer) *ConfigHandler {
	return &ConfigHandler{
		BaseHandler: BaseHandler{App: app},
	}
}

// RegisterRoutes 注册配置相关路由
func (h *ConfigHandler) RegisterRoutes(server *core.AppServer) {
	api := server.Engine.Group("/api/v1")

	config := api.Group("/config")
	{
		config.GET("/deepseek", h.getDeepSeekConfig)
		config.PUT("/deepseek", h.updateDeepSeekConfig)
		config.GET("/proxy", h.getProxyConfig)
		config.PUT("/proxy", h.updateProxyConfig)
	}
}

// DeepSeekConfigRequest DeepSeek配置请求
type DeepSeekConfigRequest struct {
	Enabled   *bool   `json:"enabled,omitempty"`    // 是否启用（可选）
	ApiKey    *string `json:"api_key,omitempty"`    // API Key（可选）
	Model     *string `json:"model,omitempty"`      // 模型（可选）
	Endpoint  *string `json:"endpoint,omitempty"`   // 端点（可选）
	Timeout   *int    `json:"timeout,omitempty"`    // 超时时间（可选）
	MaxTokens *int    `json:"max_tokens,omitempty"` // 最大Token数（可选）
}

// DeepSeekConfigResponse DeepSeek配置响应
type DeepSeekConfigResponse struct {
	Enabled   bool   `json:"enabled"`
	ApiKey    string `json:"api_key"` // 为了安全只返回部分字符
	Model     string `json:"model"`
	Endpoint  string `json:"endpoint"`
	Timeout   int    `json:"timeout"`
	MaxTokens int    `json:"max_tokens"`
}

// ProxyConfigRequest 代理配置请求
type ProxyConfigRequest struct {
	UseProxy  *bool   `json:"useProxy,omitempty"`  // 是否使用代理（可选）
	ProxyHost *string `json:"proxyHost,omitempty"` // 代理地址（可选）
}

// ProxyConfigResponse 代理配置响应
type ProxyConfigResponse struct {
	UseProxy  bool   `json:"useProxy"`  // 是否使用代理
	ProxyHost string `json:"proxyHost"` // 代理地址
}

// getDeepSeekConfig 获取DeepSeek配置
func (h *ConfigHandler) getDeepSeekConfig(c *gin.Context) {
	config := h.App.Config.DeepSeekTransConfig
	if config == nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "success",
			"data": DeepSeekConfigResponse{
				Enabled:   false,
				ApiKey:    "",
				Model:     "deepseek-chat",
				Endpoint:  "https://api.deepseek.com",
				Timeout:   60,
				MaxTokens: 4000,
			},
		})
		return
	}

	// 隐藏完整的API Key，只显示前几位和后几位
	apiKeyMasked := ""
	if config.ApiKey != "" {
		if len(config.ApiKey) > 10 {
			apiKeyMasked = config.ApiKey[:6] + "..." + config.ApiKey[len(config.ApiKey)-4:]
		} else {
			apiKeyMasked = "***"
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": DeepSeekConfigResponse{
			Enabled:   config.Enabled,
			ApiKey:    apiKeyMasked,
			Model:     config.Model,
			Endpoint:  config.Endpoint,
			Timeout:   config.Timeout,
			MaxTokens: config.MaxTokens,
		},
	})
}

// updateDeepSeekConfig 更新DeepSeek配置
func (h *ConfigHandler) updateDeepSeekConfig(c *gin.Context) {
	var req DeepSeekConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	// 确保配置对象存在
	if h.App.Config.DeepSeekTransConfig == nil {
		h.App.Config.DeepSeekTransConfig = &types.DeepSeekTransConfig{
			Enabled:   false,
			ApiKey:    "",
			Model:     "deepseek-chat",
			Endpoint:  "https://api.deepseek.com",
			Timeout:   60,
			MaxTokens: 4000,
		}
	}

	config := h.App.Config.DeepSeekTransConfig

	// 更新配置字段（只更新提供的字段）
	if req.Enabled != nil {
		config.Enabled = *req.Enabled
		h.App.Logger.Infof("Updated DeepSeek enabled: %v", config.Enabled)
	}

	if req.ApiKey != nil {
		config.ApiKey = *req.ApiKey
		h.App.Logger.Infof("Updated DeepSeek API Key: %s", maskApiKey(*req.ApiKey))
	}

	if req.Model != nil {
		config.Model = *req.Model
		h.App.Logger.Infof("Updated DeepSeek model: %s", config.Model)
	}

	if req.Endpoint != nil {
		config.Endpoint = *req.Endpoint
		h.App.Logger.Infof("Updated DeepSeek endpoint: %s", config.Endpoint)
	}

	if req.Timeout != nil {
		config.Timeout = *req.Timeout
		h.App.Logger.Infof("Updated DeepSeek timeout: %d", config.Timeout)
	}

	if req.MaxTokens != nil {
		config.MaxTokens = *req.MaxTokens
		h.App.Logger.Infof("Updated DeepSeek max_tokens: %d", config.MaxTokens)
	}

	// 保存配置到文件
	if err := types.SaveConfig(h.App.Config); err != nil {
		h.App.Logger.Errorf("Failed to save config: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to save configuration: " + err.Error(),
		})
		return
	}

	// 实时更新应用服务器的配置（不需要重启）
	h.App.Config.DeepSeekTransConfig = config
	h.App.Logger.Info("✅ DeepSeek configuration updated and applied successfully (no restart required)")

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Configuration updated and applied successfully (no restart required)",
		"data": DeepSeekConfigResponse{
			Enabled:   config.Enabled,
			ApiKey:    maskApiKey(config.ApiKey),
			Model:     config.Model,
			Endpoint:  config.Endpoint,
			Timeout:   config.Timeout,
			MaxTokens: config.MaxTokens,
		},
	})
}

// getProxyConfig 获取代理配置
func (h *ConfigHandler) getProxyConfig(c *gin.Context) {
	// 检查配置中是否有代理配置
	proxyConfig := h.App.Config.ProxyConfig
	if proxyConfig == nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "success",
			"data": ProxyConfigResponse{
				UseProxy:  false,
				ProxyHost: "",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": ProxyConfigResponse{
			UseProxy:  proxyConfig.UseProxy,
			ProxyHost: proxyConfig.ProxyHost,
		},
	})
}

// updateProxyConfig 更新代理配置
func (h *ConfigHandler) updateProxyConfig(c *gin.Context) {
	var req ProxyConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	// 确保配置对象存在
	if h.App.Config.ProxyConfig == nil {
		h.App.Config.ProxyConfig = &types.ProxyConfig{
			UseProxy:  false,
			ProxyHost: "",
		}
	}

	config := h.App.Config.ProxyConfig

	// 更新配置字段（只更新提供的字段）
	if req.UseProxy != nil {
		config.UseProxy = *req.UseProxy
		h.App.Logger.Infof("Updated proxy enabled: %v", config.UseProxy)
	}

	if req.ProxyHost != nil {
		config.ProxyHost = *req.ProxyHost
		h.App.Logger.Infof("Updated proxy host: %s", config.ProxyHost)
	}

	// 保存配置到文件
	if err := types.SaveConfig(h.App.Config); err != nil {
		h.App.Logger.Errorf("Failed to save config: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to save configuration: " + err.Error(),
		})
		return
	}

	// 实时更新应用服务器的配置（不需要重启）
	h.App.Config.ProxyConfig = config
	h.App.Logger.Info("✅ Proxy configuration updated and applied successfully (no restart required)")

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Configuration updated and applied successfully (no restart required)",
		"data": ProxyConfigResponse{
			UseProxy:  config.UseProxy,
			ProxyHost: config.ProxyHost,
		},
	})
}

// maskApiKey 隐藏API Key的敏感信息
func maskApiKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}
	if len(apiKey) > 10 {
		return apiKey[:6] + "..." + apiKey[len(apiKey)-4:]
	}
	return "***"
}
