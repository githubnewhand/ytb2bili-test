package bilibiliutil

import (
	"github.com/difyz9/bilibili-go-sdk/bilibili"
	"net/http"
	"testing"
	"time"
)

func TestVideoOptionsIgnoreDownloadProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	client := bilibili.NewClient(VideoUploadOptions(time.Minute)...)
	tr := client.GetHTTPClient().Transport.(*http.Transport)
	if tr.Proxy != nil || tr.ForceAttemptHTTP2 || len(tr.TLSClientConfig.NextProtos) != 1 || tr.TLSClientConfig.NextProtos[0] != "http/1.1" {
		t.Fatal("video upload must explicitly use direct HTTP/1.1")
	}
}
