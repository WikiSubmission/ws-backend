package db

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"time"
)

// seedFiles embeds every lemma fixture that exists at compile time.
// New languages are supported by adding db/seeds/games_lemma_<lang>.sql and
// recompiling — no code change required.
//
//go:embed seeds/games_lemma_*.sql
var seedFiles embed.FS

// LoadLemmaFixture loads the English lemma fixture. Kept for backward compat.
func (db *DB) LoadLemmaFixture(ctx context.Context) (int, error) {
	return db.LoadLemmaFixtureFor(ctx, "en")
}

// LoadLemmaFixtureFor executes the embedded lemma fixture for the given language
// and returns the resulting row count. Returns 0, nil when no fixture has been
// shipped for that language (Hard-tier acceptance falls back to exact match).
// The fixture is idempotent: DELETE for the language, then upsert.
func (db *DB) LoadLemmaFixtureFor(ctx context.Context, language string) (int, error) {
	defer observe("load_lemma_fixture", time.Now())

	content, err := seedFiles.ReadFile(fmt.Sprintf("seeds/games_lemma_%s.sql", language))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read embedded lemma fixture: %w", err)
	}
	if _, err := db.Pool.Exec(ctx, string(content)); err != nil {
		return 0, fmt.Errorf("exec lemma fixture: %w", err)
	}

	var count int
	if err := db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM game_lemma WHERE language = $1`, language).Scan(&count); err != nil {
		return 0, fmt.Errorf("count lemma rows: %w", err)
	}
	return count, nil
}
