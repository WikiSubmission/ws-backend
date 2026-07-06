// Command seed-library loads the embedded library seed
// (db/seeds/library_docs_en.json — introduction, proclamation, appendices)
// into library_documents/library_sections. Full-replace per language, so
// re-running after regenerating the seed is the content-update path.
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

	n, err := database.SeedLibraryDocuments(ctx)
	if err != nil {
		slog.Error("Library seed failed", slog.Any("error", err))
		os.Exit(1)
	}
	fmt.Printf("Seeded %d library documents.\n", n)
}
