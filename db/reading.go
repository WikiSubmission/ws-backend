package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// ── Reading progress ──────────────────────────────────────────────────────

type ReadingProgressRow struct {
	UserID    int       `db:"user_id"`
	Scripture string    `db:"scripture"`
	VerseKey  string    `db:"verse_key"`
	UpdatedAt time.Time `db:"updated_at"`
}

func (db *DB) GetReadingProgress(ctx context.Context, authID, scripture string) (*ReadingProgressRow, error) {
	query := `SELECT rp.user_id, rp.scripture, rp.verse_key, rp.updated_at
	          FROM reading_progress rp
	          JOIN users u ON u.id = rp.user_id
	          WHERE u.auth_id = $1 AND rp.scripture = $2`
	rows, err := db.Pool.Query(ctx, query, authID, scripture)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ReadingProgressRow])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (db *DB) UpsertReadingProgress(ctx context.Context, authID, scripture, verseKey string) (*ReadingProgressRow, error) {
	query := `INSERT INTO reading_progress (user_id, scripture, verse_key, updated_at)
	          SELECT u.id, $2, $3, NOW() FROM users u WHERE u.auth_id = $1
	          ON CONFLICT (user_id, scripture) DO UPDATE SET verse_key = EXCLUDED.verse_key, updated_at = NOW()
	          RETURNING user_id, scripture, verse_key, updated_at`
	rows, err := db.Pool.Query(ctx, query, authID, scripture, verseKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ReadingProgressRow])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ── Reading log + streak ──────────────────────────────────────────────────

type ReadingLogRow struct {
	UserID     int    `db:"user_id"`
	Scripture  string `db:"scripture"`
	Day        string `db:"day"`
	VersesRead int    `db:"verses_read"`
}

type StreakResult struct {
	CurrentStreak  int
	LongestStreak  int
	LastActiveDay  string
	TotalVersesRead int
}

func (db *DB) IncrementReadingLog(ctx context.Context, authID, scripture, day string, versesRead int) error {
	query := `INSERT INTO reading_log (user_id, scripture, day, verses_read)
	          SELECT u.id, $2, $3::date, $4 FROM users u WHERE u.auth_id = $1
	          ON CONFLICT (user_id, scripture, day) DO UPDATE SET verses_read = reading_log.verses_read + EXCLUDED.verses_read`
	_, err := db.Pool.Exec(ctx, query, authID, scripture, day, versesRead)
	return err
}

func (db *DB) GetStreak(ctx context.Context, authID, scripture string) (*StreakResult, error) {
	// Compute streak in SQL: ordered list of active days, gap detection.
	query := `WITH days AS (
	            SELECT rl.day
	            FROM reading_log rl
	            JOIN users u ON u.id = rl.user_id
	            WHERE u.auth_id = $1 AND rl.scripture = $2
	            ORDER BY rl.day DESC
	          ),
	          totals AS (
	            SELECT COALESCE(SUM(rl.verses_read), 0)::int AS total
	            FROM reading_log rl
	            JOIN users u ON u.id = rl.user_id
	            WHERE u.auth_id = $1 AND rl.scripture = $2
	          ),
	          gaps AS (
	            SELECT day,
	                   day + (ROW_NUMBER() OVER (ORDER BY day DESC))::int * INTERVAL '1 day' AS grp
	            FROM days
	          ),
	          runs AS (
	            SELECT grp, COUNT(*) AS len, MAX(day) AS last_day
	            FROM gaps
	            GROUP BY grp
	          )
	          SELECT
	            (SELECT len FROM runs ORDER BY last_day DESC LIMIT 1)::int AS current_streak,
	            (SELECT MAX(len)::int FROM runs) AS longest_streak,
	            (SELECT MAX(day)::text FROM days) AS last_active_day,
	            (SELECT total FROM totals) AS total_verses_read`
	row := db.Pool.QueryRow(ctx, query, authID, scripture)
	var cs, ls, tvr *int
	var lad *string
	if err := row.Scan(&cs, &ls, &lad, &tvr); err != nil {
		return nil, err
	}
	result := &StreakResult{}
	if cs != nil {
		result.CurrentStreak = *cs
	}
	if ls != nil {
		result.LongestStreak = *ls
	}
	if lad != nil {
		result.LastActiveDay = *lad
	}
	if tvr != nil {
		result.TotalVersesRead = *tvr
	}
	return result, nil
}
