// seed_embeddings generates and stores OpenAI embeddings for all English
// verse_text rows whose embedding column is NULL.
//
// Usage:
//
//	go run ./api/cmd/seed_embeddings/
//
// Required env vars: OPENAI_API_KEY (and the usual DATABASE_* / DATABASE_URL vars).
// The script is idempotent: rows that already have an embedding are skipped.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	pgvector "github.com/pgvector/pgvector-go"
	apitembed "github.com/wikiSubmission/ws-backend/api/embed"
	"github.com/wikiSubmission/ws-backend/db"
)

const (
	workerCount = 5   // concurrent OpenAI calls (stay within rate limits)
	batchSize   = 200 // rows per pgx.Batch UPDATE
	logEvery    = 500 // print progress every N rows
)

type row struct {
	id   int
	text string
}

func main() {
	_ = godotenv.Load()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		slog.Error("OPENAI_API_KEY is not set")
		os.Exit(1)
	}

	database, err := db.DbSetup()
	if err != nil {
		slog.Error("database setup failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	ctx := context.Background()
	embedder := apitembed.NewOpenAIEmbedder(apiKey)

	// Fetch all English verse_text rows that still need an embedding.
	rows, err := fetchPending(ctx, database)
	if err != nil {
		slog.Error("failed to fetch pending rows", "error", err)
		os.Exit(1)
	}
	total := len(rows)
	if total == 0 {
		fmt.Println("All rows already embedded — nothing to do.")
		return
	}
	fmt.Printf("Embedding %d English verse_text rows…\n", total)

	// Fan-out: produce rows into a channel, workers embed and send results back.
	type result struct {
		id        int
		embedding []float32
		err       error
	}

	jobs := make(chan row, workerCount*2)
	results := make(chan result, workerCount*2)

	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range jobs {
				vec, embedErr := embedWithRetry(ctx, embedder, r.text, 3)
				results <- result{id: r.id, embedding: vec, err: embedErr}
			}
		}()
	}

	// Close results when all workers finish.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Feed jobs.
	go func() {
		for _, r := range rows {
			jobs <- r
		}
		close(jobs)
	}()

	// Collect results and flush in batches.
	pending := make([]result, 0, batchSize)
	done := 0
	failed := 0

	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		batch := &pgx.Batch{}
		for _, res := range pending {
			batch.Queue(
				"UPDATE verse_text SET embedding = $1 WHERE id = $2",
				pgvector.NewVector(res.embedding), res.id,
			)
		}
		br := database.Pool.SendBatch(ctx, batch)
		defer br.Close()
		for range pending {
			if _, execErr := br.Exec(); execErr != nil {
				return fmt.Errorf("batch update failed: %w", execErr)
			}
		}
		pending = pending[:0]
		return nil
	}

	for res := range results {
		if res.err != nil {
			slog.Warn("embedding failed for row", "id", res.id, "error", res.err)
			failed++
			continue
		}
		pending = append(pending, res)
		done++
		if len(pending) >= batchSize {
			if err := flush(); err != nil {
				slog.Error("flush failed", "error", err)
				os.Exit(1)
			}
		}
		if done%logEvery == 0 {
			fmt.Printf("  %d/%d embedded…\n", done, total)
		}
	}
	if err := flush(); err != nil {
		slog.Error("final flush failed", "error", err)
		os.Exit(1)
	}

	fmt.Printf("Done. Embedded %d rows", done)
	if failed > 0 {
		fmt.Printf(", %d failed (re-run to retry)", failed)
	}
	fmt.Println(".")
}

// fetchPending returns all English verse_text rows without an embedding.
func fetchPending(ctx context.Context, database *db.DB) ([]row, error) {
	sql := `
		SELECT vt.id,
		       vt.content
		       || CASE WHEN vt.subtitle IS NOT NULL AND vt.subtitle <> '' THEN ' ' || vt.subtitle ELSE '' END
		       || CASE WHEN vt.footer   IS NOT NULL AND vt.footer   <> '' THEN ' ' || vt.footer   ELSE '' END
		           AS text
		FROM   verse_text vt
		JOIN   languages  l ON l.id = vt.lang_id
		WHERE  l.code = 'en'
		  AND  vt.embedding IS NULL
		ORDER  BY vt.id
	`
	pgRows, err := database.Pool.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer pgRows.Close()

	var out []row
	for pgRows.Next() {
		var r row
		if err := pgRows.Scan(&r.id, &r.text); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, pgRows.Err()
}

// embedWithRetry calls Embed with exponential backoff on failure.
func embedWithRetry(ctx context.Context, embedder *apitembed.OpenAIEmbedder, text string, maxRetries int) ([]float32, error) {
	var lastErr error
	for attempt := range maxRetries {
		vec, err := embedder.Embed(ctx, text)
		if err == nil {
			return vec, nil
		}
		lastErr = err
		wait := time.Duration(1<<uint(attempt)) * time.Second
		slog.Warn("embed attempt failed, retrying", "attempt", attempt+1, "wait", wait, "error", err)
		time.Sleep(wait)
	}
	return nil, lastErr
}
