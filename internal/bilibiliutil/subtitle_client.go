package bilibiliutil

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/difyz9/bilibili-go-sdk/bilibili"
	coretypes "github.com/difyz9/ytb2bili/internal/core/types"
)

// Subtitle traffic prefers a direct connection to domestic Bilibili endpoints.
// Only read-only requests may fail over: an EOF on a POST can mean the server
// already accepted it, so replaying a subtitle submission is unsafe.
type subtitleTransport struct{ direct, fallback http.RoundTripper }

func (t *subtitleTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.direct.RoundTrip(req)
	if err == nil || t.fallback == nil || req.Context().Err() != nil ||
		(req.Method != http.MethodGet && req.Method != http.MethodHead) || !subtitleNetworkError(err) {
		return resp, err
	}
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	return t.fallback.RoundTrip(req.Clone(req.Context()))
}

func subtitleNetworkError(err error) bool {
	var networkErr net.Error
	return errors.As(err, &networkErr) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

func (t *subtitleTransport) CloseIdleConnections() {
	for _, transport := range []http.RoundTripper{t.direct, t.fallback} {
		if closer, ok := transport.(interface{ CloseIdleConnections() }); ok {
			closer.CloseIdleConnections()
		}
	}
}

func NewSubtitleClient(config *coretypes.AppConfig) (*bilibili.Client, error) {
	direct := http.DefaultTransport.(*http.Transport).Clone()
	direct.Proxy = nil // Do not inherit the downloader's or shell's proxy.
	direct.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	direct.TLSHandshakeTimeout = 30 * time.Second
	transport := &subtitleTransport{direct: direct}
	if config != nil && config.ProxyConfig != nil && config.ProxyConfig.UseProxy && config.ProxyConfig.ProxyHost != "" {
		proxy, err := url.Parse(config.ProxyConfig.ProxyHost)
		if err != nil || proxy.Host == "" {
			return nil, fmt.Errorf("字幕备用代理地址无效")
		}
		fallback := direct.Clone()
		fallback.Proxy = http.ProxyURL(proxy)
		transport.fallback = fallback
	}
	return bilibili.NewClient(bilibili.WithTimeout(2*time.Minute), bilibili.WithHTTPClient(&http.Client{Transport: transport})), nil
}
