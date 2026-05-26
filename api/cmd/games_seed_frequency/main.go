// games_seed_frequency populates game_word_frequency for the English
// (Rashad Khalifa) translation.
//
// Tokenizes every verse_text row for `language = 'en'`, normalizes each token
// (lowercase, strip punctuation), counts occurrences, and writes the result.
// Idempotent: existing rows for the language are TRUNCATEd before insert.
//
//	just games-seed-frequency
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/wikiSubmission/ws-backend/db"
)

func main() {
	language := flag.String("language", "en", "Language code to seed (only 'en' supported in v1)")
	flag.Parse()

	if *language != "en" {
		fmt.Fprintf(os.Stderr, "Unsupported language %q (v1 only ships 'en').\n", *language)
		os.Exit(1)
	}

	ctx := context.Background()
	database, err := db.DbSetup()
	if err != nil {
		slog.Error("Database setup failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer database.Close()

	rows, err := database.Pool.Query(ctx, `
		SELECT vt.content
		FROM verse_text vt
		JOIN languages l ON l.id = vt.lang_id
		WHERE l.code = $1
	`, *language)
	if err != nil {
		slog.Error("Failed to read verse_text", slog.Any("error", err))
		os.Exit(1)
	}

	counts := make(map[string]int)
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			slog.Error("Failed to scan verse_text row", slog.Any("error", err))
			os.Exit(1)
		}
		for _, tok := range tokenize(content) {
			counts[tok]++
		}
	}
	if err := rows.Err(); err != nil {
		slog.Error("verse_text iteration error", slog.Any("error", err))
		os.Exit(1)
	}
	rows.Close()

	if len(counts) == 0 {
		slog.Error("No tokens collected — is verse_text populated for the language?", "language", *language)
		os.Exit(1)
	}

	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		slog.Error("Failed to begin tx", slog.Any("error", err))
		os.Exit(1)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx,
		`DELETE FROM game_word_frequency WHERE language = $1`, *language); err != nil {
		slog.Error("Failed to clear existing frequencies", slog.Any("error", err))
		os.Exit(1)
	}

	batch := &pgx.Batch{}
	for word, freq := range counts {
		batch.Queue(
			`INSERT INTO game_word_frequency (language, word_normalized, frequency)
			 VALUES ($1, $2, $3)`,
			*language, word, freq)
	}
	br := tx.SendBatch(ctx, batch)
	for i := 0; i < len(counts); i++ {
		if _, err := br.Exec(); err != nil {
			br.Close() //nolint:errcheck
			slog.Error("Batch insert failed", slog.Any("error", err))
			os.Exit(1)
		}
	}
	if err := br.Close(); err != nil {
		slog.Error("Batch close failed", slog.Any("error", err))
		os.Exit(1)
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("Commit failed", slog.Any("error", err))
		os.Exit(1)
	}

	fmt.Printf("Seeded %d distinct tokens for language %q.\n", len(counts), *language)
}

// tokenize splits text into normalized word tokens. Letters/digits are kept
// (lowercased); everything else is treated as a separator. Empty tokens are
// dropped.
func tokenize(s string) []string {
	out := []string{}
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		default:
			flush()
		}
	}
	flush()
	return out
}
