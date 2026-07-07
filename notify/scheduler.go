package notify

import (
	"context"
	"log/slog"
	"time"

	"github.com/wikiSubmission/ws-backend/db"
)

// claimBatchSize caps how many due schedules one tick processes.
const claimBatchSize = 20

// Scheduler periodically claims due scheduled_notifications rows and hands
// them to the Dispatcher. Claiming uses FOR UPDATE SKIP LOCKED and advances
// next_run_at inside the claim transaction, so multiple instances split the
// work safely and each occurrence dispatches at most once.
type Scheduler struct {
	Dispatcher *Dispatcher
	Interval   time.Duration // defaults to 60s
}

// Run blocks until ctx is cancelled. Call it in a goroutine from StartServer.
func (s *Scheduler) Run(ctx context.Context) {
	interval := s.Interval
	if interval <= 0 {
		interval = time.Minute
	}
	slog.Info("Notification scheduler started", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("Notification scheduler stopped")
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	now := time.Now()
	claimed, err := s.Dispatcher.DB.ClaimDueScheduledNotifications(ctx, now, claimBatchSize, s.nextRun(now))
	if err != nil {
		slog.Error("notify: claiming due schedules failed", "error", err)
		return
	}

	for _, row := range claimed {
		webSent, fcmSent, dispatchErr := s.Dispatcher.Dispatch(ctx, row)

		status := "ok"
		var errText *string
		if dispatchErr != nil {
			msg := dispatchErr.Error()
			errText = &msg
			if webSent > 0 || fcmSent > 0 {
				status = "partial"
			} else {
				status = "error"
			}
		}
		if recordErr := s.Dispatcher.DB.RecordScheduleResult(ctx, row.ID, status, errText, webSent, fcmSent); recordErr != nil {
			slog.Error("notify: recording schedule result failed", "id", row.ID, "error", recordErr)
		}
		slog.Info("notify: schedule dispatched",
			"id", row.ID, "title", row.Title, "status", status,
			"web_sent", webSent, "fcm_sent", fcmSent)
	}
}

// nextRun computes the follow-up occurrence for a claimed row. 'once' rows are
// disabled by the claim query regardless of the returned value. An invalid
// timezone (should be prevented at the API boundary) falls back to UTC rather
// than stalling the schedule.
func (s *Scheduler) nextRun(now time.Time) func(row db.ScheduledNotificationRow) time.Time {
	return func(row db.ScheduledNotificationRow) time.Time {
		loc, err := time.LoadLocation(row.Timezone)
		if err != nil {
			slog.Warn("notify: invalid schedule timezone, using UTC", "id", row.ID, "timezone", row.Timezone)
			loc = time.UTC
		}
		return NextRunAt(row.Frequency, row.StartsAt, now, loc)
	}
}
