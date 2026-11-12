package bilibiliutil

import (
	"crypto/tls"
	"github.com/difyz9/bilibili-go-sdk/bilibili"
	"net"
	"net/http"
	"time"
)

// VideoUploadOptions is shared by queued and standalone manual uploads.
func VideoUploadOptions(timeout time.Duration) []bilibili.Option {
	direct := http.DefaultTransport.(*http.Transport).Clone()
	direct.Proxy = nil
	direct.ForceAttemptHTTP2 = false
	if direct.TLSClientConfig == nil {
		direct.TLSClientConfig = &tls.Config{}
	} else {
		direct.TLSClientConfig = direct.TLSClientConfig.Clone()
	}
	direct.TLSClientConfig.NextProtos = []string{"http/1.1"}
	direct.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	direct.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	direct.TLSHandshakeTimeout = 20 * time.Second
	direct.ResponseHeaderTimeout = 30 * time.Second
	options := []bilibili.Option{bilibili.WithTimeout(timeout), bilibili.WithHTTPClient(&http.Client{Transport: direct}), bilibili.WithProxy("")}
	return options
}
