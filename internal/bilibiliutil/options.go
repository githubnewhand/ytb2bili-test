package bilibiliutil

import (
	"time"

	"github.com/difyz9/bilibili-go-sdk/bilibili"
	coretypes "github.com/difyz9/ytb2bili/internal/core/types"
)

// BuildOptions builds shared timeout and proxy options for the Bilibili SDK.
func BuildOptions(config *coretypes.AppConfig, timeout time.Duration) []bilibili.Option {
	var opts []bilibili.Option
	if timeout > 0 {
		opts = append(opts, bilibili.WithTimeout(timeout))
	}

	if config != nil && config.ProxyConfig != nil && config.ProxyConfig.UseProxy && config.ProxyConfig.ProxyHost != "" {
		opts = append(opts, bilibili.WithProxy(config.ProxyConfig.ProxyHost))
	}

	return opts
}
