package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
)

// ── Row types ──────────────────────────────────────────────────────────────

type GamePassageRow struct {
	ID            int64      `db:"id"`
	ChapterStart  int        `db:"chapter_start"`
	VerseStart    int        `db:"verse_start"`
	ChapterEnd    int        `db:"chapter_end"`
	VerseEnd      int        `db:"verse_end"`
	Label         string     `db:"label"`
	Themes        []string   `db:"themes"`
	Rationale     string     `db:"rationale"`
	LlmDifficulty *string    `db:"llm_difficulty"`
	LlmBlankHint  *int       `db:"llm_blank_hint"`
	Status        string     `db:"status"`
	Source        string     `db:"source"`
	ApprovedAt    *time.Time `db:"approved_at"`
	ApprovedBy    *int       `db:"approved_by"`
	CreatedAt     time.Time  `db:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at"`
}

type GameVariantRow struct {
	ID         int64           `db:"id"`
	VariantID  string          `db:"variant_id"`
	PassageID  int64           `db:"passage_id"`
	Language   string          `db:"language"`
	Difficulty string          `db:"difficulty"`
	Size       string          `db:"size"`
	SkillBand  string          `db:"skill_band"`
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

// FetchPassagesForReview lists passages for the editorial workflow, optionally
// filtered by status and/or chapter. Unlike FetchApprovedPassages this returns
// every status and includes the editor-only fields. Ordered newest first.
func (db *DB) FetchPassagesForReview(ctx context.Context, status *string, chapter *int) ([]GamePassageRow, error) {
	defer observe("fetch_game_passages_for_review", time.Now())

	var b strings.Builder
	b.WriteString(`SELECT `)
	b.WriteString(passageCols)
	b.WriteString(` FROM game_passages p WHERE 1=1`)

	args := []any{}
	if status != nil && *status != "" {
		args = append(args, *status)
		fmt.Fprintf(&b, ` AND p.status = $%d`, len(args))
	}
	if chapter != nil {
		args = append(args, *chapter)
		fmt.Fprintf(&b, ` AND p.chapter_start <= $%d AND p.chapter_end >= $%d`, len(args), len(args))
	}
	b.WriteString(` ORDER BY p.created_at DESC, p.id DESC`)

	rows, err := db.Pool.Query(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[GamePassageRow])
}

// SetPassageStatus updates a passage's review status. When the new status is
// 'approved', approved_at is set to now() and approved_by resolves from
// editorEmail (when provided and the email maps to a users row); for any other
// status both approval fields are cleared. Returns false when no row matched.
func (db *DB) SetPassageStatus(ctx context.Context, id int64, status string, editorEmail *string) (bool, error) {
	defer observe("set_game_passage_status", time.Now())

	if status == "approved" {
		tag, err := db.Pool.Exec(ctx, `
			UPDATE game_passages
			SET status = $2,
			    approved_at = now(),
			    approved_by = (SELECT id FROM users WHERE email = $3),
			    updated_at = now()
			WHERE id = $1
		`, id, status, editorEmail)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil
	}

	tag, err := db.Pool.Exec(ctx, `
		UPDATE game_passages
		SET status = $2,
		    approved_at = NULL,
		    approved_by = NULL,
		    updated_at = now()
		WHERE id = $1
	`, id, status)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
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
	v.size, v.skill_band, v.counter, v.seed, v.blanks, v.rendered, v.created_at`

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
// (passage, language, difficulty, size, skillBand) that the user (resolved
// via auth_id) has not yet submitted. Returns nil if every existing variant
// in the band has been attempted — the caller is then expected to mint a new one.
func (db *DB) FetchNextUnseenVariant(ctx context.Context, authID string, passageID int64, language, difficulty, size, skillBand string) (*GameVariantRow, error) {
	defer observe("fetch_next_unseen_game_variant", time.Now())

	query := `SELECT ` + variantCols + `
	          FROM game_variants v
	          WHERE v.passage_id = $1
	            AND v.language   = $2
	            AND v.difficulty = $3
	            AND v.size       = $4
	            AND v.skill_band = $5
	            AND NOT EXISTS (
	                SELECT 1 FROM game_attempts a
	                JOIN users u ON u.id = a.user_id
	                WHERE u.auth_id = $6 AND a.variant_id = v.variant_id
	            )
	          ORDER BY v.counter ASC
	          LIMIT 1`
	rows, err := db.Pool.Query(ctx, query, passageID, language, difficulty, size, skillBand, authID)
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
// (passage, language, difficulty, size, skillBand) group, or 0 if none exist.
func (db *DB) MaxVariantCounter(ctx context.Context, passageID int64, language, difficulty, size, skillBand string) (int, error) {
	defer observe("max_game_variant_counter", time.Now())

	var counter *int
	err := db.Pool.QueryRow(ctx,
		`SELECT MAX(counter) FROM game_variants
		 WHERE passage_id = $1 AND language = $2 AND difficulty = $3 AND size = $4 AND skill_band = $5`,
		passageID, language, difficulty, size, skillBand).Scan(&counter)
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

	// Alias the target as v so the shared variantCols (v.*) resolve in RETURNING.
	query := `INSERT INTO game_variants AS v
	          (variant_id, passage_id, language, difficulty, size, skill_band, counter, seed, blanks, rendered)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	          RETURNING ` + variantCols
	rows, err := db.Pool.Query(ctx, query,
		v.VariantID, v.PassageID, v.Language, v.Difficulty, v.Size, v.SkillBand,
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

	// Alias the target as a so the shared attemptCols (a.*) resolve in RETURNING.
	query := `INSERT INTO game_attempts AS a
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

// RecentAccuracyResult holds aggregated accuracy data for adaptive difficulty.
type RecentAccuracyResult struct {
	Count        int     `db:"count"`
	MeanAccuracy float64 `db:"mean_accuracy"`
}

// FetchRecentAccuracy returns the count and mean accuracy for the user's last n
// attempts at the given difficulty tier. Used to compute their skill band.
func (db *DB) FetchRecentAccuracy(ctx context.Context, authID, difficulty string, n int) (*RecentAccuracyResult, error) {
	defer observe("fetch_recent_accuracy", time.Now())

	var res RecentAccuracyResult
	err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)::int                                          AS count,
		       COALESCE(AVG(acc), 0.0)::float                        AS mean_accuracy
		FROM (
		    SELECT a.correct_count::float / NULLIF(a.total_count, 0) AS acc
		    FROM game_attempts a
		    JOIN users u ON u.id = a.user_id
		    WHERE u.auth_id = $1 AND a.difficulty = $2
		    ORDER BY a.completed_at DESC
		    LIMIT $3
		) sub
	`, authID, difficulty, n).Scan(&res.Count, &res.MeanAccuracy)
	if err != nil {
		return nil, err
	}
	return &res, nil
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

// FetchGlobalLeaderboard returns each user's best attempt (by score) across
// every (size, difficulty) combination, ordered score DESC, elapsed_ms ASC.
// Difficulty multipliers applied at submit time make scores comparable across
// tiers. When size or difficulty is non-empty the result is filtered to that
// segment; pass "" for an unsegmented board.
func (db *DB) FetchGlobalLeaderboard(ctx context.Context, size, difficulty string, limit int) ([]GameLeaderboardRow, error) {
	defer observe("fetch_global_game_leaderboard", time.Now())

	inner, args := buildLeaderboardInner(size, difficulty, "")
	wrapped := `SELECT * FROM (` + inner + `) t
	           ORDER BY t.score DESC, t.elapsed_ms ASC NULLS LAST, t.completed_at ASC
	           LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit)
	rows, err := db.Pool.Query(ctx, wrapped, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[GameLeaderboardRow])
}

