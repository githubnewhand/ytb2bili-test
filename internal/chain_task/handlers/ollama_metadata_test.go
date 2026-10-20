package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/types"
	"go.uber.org/zap"
)

func TestFormalMetadataHandlerUsesOllamaAndWritesFileOnly(t *testing.T) {
	chatCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			fmt.Fprint(w, `{"models":[{"name":"qwen3:4b-instruct"}]}`)
		case "/api/chat":
			chatCalls++
			var req struct {
				Model  string `json:"model"`
				Format string `json:"format"`
				Think  bool   `json:"think"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			if req.Model != "qwen3:4b-instruct" || req.Format != "json" || req.Think {
				t.Errorf("invalid Ollama request: %+v", req)
			}
			fmt.Fprint(w, `{"message":{"content":"{\"title\":\"助眠理发\",\"description\":\"这是一段私人理发主题的助眠视频，中文字幕来自自动识别内容，可能存在误差。\",\"tags\":[\"ASMR\",\"理发\",\"助眠\"]}"}}`)
		default:
			t.Errorf("unexpected route: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	config := &types.AppConfig{MetadataGenerationConfig: &types.MetadataGenerationConfig{
		Provider: "ollama", OllamaURL: server.URL, OllamaModel: "qwen3:4b-instruct", Timeout: 5,
	}}
	logger := zap.NewExample()
	defer logger.Sync()
	state := manager.NewStateManager(0, "example", t.TempDir(), time.Now())
	if err := os.WriteFile(filepath.Join(state.CurrentDir, "zh.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\n欢迎来到理发店。\n"), 0644); err != nil {
		t.Fatal(err)
	}
	handler := NewGenerateMetadata("生成元数据", &core.AppServer{Config: config, Logger: logger.Sugar()}, state, nil, "", nil, nil)
	handler.FileOnly = true
	context := map[string]interface{}{}
	if !handler.Execute(context) {
		t.Fatalf("formal handler failed: %v", context["error"])
	}
	if chatCalls != 1 {
		t.Fatalf("expected one local generation, got %d", chatCalls)
	}
	data, err := os.ReadFile(filepath.Join(state.CurrentDir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved VideoMetadata
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Title != "助眠理发" || len(saved.Tags) != 3 {
		t.Fatalf("unexpected meta.json: %s", data)
	}
}

func TestMetadataOllamaRejectsRemoteEndpoint(t *testing.T) {
	_, _, _, err := ollamaMetadataSettings(&types.AppConfig{MetadataGenerationConfig: &types.MetadataGenerationConfig{
		Provider: "ollama", OllamaURL: "http://example.com:11434",
	}})
	if err == nil || !strings.Contains(err.Error(), "本机") {
		t.Fatalf("remote metadata endpoint accepted: %v", err)
	}
}
