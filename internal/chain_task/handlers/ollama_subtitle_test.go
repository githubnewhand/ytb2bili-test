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

func TestOllamaSubtitleUsesLocalDirectJSONAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected route %s", r.URL.Path)
		}
		var req struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Think    bool   `json:"think"`
			Format   string `json:"format"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		if req.Model != "qwen3:4b-instruct" || req.Stream || req.Think || req.Format != "json" {
			t.Errorf("unexpected local inference request: %+v", req)
		}
		if len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Content != `{"T000001":"Good morning."}` {
			t.Errorf("prompt not forwarded: %+v", req.Messages)
		}
		fmt.Fprint(w, `{"message":{"content":"{\"T000001\":\"早上好。\"}"}}`)
	}))
	defer server.Close()
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	config := &types.AppConfig{SubtitleTranslationConfig: &types.SubtitleTranslationConfig{
		Provider: "ollama", OllamaURL: server.URL, OllamaModel: "qwen3:4b-instruct", Timeout: 5,
	}}
	translator := &TranslateSubtitle{App: &core.AppServer{Config: config}}
	got, err := translator.callSubtitleTranslationAPI("translate", `{"T000001":"Good morning."}`)
	if err != nil || got != `{"T000001":"早上好。"}` {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestOllamaSubtitleRejectsRemoteEndpoint(t *testing.T) {
	_, _, _, err := ollamaSubtitleSettings(&types.AppConfig{SubtitleTranslationConfig: &types.SubtitleTranslationConfig{
		Provider: "ollama", OllamaURL: "http://example.com:11434",
	}})
	if err == nil || !strings.Contains(err.Error(), "本机") {
		t.Fatalf("remote URL accepted: %v", err)
	}
}

func TestSubtitleCacheSeparatesProviders(t *testing.T) {
	source := "Please close your eyes."
	legacyDeepSeek := subtitleTranslationCacheKey(source, "deepseek-chat")
	selectedDeepSeek := subtitleTranslationCacheKey(source, "deepseek:deepseek-chat")
	ollama := subtitleTranslationCacheKey(source, "ollama:qwen3:4b-instruct")
	if legacyDeepSeek != selectedDeepSeek {
		t.Fatal("existing DeepSeek cache keys changed")
	}
	if legacyDeepSeek == ollama {
		t.Fatal("Ollama and DeepSeek shared a subtitle cache key")
	}
}

func TestOllamaPreflightRequiresConfiguredModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"models":[{"name":"qwen3:4b-instruct"}]}`)
	}))
	defer server.Close()
	config := &types.AppConfig{SubtitleTranslationConfig: &types.SubtitleTranslationConfig{
		Provider: "ollama", OllamaURL: server.URL, OllamaModel: "qwen3:4b-instruct",
	}}
	if err := checkOllamaReady(config); err != nil {
		t.Fatal(err)
	}
	config.SubtitleTranslationConfig.OllamaModel = "missing-model"
	if err := checkOllamaReady(config); err == nil {
		t.Fatal("missing model was accepted")
	}
}

func TestOllamaMissingCueIsRetriedWithoutPlaceholder(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		var input map[string]string
		if err := json.Unmarshal([]byte(req.Messages[1].Content), &input); err != nil {
			t.Fatal(err)
		}
		output := map[string]string{}
		for id := range input {
			if calls == 1 && id == "T000002" {
				continue
			}
			output[id] = "有效译文"
		}
		payload, _ := json.Marshal(output)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": map[string]string{"content": string(payload)}})
	}))
	defer server.Close()
	config := &types.AppConfig{SubtitleTranslationConfig: &types.SubtitleTranslationConfig{
		Provider: "ollama", OllamaURL: server.URL, OllamaModel: "qwen3:4b-instruct",
	}}
	translator := &TranslateSubtitle{App: &core.AppServer{Config: config}}
	items := []subtitleTranslationItem{{ID: "T000001", Text: "Good morning."}, {ID: "T000002", Text: "How are you?"}}
	got, err := translator.translateSubtitleItemGroupResilient(items)
	if err != nil || len(got) != 2 || calls != 2 {
		t.Fatalf("recovery failed: translations=%v calls=%d err=%v", got, calls, err)
	}
}

func TestOllamaCheckpointRejectsChangedSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkpoint.json")
	translator := &TranslateSubtitle{CheckpointPath: path, CheckpointSignature: "source-1",
		CheckpointResults: map[string]string{"T000001": "早上好。", "T000002": "[翻译失败] hello"}}
	if err := translator.saveSubtitleCheckpoint(); err != nil {
		t.Fatal(err)
	}
	got := translator.loadSubtitleCheckpoint()
	if len(got) != 1 || got["T000001"] != "早上好。" {
		t.Fatalf("invalid checkpoint data: %v", got)
	}
	translator.CheckpointSignature = "source-2"
	if got := translator.loadSubtitleCheckpoint(); len(got) != 0 {
		t.Fatalf("stale checkpoint reused: %v", got)
	}
}

func TestOllamaExecuteCompletesWithoutDeepSeekValidator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			fmt.Fprint(w, `{"models":[{"name":"qwen3:4b-instruct"}]}`)
		case "/api/chat":
			fmt.Fprint(w, `{"message":{"content":"{\"T000001\":\"早上好。\"}"}}`)
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
		}
	}))
	defer server.Close()
	config := &types.AppConfig{SubtitleTranslationConfig: &types.SubtitleTranslationConfig{
		Provider: "ollama", OllamaURL: server.URL, OllamaModel: "qwen3:4b-instruct",
	}}
	logger := zap.NewExample()
	defer logger.Sync()
	app := &core.AppServer{Config: config, Logger: logger.Sugar()}
	state := manager.NewStateManager(0, "example", t.TempDir(), time.Now())
	source := "1\n00:00:01,000 --> 00:00:03,000\nGood morning.\n\n"
	if err := os.WriteFile(filepath.Join(state.CurrentDir, "example.srt"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	translator := NewTranslateSubtitle("翻译字幕", app, state, nil, nil, "")
	if !translator.Execute(map[string]interface{}{}) {
		t.Fatal("Ollama handler did not complete")
	}
	translated, err := os.ReadFile(filepath.Join(state.CurrentDir, "zh.srt"))
	if err != nil || !strings.Contains(string(translated), "早上好。") {
		t.Fatalf("missing translated SRT: %s, %v", translated, err)
	}
}

func TestParseSRTDoesNotMergeEmptyAutoCaptionCue(t *testing.T) {
	input := "1\n00:00:01,000 --> 00:00:02,000\n\nHello.\n\n2\n00:00:02,000 --> 00:00:03,000\n \n\n3\n00:00:03,000 --> 00:00:04,000\n\nGoodbye.\n"
	entries, err := (&TranslateSubtitle{}).parseSRTContent(input)
	if err != nil || len(entries) != 2 || entries[0].Index != 1 || entries[1].Index != 3 || entries[1].Text != "Goodbye." {
		t.Fatalf("blank cue swallowed its neighbor: %+v, %v", entries, err)
	}
}