func (db *DB) FetchWeeklyLeaderboard(ctx context.Context, size, difficulty string, limit int) ([]GameLeaderboardRow, error) {
	defer observe("fetch_weekly_game_leaderboard", time.Now())

	inner, args := buildLeaderboardInner(size, difficulty, "a.completed_at >= NOW() - INTERVAL '7 days'")
	wrapped := `SELECT * FROM (` + inner + `) t
	           ORDER BY t.score DESC, t.elapsed_ms ASC NULLS LAST, t.completed_at ASC
	           LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit)
	rows, err := db.Pool.Query(ctx, wrapped, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[GameLeaderboardRow])
}

func buildLeaderboardInner(size, difficulty, extraWhere string) (string, []any) {
	args := make([]any, 0, 3)
	clauses := make([]string, 0, 3)
	if size != "" {
		args = append(args, size)
		clauses = append(clauses, "a.size = $"+strconv.Itoa(len(args)))
	}
	if difficulty != "" {
		args = append(args, difficulty)
		clauses = append(clauses, "a.difficulty = $"+strconv.Itoa(len(args)))
	}
	if extraWhere != "" {
		clauses = append(clauses, extraWhere)
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	// Fall back to the local-part of the email when display_name is not set,
	// so users who never set a name still surface as something recognisable
	// (e.g. "hesham" rather than "anonymous"). The domain is dropped to avoid
	// exposing the full address on a public leaderboard.
	return `
		SELECT DISTINCT ON (a.user_id)
		       a.user_id,
		       COALESCE(NULLIF(u.display_name, ''), split_part(u.email, '@', 1)) AS display_name,
		       a.score, a.size, a.difficulty,
		       a.elapsed_ms, a.completed_at, a.variant_id
		FROM game_attempts a
		JOIN users u ON u.id = a.user_id
		` + where + `
		ORDER BY a.user_id, a.score DESC, a.elapsed_ms ASC NULLS LAST
	`, args
}

// RebuildWordFrequency tokenizes every verse_text row for the language,
// normalizes each token (lowercase, letters and digits only), counts
// occurrences, and replaces game_word_frequency for that language in a single
// transaction. Returns the number of distinct tokens written. Idempotent.
func (db *DB) RebuildWordFrequency(ctx context.Context, language string) (int, error) {
	defer observe("rebuild_word_frequency", time.Now())

	rows, err := db.Pool.Query(ctx, `
		SELECT vt.content
		FROM verse_text vt
		JOIN languages l ON l.id = vt.lang_id
		WHERE l.code = $1
	`, language)
	if err != nil {
		return 0, fmt.Errorf("read verse_text: %w", err)
	}
	counts := make(map[string]int)
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan verse_text: %w", err)
		}
		for _, tok := range frequencyTokens(content) {
			counts[tok]++
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate verse_text: %w", err)
	}
	rows.Close()

	if len(counts) == 0 {
		return 0, fmt.Errorf("no tokens collected for language %q (is verse_text populated?)", language)
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `DELETE FROM game_word_frequency WHERE language = $1`, language); err != nil {
		return 0, fmt.Errorf("clear frequencies: %w", err)
	}

	batch := &pgx.Batch{}
	for word, freq := range counts {
		batch.Queue(
			`INSERT INTO game_word_frequency (language, word_normalized, frequency) VALUES ($1, $2, $3)`,
			language, word, freq)
	}
	br := tx.SendBatch(ctx, batch)
	for range counts {
		if _, err := br.Exec(); err != nil {
			br.Close() //nolint:errcheck
			return 0, fmt.Errorf("insert frequency: %w", err)
		}
	}
	if err := br.Close(); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(counts), nil
}

// frequencyTokens splits text into normalized word tokens: runs of letters and
// digits, lowercased. Matches the keys used in game_word_frequency and the
// normalization applied to guesses at submit time.
func frequencyTokens(s string) []string {
	out := []string{}
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		} else {
			flush()
		}
	}
	flush()
	return out
}

// FetchFrequencies returns corpus counts for the given normalized words.
// Missing words are absent from the map (the generator treats them as freq=1).
func (db *DB) FetchFrequencies(ctx context.Context, language string, words []string) (map[string]int, error) {
	defer observe("fetch_game_frequencies", time.Now())
	out := make(map[string]int, len(words))
	if len(words) == 0 {
		return out, nil
	}
	rows, err := db.Pool.Query(ctx,
		`SELECT word_normalized, frequency
		 FROM game_word_frequency
		 WHERE language = $1 AND word_normalized = ANY($2)`,
		language, words)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var w string
		var f int
		if err := rows.Scan(&w, &f); err != nil {
			return nil, err
		}
		out[w] = f
	}
	return out, rows.Err()
}

// FetchLemmasFor returns lemma assignments for the given normalized words.
// Missing words are absent from the map.
func (db *DB) FetchLemmasFor(ctx context.Context, language string, words []string) (map[string]string, error) {
	defer observe("fetch_game_lemmas", time.Now())
	out := make(map[string]string, len(words))
	if len(words) == 0 {
		return out, nil
	}
	rows, err := db.Pool.Query(ctx,
		`SELECT word_normalized, lemma
		 FROM game_lemma
		 WHERE language = $1 AND word_normalized = ANY($2)`,
		language, words)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var w, lemma string
		if err := rows.Scan(&w, &lemma); err != nil {
			return nil, err
		}
		out[w] = lemma
	}
	return out, rows.Err()
}

// FetchChapterVerseContents returns every verse_text.content for a chapter in
// the given language, in canonical order. Used by the variant generator to
// build the easy-tier distractor pool.
func (db *DB) FetchChapterVerseContents(ctx context.Context, language string, chapter int) ([]string, error) {
	defer observe("fetch_chapter_verse_contents", time.Now())
	rows, err := db.Pool.Query(ctx, `
		SELECT vt.content
		FROM verse_text vt
		JOIN verses v ON v.id = vt.verse_id
		JOIN chapters c ON c.id = v.chapter_id
		JOIN languages l ON l.id = vt.lang_id
		WHERE c.chapter_number = $1 AND l.code = $2
		ORDER BY v.verse_index
	`, chapter, language)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// OldestVariantInGroup returns the lowest-counter variant in the band group.
// Used for the replay-cycle fallback when the user has exhausted the catalog.
func (db *DB) OldestVariantInGroup(ctx context.Context, passageID int64, language, difficulty, size, skillBand string) (*GameVariantRow, error) {
	defer observe("fetch_oldest_variant_in_group", time.Now())
	query := `SELECT ` + variantCols + `
	          FROM game_variants v
	          WHERE v.passage_id = $1 AND v.language = $2 AND v.difficulty = $3
	            AND v.size = $4 AND v.skill_band = $5
	          ORDER BY v.counter ASC LIMIT 1`
	rows, err := db.Pool.Query(ctx, query, passageID, language, difficulty, size, skillBand)
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

func (db *DB) FetchVariantLeaderboard(ctx context.Context, variantID string, limit int) ([]GameLeaderboardRow, error) {
	defer observe("fetch_variant_game_leaderboard", time.Now())

	query := `
		SELECT DISTINCT ON (a.user_id)
		       a.user_id,
		       COALESCE(NULLIF(u.display_name, ''), split_part(u.email, '@', 1)) AS display_name,
		       a.score, a.size, a.difficulty,
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
