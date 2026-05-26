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
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	logQuery := `INSERT INTO reading_log (user_id, scripture, day, verses_read)
	             SELECT u.id, $2, $3::date, $4 FROM users u WHERE u.auth_id = $1
	             ON CONFLICT (user_id, scripture, day) DO UPDATE SET verses_read = reading_log.verses_read + EXCLUDED.verses_read`
	if _, err := tx.Exec(ctx, logQuery, authID, scripture, day, versesRead); err != nil {
		return err
	}

	eventQuery := `INSERT INTO reading_events (user_id, scripture, verses_read)
	               SELECT u.id, $2, $3 FROM users u WHERE u.auth_id = $1`
	if _, err := tx.Exec(ctx, eventQuery, authID, scripture, versesRead); err != nil {
		return err
	}

	return tx.Commit(ctx)
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

// ── Reading stats (analytics) ─────────────────────────────────────────────

type ReadingStatsDaily struct {
	Day        string `db:"day" json:"day"`
	VersesRead int    `db:"verses_read" json:"verses_read"`
}

type ReadingStatsHourly struct {
	Hour       int `db:"hour" json:"hour"`
	VersesRead int `db:"verses_read" json:"verses_read"`
}

type ReadingStatsWeekday struct {
	Weekday    int `db:"weekday" json:"weekday"`
	VersesRead int `db:"verses_read" json:"verses_read"`
}

type ReadingStatsResult struct {
	Total      int                   `json:"total"`
	ActiveDays int                   `json:"active_days"`
	BestDay    *ReadingStatsDaily    `json:"best_day,omitempty"`
	Daily      []ReadingStatsDaily   `json:"daily"`
	Hourly     []ReadingStatsHourly  `json:"hourly"`
	Weekday    []ReadingStatsWeekday `json:"weekday"`
	Cumulative []ReadingStatsDaily   `json:"cumulative"`
}

// GetReadingStats returns aggregated reading metrics from reading_events for the
// given range. `since` is inclusive lower bound; pass zero time for "all time".
// `tz` is the IANA timezone used for day/hour/weekday bucketing.
func (db *DB) GetReadingStats(ctx context.Context, authID, scripture, tz string, since time.Time) (*ReadingStatsResult, error) {
	if tz == "" {
		tz = "UTC"
	}
	hasSince := !since.IsZero()

	dailyQuery := `WITH ev AS (
	                 SELECT e.ts, e.verses_read
	                 FROM reading_events e
	                 JOIN users u ON u.id = e.user_id
	                 WHERE u.auth_id = $1
	                   AND e.scripture = $2
	                   AND ($4::timestamptz IS NULL OR e.ts >= $4)
	               )
	               SELECT to_char((ts AT TIME ZONE $3)::date, 'YYYY-MM-DD') AS day,
	                      SUM(verses_read)::int AS verses_read
	               FROM ev
	               GROUP BY 1
	               ORDER BY 1`
	hourlyQuery := `WITH ev AS (
	                  SELECT e.ts, e.verses_read
	                  FROM reading_events e
	                  JOIN users u ON u.id = e.user_id
	                  WHERE u.auth_id = $1
	                    AND e.scripture = $2
	                    AND ($4::timestamptz IS NULL OR e.ts >= $4)
	                )
	                SELECT EXTRACT(HOUR FROM (ts AT TIME ZONE $3))::int AS hour,
	                       SUM(verses_read)::int AS verses_read
	                FROM ev
	                GROUP BY 1
	                ORDER BY 1`
	weekdayQuery := `WITH ev AS (
	                   SELECT e.ts, e.verses_read
	                   FROM reading_events e
	                   JOIN users u ON u.id = e.user_id
	                   WHERE u.auth_id = $1
	                     AND e.scripture = $2
	                     AND ($4::timestamptz IS NULL OR e.ts >= $4)
	                 )
	                 SELECT EXTRACT(ISODOW FROM (ts AT TIME ZONE $3))::int AS weekday,
	                        SUM(verses_read)::int AS verses_read
	                 FROM ev
	                 GROUP BY 1
	                 ORDER BY 1`

	var sinceArg interface{}
	if hasSince {
		sinceArg = since
	} else {
		sinceArg = nil
	}

	result := &ReadingStatsResult{
		Daily:      []ReadingStatsDaily{},
		Hourly:     []ReadingStatsHourly{},
		Weekday:    []ReadingStatsWeekday{},
		Cumulative: []ReadingStatsDaily{},
	}

	// daily
	rows, err := db.Pool.Query(ctx, dailyQuery, authID, scripture, tz, sinceArg)
	if err != nil {
		return nil, err
	}
	daily, err := pgx.CollectRows(rows, pgx.RowToStructByName[ReadingStatsDaily])
	if err != nil {
		return nil, err
	}
	result.Daily = daily

	// hourly
	rows, err = db.Pool.Query(ctx, hourlyQuery, authID, scripture, tz, sinceArg)
	if err != nil {
		return nil, err
	}
	hourly, err := pgx.CollectRows(rows, pgx.RowToStructByName[ReadingStatsHourly])
	if err != nil {
		return nil, err
	}
	result.Hourly = hourly

	// weekday
	rows, err = db.Pool.Query(ctx, weekdayQuery, authID, scripture, tz, sinceArg)
	if err != nil {
		return nil, err
	}
	weekday, err := pgx.CollectRows(rows, pgx.RowToStructByName[ReadingStatsWeekday])
	if err != nil {
		return nil, err
	}
	result.Weekday = weekday

	// aggregates + cumulative derived from daily
	var running int
	cumulative := make([]ReadingStatsDaily, 0, len(daily))
	for _, d := range daily {
		running += d.VersesRead
		cumulative = append(cumulative, ReadingStatsDaily{Day: d.Day, VersesRead: running})
		result.Total += d.VersesRead
		if d.VersesRead > 0 {
			result.ActiveDays++
			if result.BestDay == nil || d.VersesRead > result.BestDay.VersesRead {
				best := d
				result.BestDay = &best
			}
		}
	}
	result.Cumulative = cumulative

	return result, nil
}
