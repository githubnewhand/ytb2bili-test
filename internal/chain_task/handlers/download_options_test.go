package handlers

import (
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/types"
	"testing"
	"time"
)

func TestDownloadProgressDetails(t *testing.T) {
	for _, tc := range []struct {
		line    string
		percent int
		want    string
	}{
		{"[download]   9.4% of 1.44GiB at 40.25KiB/s ETA 10:02:30", 9, "下载中 9.4% · 40.25KiB/s · 剩余 10:02:30"},
		{"[#abcd 100MiB/1GiB(9%) CN:4 DL:150KiB ETA:1h20m]", 9, "下载中 9% · 150KiB/s · 剩余 1h20m"},
		{"[download] 100.0% of 100MiB in 00:30 at 3.33MiB/s", 100, "下载中 100.0% · 3.33MiB/s"},
		{"[download] 0.1% of 10MiB at Unknown B/s ETA Unknown", 0, "下载中 0.1%"},
		{"https://example.com/private?token=secret", 5, "下载中 5%"},
	} {
		if got := downloadProgressMessage(tc.line, tc.percent); got != tc.want {
			t.Errorf("got %q; want %q", got, tc.want)
		}
	}
}

func TestDownloadOverallTimeout(t *testing.T) {
	task := &DownloadVideo{}
	if got := task.overallTimeout(); got != 24*time.Hour {
		t.Fatal(got)
	}
	task.App = &core.AppServer{Config: &types.AppConfig{}}
	for _, tc := range []struct {
		hours int
		want  time.Duration
	}{{48, 48 * time.Hour}, {0, 24 * time.Hour}, {-1, 24 * time.Hour}, {1000000000, 24 * time.Hour}} {
		task.App.Config.Download.OverallTimeoutHours = tc.hours
		if got := task.overallTimeout(); got != tc.want {
			t.Errorf("hours=%d: got %s, want %s", tc.hours, got, tc.want)
		}
	}
}
