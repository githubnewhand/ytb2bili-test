package store

import (
	"testing"
	"time"
)

func TestLegacyPublicVideoUpdatesReadyWhenPreparationCompleted(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.Local)
	updates := legacyPublicVideoUpdates(now, int64(len(legacyPreparationSteps)))

	if updates["status"] != "200" {
		t.Fatalf("status = %v, want 200", updates["status"])
	}
	if updates["publish_audience"] != "" {
		t.Fatalf("publish_audience = %v, want empty", updates["publish_audience"])
	}
	if updates["upload_policy"] != "scheduled" {
		t.Fatalf("upload_policy = %v, want scheduled", updates["upload_policy"])
	}
	if updates["updated_at"] != now {
		t.Fatalf("updated_at = %v, want %v", updates["updated_at"], now)
	}
}

func TestLegacyPublicVideoUpdatesQueuesIncompletePreparation(t *testing.T) {
	updates := legacyPublicVideoUpdates(time.Now(), 1)

	if updates["status"] != "001" {
		t.Fatalf("status = %v, want 001", updates["status"])
	}
	if updates["workflow_state"] != "" {
		t.Fatalf("workflow_state = %v, want empty", updates["workflow_state"])
	}
	if updates["rights_verified"] != false {
		t.Fatalf("rights_verified = %v, want false", updates["rights_verified"])
	}
}
