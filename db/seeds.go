package db

import (
	"context"
	"embed"
	"fmt"
	"time"
)

// seedFiles embeds reference-data seed fixtures shipped with the binary so they
// can be loaded at runtime without psql or external tooling. Unlike migrations,
// these are loaded on demand (e.g. from an admin endpoint), not at startup.
//
//go:embed seeds/games_lemma_en.sql
var seedFiles embed.FS

// LoadLemmaFixture executes the embedded English lemma fixture and returns the
// resulting game_lemma row count for the language. The fixture wraps its own
// transaction and is idempotent (DELETE for the language, then upserts). Until
// the spaCy-generated fixture is committed the embedded file is a no-op and
// Hard-tier acceptance falls back to exact match.
func (db *DB) LoadLemmaFixture(ctx context.Context) (int, error) {
	defer observe("load_lemma_fixture", time.Now())

	content, err := seedFiles.ReadFile("seeds/games_lemma_en.sql")
	if err != nil {
		return 0, fmt.Errorf("read embedded lemma fixture: %w", err)
	}
	if _, err := db.Pool.Exec(ctx, string(content)); err != nil {
		return 0, fmt.Errorf("exec lemma fixture: %w", err)
	}

	var count int
	if err := db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM game_lemma WHERE language = 'en'`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count lemma rows: %w", err)
	}
	return count, nil
}
