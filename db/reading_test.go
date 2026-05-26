package db

import (
	"context"
	"testing"
	"time"
)

func TestGetReadingStats_BucketsByDayHourWeekday(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	authID := "test-reading-stats-" + time.Now().Format("150405.000000")
	email := authID + "@example.com"
	if _, err := db.UpsertUser(ctx, authID, email, nil); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM users WHERE auth_id = $1`, authID)
	})

	tz := "UTC"
	scripture := "quran"
	today := time.Now().UTC().Format("2006-01-02")

	for i := 0; i < 3; i++ {
		if err := db.IncrementReadingLog(ctx, authID, scripture, today, 2); err != nil {
			t.Fatalf("IncrementReadingLog %d: %v", i, err)
		}
	}

	stats, err := db.GetReadingStats(ctx, authID, scripture, tz, time.Now().AddDate(0, 0, -7))
	if err != nil {
		t.Fatalf("GetReadingStats: %v", err)
	}
	if stats.Total != 6 {
		t.Fatalf("expected total=6, got %d", stats.Total)
	}
	if stats.ActiveDays != 1 {
		t.Fatalf("expected active_days=1, got %d", stats.ActiveDays)
	}
	if len(stats.Daily) != 1 {
		t.Fatalf("expected 1 daily bucket, got %d", len(stats.Daily))
	}
	if stats.Daily[0].VersesRead != 6 {
		t.Fatalf("expected daily[0].verses_read=6, got %d", stats.Daily[0].VersesRead)
	}
	if len(stats.Hourly) == 0 {
		t.Fatal("expected at least one hourly bucket")
	}
	if len(stats.Weekday) == 0 {
		t.Fatal("expected at least one weekday bucket")
	}
	if stats.BestDay == nil || stats.BestDay.VersesRead != 6 {
		t.Fatalf("best_day mismatch: %+v", stats.BestDay)
	}
	if len(stats.Cumulative) != 1 || stats.Cumulative[0].VersesRead != 6 {
		t.Fatalf("cumulative mismatch: %+v", stats.Cumulative)
	}
}

func TestGetReadingStats_EmptyUser(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	authID := "test-reading-stats-empty-" + time.Now().Format("150405.000000")
	email := authID + "@example.com"
	if _, err := db.UpsertUser(ctx, authID, email, nil); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM users WHERE auth_id = $1`, authID)
	})

	stats, err := db.GetReadingStats(ctx, authID, "quran", "UTC", time.Time{})
	if err != nil {
		t.Fatalf("GetReadingStats: %v", err)
	}
	if stats.Total != 0 || stats.ActiveDays != 0 {
		t.Fatalf("expected zeros, got total=%d active=%d", stats.Total, stats.ActiveDays)
	}
	if stats.BestDay != nil {
		t.Fatalf("expected nil best_day, got %+v", stats.BestDay)
	}
	if stats.Daily == nil || stats.Hourly == nil || stats.Weekday == nil || stats.Cumulative == nil {
		t.Fatal("slices must be non-nil even when empty")
	}
}
