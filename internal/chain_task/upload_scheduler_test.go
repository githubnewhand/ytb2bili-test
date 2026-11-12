package chain_task

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

func TestUploadSchedulerSetUpRegistersMinuteCheck(t *testing.T) {
	schedulerCron := cron.New(cron.WithSeconds())
	scheduler := &UploadScheduler{
		Task:   schedulerCron,
		logger: zap.NewNop().Sugar(),
	}

	scheduler.SetUp()

	entries := schedulerCron.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected independent video and subtitle schedules, got %d", len(entries))
	}
}

func TestSubtitleUploadDue(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name, status, step string
		uploaded, updated  time.Time
		want               bool
	}{
		{"not yet one hour", "300", "pending", now.Add(-time.Hour + time.Second), now.Add(-time.Hour), false},
		{"exactly one hour", "300", "pending", now.Add(-time.Hour), now, true},
		{"editing video does not postpone", "300", "pending", now.Add(-2 * time.Hour), now, true},
		{"restart retains elapsed time", "300", "pending", now.Add(-24 * time.Hour), now, true},
		{"retry cooling down", "399", "failed", now.Add(-2 * time.Hour), now.Add(-14 * time.Minute), false},
		{"retry due", "399", "failed", now.Add(-2 * time.Hour), now.Add(-15 * time.Minute), true},
		{"completed excluded", "300", "completed", now.Add(-2 * time.Hour), now, false},
		{"running excluded", "300", "running", now.Add(-2 * time.Hour), now, false},
		{"missing upload time", "300", "pending", time.Time{}, now, false},
		{"legacy missing subtitle step", "300", "", now.Add(-2 * time.Hour), now, true},
		{"wrong video state", "400", "pending", now.Add(-2 * time.Hour), now, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := subtitleUploadCandidate{Status: tt.status, SubtitleStatus: tt.step, UploadedAt: tt.uploaded, UpdatedAt: tt.updated}
			if got := v.due(now); got != tt.want {
				t.Fatalf("due=%v want=%v", got, tt.want)
			}
		})
	}
}
