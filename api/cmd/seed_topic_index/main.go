// Command seed-topic-index loads the Quran's printed topical index
// (db/seeds/quran_topic_index_<lang>.json — the alphabetical subject index bound
// into the back of Quran, The Final Testament, generated from masjidtucson.org's
// HTML by scripts/topic_index/build.mjs) into topic_index_entries and its child
// tables.
//
// Full-replace per language, so re-running after regenerating the JSON is the
// content-update path.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/wikiSubmission/ws-backend/db"
)

func main() {
	ctx := context.Background()
	database, err := db.DbSetup()
	if err != nil {
		slog.Error("Database setup failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer database.Close()

	seeded, err := database.SeedTopicIndex(ctx)
	if err != nil {
		slog.Error("Topic index seed failed", slog.Any("error", err))
		os.Exit(1)
	}
	fmt.Printf("Seeded %d topical index entries.\n", seeded)
}
