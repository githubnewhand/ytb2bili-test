package bilibili

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChunkUploadRecoversWithIdenticalBody(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if string(body) != "test-video" || r.ContentLength != 10 {
			t.Errorf("invalid body %q length %d", body, r.ContentLength)
		}
		if r.URL.Query().Get("partNumber") != "1" {
			t.Error("part changed")
		}
		if calls == 1 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "video.mp4")
	os.WriteFile(path, []byte("test-video"), 0600)
	var messages []string
	client := &UploadClient{client: NewClient(), uploadClient: server.Client(), uploadProgress: func(p UploadProgress) { messages = append(messages, p.Message) }}
	parts, err := client.uploadChunks(path, &PreUploadInfo{ChunkSize: 10, Endpoint: strings.TrimPrefix(server.URL, "https:"), UposUri: "upos://test"}, "upload-id")
	if err != nil || len(parts) != 1 || calls != 2 {
		t.Fatalf("parts=%v calls=%d err=%v", parts, calls, err)
	}
	text := strings.Join(messages, ";")
	if !strings.Contains(text, "尝试 2/3") || !strings.Contains(text, "等待服务器确认") {
		t.Fatalf("missing state: %s", text)
	}
}
func TestUploadTransportExplicitDirectHTTP1(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	transport := buildUploadTransport("")
	if transport.Proxy != nil || transport.ForceAttemptHTTP2 || transport.TLSNextProto == nil || transport.TLSClientConfig.NextProtos[0] != "http/1.1" {
		t.Fatal("upload is not explicitly direct HTTP/1")
	}
}

func TestPreUploadUsesSelectedStorageRoute(t *testing.T) {
	for _, route := range []string{"ws", "qn", "bda2"} {
		t.Run(route, func(t *testing.T) {
			client := NewUploadClient(&LoginInfo{}, WithUploadCDN(route))
			client.client.httpClient.Transport = routeTransport{t: t, route: route}
			info, err := client.preUpload("example.mp4", 511034299)
			if err != nil || info.Endpoint != "//storage.example" {
				t.Fatalf("%v %v", info, err)
			}
		})
	}
}

type routeTransport struct {
	t     *testing.T
	route string
}

func (tr routeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Query().Get("upcdn") != tr.route {
		tr.t.Error("storage route ignored")
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"OK":1,"upos_uri":"upos://test","endpoint":"//storage.example"}`)), Header: make(http.Header)}, nil
}

func TestStandaloneUploadChangesStorageRoute(t *testing.T) {
	client := NewUploadClient(&LoginInfo{})
	var routes []string
	video, err := client.withUploadRoutes(func() (*Video, error) {
		routes = append(routes, client.uploadCDN)
		if client.uploadCDN == "ws" {
			return nil, fmt.Errorf("upload chunk 1 failed with status 500 Internal Server Error")
		}
		return &Video{}, nil
	})
	if err != nil || video == nil || strings.Join(routes, ",") != "ws,qn" || client.uploadCDN != "" {
		t.Fatalf("routes=%v err=%v", routes, err)
	}
}
