// games_seed_frequency populates game_word_frequency for the English
// (Rashad Khalifa) translation by delegating to db.RebuildWordFrequency.
// Idempotent. The same logic is exposed as an admin endpoint for prod use.
//
//	just games-seed-frequency
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

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

	count, err := database.RebuildWordFrequency(ctx, *language)
	if err != nil {
		slog.Error("Frequency rebuild failed", slog.Any("error", err))
		os.Exit(1)
	}
	fmt.Printf("Seeded %d distinct tokens for language %q.\n", count, *language)
}
