package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type ScheduledNotificationRow struct {
	ID             int        `db:"id"`
	Title          string     `db:"title"`
	Body           string     `db:"body"`
	URL            *string    `db:"url"`
	Category       *string    `db:"category"`
	Destination    string     `db:"destination"`
	CategoryFilter *string    `db:"category_filter"`
	Frequency      string     `db:"frequency"`
	StartsAt       time.Time  `db:"starts_at"`
	Timezone       string     `db:"timezone"`
	Enabled        bool       `db:"enabled"`
	NextRunAt      time.Time  `db:"next_run_at"`
	LastRunAt      *time.Time `db:"last_run_at"`
	LastStatus     *string    `db:"last_status"`
	LastError      *string    `db:"last_error"`
	LastWebSent    *int       `db:"last_web_sent"`
	LastFCMSent    *int       `db:"last_fcm_sent"`
	CreatedBy      *string    `db:"created_by"`
	CreatedAt      time.Time  `db:"created_at"`
	UpdatedAt      time.Time  `db:"updated_at"`
}

const scheduledNotificationCols = `id, title, body, url, category, destination, category_filter,
	frequency, starts_at, timezone, enabled, next_run_at, last_run_at, last_status, last_error,
	last_web_sent, last_fcm_sent, created_by, created_at, updated_at`

func (db *DB) ListScheduledNotifications(ctx context.Context) ([]ScheduledNotificationRow, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+scheduledNotificationCols+` FROM scheduled_notifications ORDER BY next_run_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[ScheduledNotificationRow])
}

// GetScheduledNotification returns nil when the id does not exist.
func (db *DB) GetScheduledNotification(ctx context.Context, id int) (*ScheduledNotificationRow, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+scheduledNotificationCols+` FROM scheduled_notifications WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ScheduledNotificationRow])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

type NewScheduledNotification struct {
	Title          string
	Body           string
	URL            *string
	Category       *string
	Destination    string
	CategoryFilter *string
	Frequency      string
	StartsAt       time.Time
	Timezone       string
	Enabled        bool
	NextRunAt      time.Time
	CreatedBy      *string
}

func (db *DB) InsertScheduledNotification(ctx context.Context, n NewScheduledNotification) (*ScheduledNotificationRow, error) {
	query := `INSERT INTO scheduled_notifications
	            (title, body, url, category, destination, category_filter, frequency,
	             starts_at, timezone, enabled, next_run_at, created_by)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	          RETURNING ` + scheduledNotificationCols
	rows, err := db.Pool.Query(ctx, query,
		n.Title, n.Body, n.URL, n.Category, n.Destination, n.CategoryFilter, n.Frequency,
		n.StartsAt, n.Timezone, n.Enabled, n.NextRunAt, n.CreatedBy)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ScheduledNotificationRow])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// UpdateScheduledNotification applies a partial update; nil fields keep their
// current value. nextRunAt is recomputed by the handler whenever a timing field
// changes. Returns nil when the id does not exist.
func (db *DB) UpdateScheduledNotification(
	ctx context.Context,
	id int,
	title, body, url, category, destination, categoryFilter, frequency *string,
	startsAt *time.Time,
	timezone *string,
	enabled *bool,
	nextRunAt *time.Time,
) (*ScheduledNotificationRow, error) {
	query := `UPDATE scheduled_notifications SET
	            title = COALESCE($2, title),
	            body = COALESCE($3, body),
	            url = COALESCE($4, url),
	            category = COALESCE($5, category),
	            destination = COALESCE($6, destination),
	            category_filter = COALESCE($7, category_filter),
	            frequency = COALESCE($8, frequency),
	            starts_at = COALESCE($9, starts_at),
	            timezone = COALESCE($10, timezone),
	            enabled = COALESCE($11, enabled),
	            next_run_at = COALESCE($12, next_run_at),
	            updated_at = NOW()
	          WHERE id = $1
	          RETURNING ` + scheduledNotificationCols
	rows, err := db.Pool.Query(ctx, query, id,
		title, body, url, category, destination, categoryFilter, frequency,
		startsAt, timezone, enabled, nextRunAt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ScheduledNotificationRow])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// DeleteScheduledNotification removes a schedule. Returns false when nothing
// matched.
func (db *DB) DeleteScheduledNotification(ctx context.Context, id int) (bool, error) {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM scheduled_notifications WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ClaimDueScheduledNotifications atomically claims schedules that are due:
// inside one transaction it selects due rows with FOR UPDATE SKIP LOCKED (so
// concurrent instances split the work instead of double-sending), advances
// next_run_at per row, disables one-shot schedules, and marks them
// 'dispatching'. Advancing before the send gives at-most-once semantics: a
// crash mid-dispatch drops that occurrence instead of duplicating it on
// restart, and the stuck 'dispatching' status stays visible in the admin UI.
// nextRun computes the follow-up run for a claimed row ('once' rows are
// disabled regardless of its result).
func (db *DB) ClaimDueScheduledNotifications(
	ctx context.Context,
	now time.Time,
	limit int,
	nextRun func(row ScheduledNotificationRow) time.Time,
) ([]ScheduledNotificationRow, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx,
		`SELECT `+scheduledNotificationCols+`
		 FROM scheduled_notifications
		 WHERE enabled AND next_run_at <= $1
		 ORDER BY next_run_at
		 FOR UPDATE SKIP LOCKED
		 LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	claimed, err := pgx.CollectRows(rows, pgx.RowToStructByName[ScheduledNotificationRow])
	if err != nil {
		return nil, err
	}

	for _, row := range claimed {
		next := nextRun(row)
		_, err := tx.Exec(ctx,
			`UPDATE scheduled_notifications
			 SET next_run_at = $2,
			     last_run_at = $3,
			     last_status = 'dispatching',
			     enabled = (frequency <> 'once'),
			     updated_at = NOW()
			 WHERE id = $1`, row.ID, next, now)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return claimed, nil
}

// RecordScheduleResult stores the outcome of a dispatch.
func (db *DB) RecordScheduleResult(ctx context.Context, id int, status string, errText *string, webSent, fcmSent int) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE scheduled_notifications
		 SET last_status = $2, last_error = $3, last_web_sent = $4, last_fcm_sent = $5, updated_at = NOW()
		 WHERE id = $1`, id, status, errText, webSent, fcmSent)
	return err
}
