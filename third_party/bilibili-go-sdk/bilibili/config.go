// Package bilibili provides a Go SDK for Bilibili API
package bilibili

import (
	"net/http"
	"net/url"
	"time"
)

// Config SDK??
type Config struct {
	HTTPClient     *http.Client
	UserAgent      string
	Timeout        time.Duration
	ProxyURL       string
	UploadProgress UploadProgressCallback
}

// Option SDK????
type Option func(*Config)

// UploadProgress 上传进度信息
type UploadProgress struct {
	UploadedBytes int64
	TotalBytes    int64
	ChunkIndex    int
	TotalChunks   int
	Percent       float64
}

// UploadProgressCallback 上传进度回调
type UploadProgressCallback func(UploadProgress)

// WithHTTPClient ?????HTTP???
func WithHTTPClient(client *http.Client) Option {
	return func(c *Config) {
		c.HTTPClient = client
	}
}

// WithUserAgent ??User-Agent
func WithUserAgent(ua string) Option {
	return func(c *Config) {
		c.UserAgent = ua
	}
}

// WithTimeout ????????
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

// WithProxy ????
func WithProxy(proxyURL string) Option {
	return func(c *Config) {
		c.ProxyURL = proxyURL
	}
}

// WithUploadProgress 设置上传进度回调
func WithUploadProgress(callback UploadProgressCallback) Option {
	return func(c *Config) {
		c.UploadProgress = callback
	}
}

// DefaultConfig ????
func DefaultConfig() *Config {
	return &Config{
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		UserAgent: "Mozilla/5.0 BiliDroid/7.80.0 (bbcallen@gmail.com) os/android model/MI 6 mobi_app/android build/7800300 channel/bili innerVer/7800310 osVer/13 network/2",
		Timeout:   30 * time.Second,
	}
}

func applyProxyTransport(client *http.Client, proxyURL string) {
	if client == nil || proxyURL == "" {
		return
	}

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if existing, ok := client.Transport.(*http.Transport); ok && existing != nil {
		transport = existing.Clone()
	}

	transport.Proxy = http.ProxyURL(parsedURL)
	client.Transport = transport
}

// ApplyOptions ??????
func (c *Config) ApplyOptions(opts ...Option) {
	for _, opt := range opts {
		opt(c)
	}

	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{}
	}

	// ???????HTTP???
	c.HTTPClient.Timeout = c.Timeout
	applyProxyTransport(c.HTTPClient, c.ProxyURL)
}
