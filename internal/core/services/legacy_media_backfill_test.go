package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/difyz9/ytb2bili/internal/core/types"
	"github.com/difyz9/ytb2bili/pkg/store/model"
)

func TestFindLegacyMediaFileUsesExistingTaskVideo(t *testing.T) {
	root := t.TempDir()
	video := &model.SavedVideo{
		VideoID:   "legacy-video",
		BaseModel: model.BaseModel{CreatedAt: time.Date(2026, 7, 29, 12, 0, 0, 0, time.Local)},
	}
	taskDir := filepath.Join(root, "2026-07-29", video.VideoID)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(taskDir, video.VideoID+".mp4")
	if err := os.WriteFile(expected, []byte("existing media"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, video.VideoID+".wav"), []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := findLegacyMediaFile(&types.AppConfig{FileUpDir: root}, video)
	if err != nil {
		t.Fatalf("findLegacyMediaFile() error = %v", err)
	}
	if got != expected {
		t.Fatalf("findLegacyMediaFile() = %q, want %q", got, expected)
	}
}

func TestFindLegacyMediaFileFallsBackAcrossDateDirectories(t *testing.T) {
	root := t.TempDir()
	video := &model.SavedVideo{
		VideoID:   "moved-video",
		BaseModel: model.BaseModel{CreatedAt: time.Date(2026, 7, 28, 23, 59, 59, 0, time.Local)},
	}
	taskDir := filepath.Join(root, "2026-07-29", video.VideoID)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(taskDir, "downloaded.webm")
	if err := os.WriteFile(expected, []byte("existing media"), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := findLegacyMediaFile(&types.AppConfig{FileUpDir: root}, video)
	if err != nil {
		t.Fatalf("findLegacyMediaFile() error = %v", err)
	}
	if got != expected {
		t.Fatalf("findLegacyMediaFile() = %q, want %q", got, expected)
	}
}

func TestFindLegacyMediaFileRejectsTraversalVideoID(t *testing.T) {
	video := &model.SavedVideo{
		VideoID:   "../outside",
		BaseModel: model.BaseModel{CreatedAt: time.Now()},
	}
	if _, err := findLegacyMediaFile(&types.AppConfig{FileUpDir: t.TempDir()}, video); err == nil {
		t.Fatal("findLegacyMediaFile() accepted a traversal video ID")
	}
}
