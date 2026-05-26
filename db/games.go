package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ── Row types ──────────────────────────────────────────────────────────────

type GamePassageRow struct {
	ID            int64     `db:"id"`
	ChapterStart  int       `db:"chapter_start"`
	VerseStart    int       `db:"verse_start"`
	ChapterEnd    int       `db:"chapter_end"`
	VerseEnd      int       `db:"verse_end"`
	Label         string    `db:"label"`
	Themes        []string  `db:"themes"`
	Rationale     string    `db:"rationale"`
	LlmDifficulty *string   `db:"llm_difficulty"`
	LlmBlankHint  *int      `db:"llm_blank_hint"`
	Status        string    `db:"status"`
	Source        string    `db:"source"`
	ApprovedAt    *time.Time `db:"approved_at"`
	ApprovedBy    *int      `db:"approved_by"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

type GameVariantRow struct {
	ID         int64           `db:"id"`
	VariantID  string          `db:"variant_id"`
	PassageID  int64           `db:"passage_id"`
	Language   string          `db:"language"`
	Difficulty string          `db:"difficulty"`
	Size       string          `db:"size"`
	Counter    int             `db:"counter"`
	Seed       int64           `db:"seed"`
	Blanks     json.RawMessage `db:"blanks"`
	Rendered   json.RawMessage `db:"rendered"`
	CreatedAt  time.Time       `db:"created_at"`
}

type GameAttemptRow struct {
	ID           int64           `db:"id"`
	AttemptID    string          `db:"attempt_id"`
	UserID       int             `db:"user_id"`
	VariantID    string          `db:"variant_id"`
	Size         string          `db:"size"`
	Difficulty   string          `db:"difficulty"`
	CorrectCount int             `db:"correct_count"`
	TotalCount   int             `db:"total_count"`
	HintsUsed    int             `db:"hints_used"`
	Score        int             `db:"score"`
	ElapsedMs    *int            `db:"elapsed_ms"`
	PerBlank     json.RawMessage `db:"per_blank"`
	CompletedAt  time.Time       `db:"completed_at"`
}

// ── Passage queries ────────────────────────────────────────────────────────

const passageCols = `p.id, p.chapter_start, p.verse_start, p.chapter_end, p.verse_end,
	p.label, p.themes, p.rationale, p.llm_difficulty, p.llm_blank_hint,
	p.status, p.source, p.approved_at, p.approved_by, p.created_at, p.updated_at`

// FetchApprovedPassages lists approved passages, optionally filtered by theme
// and/or chapter. Returned in (chapter_start, verse_start, id) order so the
// catalog is stable for clients.
func (db *DB) FetchApprovedPassages(ctx context.Context, theme *string, chapter *int) ([]GamePassageRow, error) {
	defer observe("fetch_approved_game_passages", time.Now())

	var b strings.Builder
	b.WriteString(`SELECT `)
	b.WriteString(passageCols)
	b.WriteString(` FROM game_passages p WHERE p.status = 'approved'`)

	args := []any{}
	if theme != nil && *theme != "" {
		args = append(args, *theme)
		fmt.Fprintf(&b, ` AND $%d = ANY(p.themes)`, len(args))
	}
	if chapter != nil {
		args = append(args, *chapter)
		fmt.Fprintf(&b, ` AND p.chapter_start <= $%d AND p.chapter_end >= $%d`, len(args), len(args))
	}
	b.WriteString(` ORDER BY p.chapter_start, p.verse_start, p.id`)

	rows, err := db.Pool.Query(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[GamePassageRow])
}

func (db *DB) FetchPassageByID(ctx context.Context, id int64) (*GamePassageRow, error) {
	defer observe("fetch_game_passage_by_id", time.Now())

	rows, err := db.Pool.Query(ctx,
		`SELECT `+passageCols+` FROM game_passages p WHERE p.id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[GamePassageRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// ── Variant queries ────────────────────────────────────────────────────────

const variantCols = `v.id, v.variant_id, v.passage_id, v.language, v.difficulty,
	v.size, v.counter, v.seed, v.blanks, v.rendered, v.created_at`

func (db *DB) FetchVariantByVariantID(ctx context.Context, variantID string) (*GameVariantRow, error) {
	defer observe("fetch_game_variant_by_variant_id", time.Now())

	rows, err := db.Pool.Query(ctx,
		`SELECT `+variantCols+` FROM game_variants v WHERE v.variant_id = $1`, variantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[GameVariantRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// FetchNextUnseenVariant returns the lowest-counter variant for the given
// (passage, language, difficulty, size) that the user (resolved via auth_id)
// has not yet submitted. Returns nil if every existing variant has been
// attempted by the user — the caller is then expected to mint a new variant.
func (db *DB) FetchNextUnseenVariant(ctx context.Context, authID string, passageID int64, language, difficulty, size string) (*GameVariantRow, error) {
	defer observe("fetch_next_unseen_game_variant", time.Now())

	query := `SELECT ` + variantCols + `
	          FROM game_variants v
	          WHERE v.passage_id = $1
	            AND v.language   = $2
	            AND v.difficulty = $3
	            AND v.size       = $4
	            AND NOT EXISTS (
	                SELECT 1 FROM game_attempts a
	                JOIN users u ON u.id = a.user_id
	                WHERE u.auth_id = $5 AND a.variant_id = v.variant_id
	            )
	          ORDER BY v.counter ASC
	          LIMIT 1`
	rows, err := db.Pool.Query(ctx, query, passageID, language, difficulty, size, authID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[GameVariantRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// MaxVariantCounter returns the highest counter assigned in the
// (passage, language, difficulty, size) group, or 0 if none exist.
func (db *DB) MaxVariantCounter(ctx context.Context, passageID int64, language, difficulty, size string) (int, error) {
	defer observe("max_game_variant_counter", time.Now())

	var counter *int
	err := db.Pool.QueryRow(ctx,
		`SELECT MAX(counter) FROM game_variants
		 WHERE passage_id = $1 AND language = $2 AND difficulty = $3 AND size = $4`,
		passageID, language, difficulty, size).Scan(&counter)
	if err != nil {
		return 0, err
	}
	if counter == nil {
		return 0, nil
	}
	return *counter, nil
}

func (db *DB) InsertVariant(ctx context.Context, v *GameVariantRow) (*GameVariantRow, error) {
	defer observe("insert_game_variant", time.Now())

	query := `INSERT INTO game_variants
	          (variant_id, passage_id, language, difficulty, size, counter, seed, blanks, rendered)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	          RETURNING ` + variantCols
	rows, err := db.Pool.Query(ctx, query,
		v.VariantID, v.PassageID, v.Language, v.Difficulty, v.Size,
		v.Counter, v.Seed, v.Blanks, v.Rendered)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[GameVariantRow])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ── Attempt queries ────────────────────────────────────────────────────────

const attemptCols = `a.id, a.attempt_id, a.user_id, a.variant_id, a.size, a.difficulty,
	a.correct_count, a.total_count, a.hints_used, a.score, a.elapsed_ms,
	a.per_blank, a.completed_at`

// InsertAttempt records a completed submission. user_id is resolved from the
// authID via users.auth_id so callers never have to know the integer id.
func (db *DB) InsertAttempt(ctx context.Context, authID string, a *GameAttemptRow) (*GameAttemptRow, error) {
	defer observe("insert_game_attempt", time.Now())

	query := `INSERT INTO game_attempts
	          (attempt_id, user_id, variant_id, size, difficulty,
	           correct_count, total_count, hints_used, score, elapsed_ms, per_blank)
	          SELECT $1, u.id, $2, $3, $4, $5, $6, $7, $8, $9, $10
	          FROM users u WHERE u.auth_id = $11
	          RETURNING ` + attemptCols
	rows, err := db.Pool.Query(ctx, query,
		a.AttemptID, a.VariantID, a.Size, a.Difficulty,
		a.CorrectCount, a.TotalCount, a.HintsUsed, a.Score, a.ElapsedMs, a.PerBlank,
		authID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[GameAttemptRow])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// FetchUserAttempts returns the user's recent attempts (newest first), with
// keyset pagination by completed_at. cursor is the completed_at of the last
// row from the previous page (RFC3339); nil for the first page.
func (db *DB) FetchUserAttempts(ctx context.Context, authID string, limit int, cursor *time.Time) ([]GameAttemptRow, error) {
	defer observe("fetch_user_game_attempts", time.Now())

	var b strings.Builder
	b.WriteString(`SELECT `)
	b.WriteString(attemptCols)
	b.WriteString(` FROM game_attempts a
	            JOIN users u ON u.id = a.user_id
	            WHERE u.auth_id = $1`)
	args := []any{authID}
	if cursor != nil {
		args = append(args, *cursor)
		fmt.Fprintf(&b, ` AND a.completed_at < $%d`, len(args))
	}
	args = append(args, limit)
	fmt.Fprintf(&b, ` ORDER BY a.completed_at DESC LIMIT $%d`, len(args))

	rows, err := db.Pool.Query(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[GameAttemptRow])
}

// ── Stats ──────────────────────────────────────────────────────────────────

type GameStatsByDifficulty struct {
	Difficulty string `db:"difficulty"`
	Attempts   int    `db:"attempts"`
	BestScore  int    `db:"best_score"`
}

type GameUserStats struct {
	TotalAttempts     int                     `db:"total_attempts"`
	BestScore         int                     `db:"best_score"`
	VariantsCompleted int                     `db:"variants_completed"`
	ByDifficulty      []GameStatsByDifficulty `db:"-"`
}

func (db *DB) FetchUserGameStats(ctx context.Context, authID string) (*GameUserStats, error) {
	defer observe("fetch_user_game_stats", time.Now())

	var stats GameUserStats
	err := db.Pool.QueryRow(ctx, `
		SELECT COALESCE(COUNT(*), 0)::int                          AS total_attempts,
		       COALESCE(MAX(a.score), 0)::int                      AS best_score,
		       COALESCE(COUNT(DISTINCT a.variant_id), 0)::int      AS variants_completed
		FROM game_attempts a
		JOIN users u ON u.id = a.user_id
		WHERE u.auth_id = $1
	`, authID).Scan(&stats.TotalAttempts, &stats.BestScore, &stats.VariantsCompleted)
	if err != nil {
		return nil, err
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT a.difficulty,
		       COUNT(*)::int        AS attempts,
		       MAX(a.score)::int    AS best_score
		FROM game_attempts a
		JOIN users u ON u.id = a.user_id
		WHERE u.auth_id = $1
		GROUP BY a.difficulty
	`, authID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	by, err := pgx.CollectRows(rows, pgx.RowToStructByName[GameStatsByDifficulty])
	if err != nil {
		return nil, err
	}
	stats.ByDifficulty = by
	return &stats, nil
}

// ── Leaderboard ────────────────────────────────────────────────────────────

type GameLeaderboardRow struct {
	UserID      int       `db:"user_id"`
	DisplayName *string   `db:"display_name"`
	Score       int       `db:"score"`
	Size        string    `db:"size"`
	Difficulty  string    `db:"difficulty"`
	ElapsedMs   *int      `db:"elapsed_ms"`
	CompletedAt time.Time `db:"completed_at"`
	VariantID   string    `db:"variant_id"`
}

// FetchGlobalLeaderboard returns each user's best attempt (by score) for the
// given (size, difficulty), ordered score DESC, elapsed_ms ASC.
func (db *DB) FetchGlobalLeaderboard(ctx context.Context, size, difficulty string, limit int) ([]GameLeaderboardRow, error) {
	defer observe("fetch_global_game_leaderboard", time.Now())

	query := `
		SELECT DISTINCT ON (a.user_id)
		       a.user_id, u.display_name, a.score, a.size, a.difficulty,
		       a.elapsed_ms, a.completed_at, a.variant_id
		FROM game_attempts a
		JOIN users u ON u.id = a.user_id
		WHERE a.size = $1 AND a.difficulty = $2
		ORDER BY a.user_id, a.score DESC, a.elapsed_ms ASC NULLS LAST
	`
	// Wrap to apply final ordering and limit.
	wrapped := `SELECT * FROM (` + query + `) t
	           ORDER BY t.score DESC, t.elapsed_ms ASC NULLS LAST, t.completed_at ASC
	           LIMIT $3`
	rows, err := db.Pool.Query(ctx, wrapped, size, difficulty, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[GameLeaderboardRow])
}

func (db *DB) FetchWeeklyLeaderboard(ctx context.Context, size, difficulty string, limit int) ([]GameLeaderboardRow, error) {
	defer observe("fetch_weekly_game_leaderboard", time.Now())

	query := `
		SELECT DISTINCT ON (a.user_id)
		       a.user_id, u.display_name, a.score, a.size, a.difficulty,
		       a.elapsed_ms, a.completed_at, a.variant_id
		FROM game_attempts a
		JOIN users u ON u.id = a.user_id
		WHERE a.size = $1 AND a.difficulty = $2
		  AND a.completed_at >= NOW() - INTERVAL '7 days'
		ORDER BY a.user_id, a.score DESC, a.elapsed_ms ASC NULLS LAST
	`
	wrapped := `SELECT * FROM (` + query + `) t
	           ORDER BY t.score DESC, t.elapsed_ms ASC NULLS LAST, t.completed_at ASC
	           LIMIT $3`
	rows, err := db.Pool.Query(ctx, wrapped, size, difficulty, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[GameLeaderboardRow])
}

func (db *DB) FetchVariantLeaderboard(ctx context.Context, variantID string, limit int) ([]GameLeaderboardRow, error) {
	defer observe("fetch_variant_game_leaderboard", time.Now())

	query := `
		SELECT DISTINCT ON (a.user_id)
		       a.user_id, u.display_name, a.score, a.size, a.difficulty,
		       a.elapsed_ms, a.completed_at, a.variant_id
		FROM game_attempts a
		JOIN users u ON u.id = a.user_id
		WHERE a.variant_id = $1
		ORDER BY a.user_id, a.score DESC, a.elapsed_ms ASC NULLS LAST
	`
	wrapped := `SELECT * FROM (` + query + `) t
	           ORDER BY t.score DESC, t.elapsed_ms ASC NULLS LAST, t.completed_at ASC
	           LIMIT $2`
	rows, err := db.Pool.Query(ctx, wrapped, variantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[GameLeaderboardRow])
}
