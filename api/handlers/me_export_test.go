package handlers

import (
	"testing"
	"time"

	"github.com/wikiSubmission/ws-backend/db"
)

func TestComputeRetryAfterSecondsSentGate(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

	t.Run("sent 23h ago blocks", func(t *testing.T) {
		sent := now.Add(-23 * time.Hour)
		row := &db.UserDataExportRow{Status: "sent", SentAt: &sent}
		if got := computeRetryAfterSeconds(row, now); got <= 0 {
			t.Fatalf("expected retry_after > 0, got %d", got)
		}
	})

	t.Run("sent 25h ago eligible", func(t *testing.T) {
		sent := now.Add(-25 * time.Hour)
		row := &db.UserDataExportRow{Status: "sent", SentAt: &sent}
		if got := computeRetryAfterSeconds(row, now); got != 60 {
			t.Fatalf("expected fallback 60 seconds once eligible, got %d", got)
		}
	})
}
