package chain_task

import (
	"testing"

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
	if len(entries) != 1 {
		t.Fatalf("expected one public upload schedule, got %d", len(entries))
	}
}
