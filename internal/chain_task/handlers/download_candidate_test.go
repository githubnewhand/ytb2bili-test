package handlers

import (
	"fmt"
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadMetadataIdentityAndFreshness(t *testing.T) {
	dir := t.TempDir()
	task := &DownloadVideo{}
	task.StateManager = &manager.StateManager{VideoID: "X0tNzjNkXEk", CurrentDir: dir}
	p := filepath.Join(dir, "X0tNzjNkXEk.info.json")
	for _, tc := range []struct {
		name, body string
		old, want  bool
	}{
		{"valid", `{"id":"X0tNzjNkXEk","title":"Title","description":"Description"}`, false, true},
		{"wrong video", `{"id":"another","title":"Title"}`, false, false},
		{"missing title", `{"id":"X0tNzjNkXEk"}`, false, false},
		{"corrupt", `{`, false, false},
		{"stale", `{"id":"X0tNzjNkXEk","title":"Title"}`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			started := time.Now()
			os.WriteFile(p, []byte(tc.body), 0600)
			if tc.old {
				old := started.Add(-time.Hour)
				os.Chtimes(p, old, old)
			}
			_, err := task.readDownloadMetadata(started)
			if (err == nil) != tc.want {
				t.Fatalf("err=%v wantValid=%v", err, tc.want)
			}
		})
	}
	os.Remove(p)
	os.WriteFile(filepath.Join(dir, "other.info.json"), []byte(`{"id":"other","title":"Other"}`), 0600)
	if _, err := task.readDownloadMetadata(time.Now()); err == nil {
		t.Fatal("unrelated sidecar accepted")
	}
}
func TestDownloadMetadataURLSafety(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"https://www.youtube.com/watch?v=X0tNzjNkXEk", "X0tNzjNkXEk"},
		{"https://youtu.be/X0tNzjNkXEk", "X0tNzjNkXEk"},
		{"../other", ""}, {"https://example.org/watch?v=X0tNzjNkXEk", ""},
	} {
		if got := downloadMetadataID(tc.in); got != tc.want {
			t.Fatalf("%s: %s", tc.in, got)
		}
	}
}
func TestDownloadStreamDestinationDoesNotReset(t *testing.T) {
	p := &downloadStreamProgress{}
	if p.start() != 5 || p.percent(100) != 78 || p.start() != 78 || p.percent(0) != 78 || p.percent(100) != 90 {
		t.Fatal("stream boundaries invalid")
	}
}
func TestDownload403IsClassifiedWithoutClaimingIPCause(t *testing.T) {
	got := (&DownloadVideo{}).formatDownloadError(fmt.Errorf("exit 1"), "HTTP Error 403: Forbidden")
	if !strings.Contains(got, "[http_403]") {
		t.Fatal(got)
	}
}
