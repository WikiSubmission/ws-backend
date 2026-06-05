package handlers

import (
	"testing"
	"time"

	"github.com/wikiSubmission/ws-backend/db"
)

func TestExportGateSatisfied(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	sentRecent := now.Add(-2 * 24 * time.Hour)
	sentStale := now.Add(-40 * 24 * time.Hour)
	url := "https://cdn.example.com/export.json"

	cases := []struct {
		name string
		row  *db.UserDataExportRow
		want bool
	}{
		{"no export at all", nil, false},
		{"still queued", &db.UserDataExportRow{Status: "queued"}, false},
		{"failed export", &db.UserDataExportRow{Status: "failed"}, false},
		{"sent but no timestamp", &db.UserDataExportRow{Status: "sent", DownloadURL: &url}, false},
		{"sent but no download url", &db.UserDataExportRow{Status: "sent", SentAt: &sentRecent}, false},
		{"sent and fresh", &db.UserDataExportRow{Status: "sent", SentAt: &sentRecent, DownloadURL: &url}, true},
		{"sent but stale", &db.UserDataExportRow{Status: "sent", SentAt: &sentStale, DownloadURL: &url}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exportGateSatisfied(tc.row, now); got != tc.want {
				t.Fatalf("exportGateSatisfied = %v, want %v", got, tc.want)
			}
		})
	}
}
