package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	"go.uber.org/zap"
)

func contentMetadata(description string) *VideoMetadata {
	return &VideoMetadata{Title: "视频主题", Description: description, Tags: []string{"内容", "体验", "分享"}}
}
func metadataJSON(description string) string {
	data, _ := json.Marshal(contentMetadata(description))
	return string(data)
}

func TestMetadataRejectsTranscriptCommentary(t *testing.T) {
	for _, description := range []string{
		"沉浸式体验吸血鬼牙医的放松诊疗场景，融合超自然元素与日常护理，营造安心氛围。字幕可能存在自动识别误差，内容以原视频标题为准。",
		"沉浸式面部按摩ASMR内容，带来放松与舒适感。字幕中频繁的打鼾声可能为自动识别误差，实际音频以原视频为准。",
		"放松理发。 中文字幕由自动翻译生成，可能有误，请以原视频为准。",
		"轻柔按摩，字幕可能存在识别误差。", "本简介基于字幕生成。", "本视频字幕存在翻译错误。", "根据字幕推测，这是按摩视频。", "从提供的字幕来看，这是按摩视频。",
		"转录文本不完整，无法确认所有细节。", "A relaxing video. The subtitles may contain transcription errors.",
	} {
		t.Run(description, func(t *testing.T) {
			if err := validateGeneratedMetadata(contentMetadata(description)); err == nil {
				t.Fatal("commentary accepted")
			}
			if _, err := parseMetadataJSON(metadataJSON(description)); err == nil {
				t.Fatal("Gemini parser accepted commentary")
			}
		})
	}
	for _, field := range []string{"title", "tag"} {
		m := contentMetadata("视频展示放松按摩。")
		if field == "title" {
			m.Title = "字幕可能存在识别误差"
		} else {
			m.Tags[0] = "字幕由自动翻译生成"
		}
		if err := validateGeneratedMetadata(m); err == nil {
			t.Fatalf("%s bypassed guard", field)
		}
	}
}

func TestMetadataAllowsActualVideoTopics(t *testing.T) {
	for _, description := range []string{
		"视频展示吸血鬼牙医的角色扮演，通过轻柔交流营造放松氛围。",
		"介绍如何制作双语字幕、调整时间轴，并导出字幕文件。", "字幕识别错误修复教程", "字幕自动识别教程",
		"视频讲解如何修复自动字幕的识别错误，并演示提高字幕准确性的方法。",
		"本期介绍机器翻译的原理与常见误差，通过案例讲解评估方法。",
		"讲解语音识别模型处理打鼾声和背景噪声的方法。",
	} {
		if err := validateGeneratedMetadata(contentMetadata(description)); err != nil {
			t.Fatalf("valid topic rejected: %s: %v", description, err)
		}
	}
}

func TestGeminiMetadataRetriesAndFailsClosed(t *testing.T) {
	for _, recover := range []bool{true, false} {
		calls := 0
		got, err := requestContentOnlyMetadata("prompt", func(prompt string) (string, error) {
			calls++
			if calls > 1 && !strings.Contains(prompt, "上次结果未通过校验") {
				t.Error("missing correction")
			}
			if recover && calls == 2 {
				return metadataJSON("视频展示轻柔按摩，营造放松氛围。"), nil
			}
			return metadataJSON("字幕可能存在自动识别误差。"), nil
		})
		if recover {
			if err != nil || got == nil || calls != 2 {
				t.Fatalf("recovery failed: %v %d", err, calls)
			}
		} else if err == nil || got != nil || calls != 3 {
			t.Fatalf("invalid output escaped: %v %d", err, calls)
		}
	}
}

func TestOllamaMetadataRetriesCommentary(t *testing.T) {
	for _, recover := range []bool{true, false} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			var req struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			if len(req.Messages) < 2 || !strings.Contains(req.Messages[0].Content, metadataContentOnlyRules) {
				t.Error("missing shared policy")
			}
			if calls > 1 && !strings.Contains(req.Messages[1].Content, "上次结果未通过校验") {
				t.Error("missing retry feedback")
			}
			description := "字幕可能存在自动识别误差。"
			if recover && calls == 2 {
				description = "视频以轻柔的面部按摩营造放松氛围。"
			}
			json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": metadataJSON(description)}})
		}))
		got, err := (&GenerateMetadata{}).generateMetadataFromOllama(server.URL, "test", time.Second, "欢迎来到按摩体验。", "ASMR Massage")
		server.Close()
		if recover {
			if err != nil || got == nil || calls != 2 || strings.Contains(got.Description, "字幕") {
				t.Fatalf("recovery failed: %v %d", err, calls)
			}
		} else if err == nil || got != nil || calls != 3 {
			t.Fatalf("invalid output escaped: %v %d", err, calls)
		}
	}
}

func TestDeepSeekMetadataRetriesCommentary(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req DeepSeekRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if len(req.Messages) < 2 || !strings.Contains(req.Messages[0].Content, metadataContentOnlyRules) {
			t.Error("missing shared policy")
		}
		if calls > 1 && !strings.Contains(req.Messages[1].Content, "上次结果未通过校验") {
			t.Error("missing retry feedback")
		}
		description := "字幕可能存在自动识别误差。"
		if calls == 2 {
			description = "视频展示舒缓的按摩体验。"
		}
		json.NewEncoder(w).Encode(DeepSeekResponse{Choices: []DeepSeekChoice{{Message: DeepSeekMessage{Content: metadataJSON(description)}, FinishReason: "stop"}}})
	}))
	defer server.Close()
	g := &GenerateMetadata{App: &core.AppServer{Logger: zap.NewNop().Sugar()}, DeepSeekClient: &DeepSeekClient{BaseURL: server.URL, Model: "test", Client: server.Client()}}
	got, err := g.generateMetadataFromDeepSeek("欢迎体验按摩。")
	if err != nil || calls != 2 || got == nil || strings.Contains(got.Description, "字幕") {
		t.Fatalf("recovery failed: %v %d", err, calls)
	}
}

func TestInvalidMetadataCannotOverwriteSavedResults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.json")
	original := []byte(`{"description":"已有的正确简介"}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	g := &GenerateMetadata{FileOnly: true}
	g.StateManager = &manager.StateManager{CurrentDir: dir}
	ctx := map[string]interface{}{"video_description": "已有的正确简介"}
	bad := contentMetadata("字幕可能存在识别误差。")
	if g.saveMetadataResults(bad, ctx) {
		t.Fatal("invalid metadata saved")
	}
	if ctx["video_description"] != "已有的正确简介" {
		t.Fatal("context was overwritten")
	}
	if err := g.saveMetadataToFile(bad); err == nil {
		t.Fatal("file guard missing")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(original) {
		t.Fatal("previous metadata overwritten")
	}
	if g.saveMetadataResults(nil, ctx) {
		t.Fatal("nil metadata accepted")
	}
}
