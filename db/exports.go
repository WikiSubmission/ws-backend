package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type UserDataExportRow struct {
	ID          int64      `db:"id" json:"id"`
	UserID      int        `db:"user_id" json:"user_id"`
	Status      string     `db:"status" json:"status"`
	ObjectKey   *string    `db:"object_key" json:"object_key,omitempty"`
	DownloadURL *string    `db:"download_url" json:"download_url,omitempty"`
	Error       *string    `db:"error" json:"error,omitempty"`
	RequestedAt time.Time  `db:"requested_at" json:"requested_at"`
	SentAt      *time.Time `db:"sent_at" json:"sent_at,omitempty"`
}

func (db *DB) CreateQueuedUserDataExport(ctx context.Context, userID int) (*UserDataExportRow, error) {
	rows, err := db.Pool.Query(ctx, `INSERT INTO user_data_exports (user_id, status) VALUES ($1, 'queued')
		RETURNING id, user_id, status, object_key, download_url, error, requested_at, sent_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[UserDataExportRow])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (db *DB) LatestUserDataExport(ctx context.Context, userID int) (*UserDataExportRow, error) {
	rows, err := db.Pool.Query(ctx, `SELECT id, user_id, status, object_key, download_url, error, requested_at, sent_at
		FROM user_data_exports WHERE user_id = $1 ORDER BY requested_at DESC LIMIT 1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[UserDataExportRow])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (db *DB) FindExportBlocker(ctx context.Context, userID int, now time.Time) (*UserDataExportRow, error) {
	cutoff := now.Add(-24 * time.Hour)
	rows, err := db.Pool.Query(ctx, `SELECT id, user_id, status, object_key, download_url, error, requested_at, sent_at
		FROM user_data_exports
		WHERE user_id = $1
		  AND (status IN ('queued','running') OR (status = 'sent' AND sent_at IS NOT NULL AND sent_at > $2))
		ORDER BY
		  CASE WHEN status IN ('queued','running') THEN 0 ELSE 1 END,
		  requested_at DESC
		LIMIT 1`, userID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[UserDataExportRow])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (db *DB) SetUserDataExportRunning(ctx context.Context, id int64) error {
	_, err := db.Pool.Exec(ctx, `UPDATE user_data_exports SET status = 'running' WHERE id = $1`, id)
	return err
}

func (db *DB) SetUserDataExportSent(ctx context.Context, id int64, objectKey, downloadURL string) error {
	_, err := db.Pool.Exec(ctx, `UPDATE user_data_exports
		SET status = 'sent', object_key = $2, download_url = $3, sent_at = now(), error = NULL
		WHERE id = $1`, id, objectKey, downloadURL)
	return err
}

func (db *DB) SetUserDataExportFailed(ctx context.Context, id int64, errMsg string) error {
	_, err := db.Pool.Exec(ctx, `UPDATE user_data_exports SET status = 'failed', error = $2 WHERE id = $1`, id, errMsg)
	return err
}
