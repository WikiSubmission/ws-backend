// groq_curate proposes Quran Games passages for a chapter using the Groq chat
// completions API and writes them to game_passages with status='proposed'.
//
// The curation logic lives in the importable games/curate package, shared with
// the editorial admin endpoint; this command is a thin CLI wrapper.
//
// Usage:
//
//	just games-curate-chapter 1               # propose new passages for chapter 1
//	just games-refine-rejected 1              # re-propose for chapter 1 passages currently
//	                                          # rejected or needs_refinement
//
// Env vars:
//
//	GROQ_API_KEY   required
//	GROQ_MODEL     optional, default "llama-3.3-70b-versatile"
//	GROQ_BASE_URL  optional, default "https://api.groq.com/openai/v1"
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/wikiSubmission/ws-backend/db"
	"github.com/wikiSubmission/ws-backend/games/curate"
)

func main() {
	chapter := flag.Int("chapter", 0, "Chapter number to curate (1-114)")
	refine := flag.Bool("refine-rejected", false, "Only re-propose for rejected / needs_refinement passages in this chapter")
	flag.Parse()

	if *chapter < 1 || *chapter > 114 {
		fmt.Fprintln(os.Stderr, "Usage: groq_curate --chapter N [--refine-rejected]")
		os.Exit(2)
	}

	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "GROQ_API_KEY is required")
		os.Exit(2)
	}
	cfg := curate.Config{
		APIKey:  apiKey,
		Model:   os.Getenv("GROQ_MODEL"),
		BaseURL: os.Getenv("GROQ_BASE_URL"),
	}

	ctx := context.Background()
	database, err := db.DbSetup()
	if err != nil {
		slog.Error("db setup failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer database.Close()

	res, err := curate.Run(ctx, database, cfg, *chapter, *refine)
	if err != nil {
		slog.Error("curate", slog.Any("error", err))
		os.Exit(1)
	}

	for _, d := range res.Dropped {
		slog.Warn("dropped proposal", "reason", d.Reason, "verse_start", d.Proposal.VerseStart, "verse_end", d.Proposal.VerseEnd, "label", d.Proposal.Label)
	}

	if res.Skipped {
		fmt.Printf("Chapter %d has no rejected or needs_refinement passages.\n", *chapter)
		return
	}

	verb := "Proposed"
	if res.Refine {
		verb = "Refined"
	}
	fmt.Printf("%s %d passage(s) for chapter %d.\n", verb, res.Proposed, *chapter)
	if res.Partial {
		fmt.Printf("Stopped early on a rate limit; run again to curate the remaining verses.\n")
	}
}
