// games_review is the editorial tool for the Quran Games passage catalog.
// It lists proposed passages and flips their review status.
//
// List (default lists status='proposed'):
//
//	just games-review                       # all proposed
//	just games-review --status all          # every status
//	just games-review --chapter 1           # proposed in chapter 1
//	just games-review --id 42               # full detail for one passage
//
// Set status:
//
//	just games-review --id 42 --approve --by editor@example.com
//	just games-review --id 42 --reject
//	just games-review --id 42 --needs-refinement
//
// Only `approved` passages are served by GET /games/fill-blank/passages.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/wikiSubmission/ws-backend/db"
)

func main() {
	var (
		id           = flag.Int64("id", 0, "Passage id to show or modify")
		status       = flag.String("status", "proposed", "Status filter for list mode: proposed|approved|rejected|needs_refinement|all")
		chapter      = flag.Int("chapter", 0, "Chapter filter for list mode (1-114)")
		approve      = flag.Bool("approve", false, "Set the --id passage to approved")
		reject       = flag.Bool("reject", false, "Set the --id passage to rejected")
		needsRefine  = flag.Bool("needs-refinement", false, "Set the --id passage to needs_refinement")
		by           = flag.String("by", "", "Editor email recorded as approved_by (approve only)")
	)
	flag.Parse()

	ctx := context.Background()
	database, err := db.DbSetup()
	if err != nil {
		slog.Error("db setup failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer database.Close()

	// Determine the requested status change, if any.
	newStatus, changing, err := resolveStatusChange(*approve, *reject, *needsRefine)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		flag.Usage()
		os.Exit(2)
	}

	if changing {
		if *id <= 0 {
			fmt.Fprintln(os.Stderr, "--id is required when changing status")
			os.Exit(2)
		}
		if err := changeStatus(ctx, database, *id, newStatus, *by); err != nil {
			slog.Error("status change failed", slog.Any("error", err))
			os.Exit(1)
		}
		return
	}

	// Detail view for a single passage.
	if *id > 0 {
		p, err := database.FetchPassageByID(ctx, *id)
		if err != nil {
			slog.Error("fetch passage failed", slog.Any("error", err))
			os.Exit(1)
		}
		if p == nil {
			fmt.Fprintf(os.Stderr, "No passage with id %d.\n", *id)
			os.Exit(1)
		}
		printDetail(p)
		return
	}

	// List mode.
	var statusFilter *string
	if *status != "all" {
		s := *status
		statusFilter = &s
	}
	var chapterFilter *int
	if *chapter > 0 {
		c := *chapter
		chapterFilter = &c
	}
	rows, err := database.FetchPassagesForReview(ctx, statusFilter, chapterFilter)
	if err != nil {
		slog.Error("list passages failed", slog.Any("error", err))
		os.Exit(1)
	}
	printList(rows)
}

func resolveStatusChange(approve, reject, needsRefine bool) (status string, changing bool, err error) {
	n := 0
	if approve {
		n++
		status = "approved"
	}
	if reject {
		n++
		status = "rejected"
	}
	if needsRefine {
		n++
		status = "needs_refinement"
	}
	if n == 0 {
		return "", false, nil
	}
	if n > 1 {
		return "", false, fmt.Errorf("choose only one of --approve / --reject / --needs-refinement")
	}
	return status, true, nil
}

func changeStatus(ctx context.Context, database *db.DB, id int64, status, by string) error {
	var editor *string
	if status == "approved" && by != "" {
		editor = &by
	}
	ok, err := database.SetPassageStatus(ctx, id, status, editor)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no passage with id %d", id)
	}
	if status == "approved" && by != "" {
		fmt.Printf("Passage %d set to %s (by %s).\n", id, status, by)
	} else {
		fmt.Printf("Passage %d set to %s.\n", id, status)
	}
	return nil
}

func printList(rows []db.GamePassageRow) {
	if len(rows) == 0 {
		fmt.Println("No passages match.")
		return
	}
	fmt.Printf("%-6s %-12s %-9s %-10s %s\n", "ID", "RANGE", "STATUS", "LLM_DIFF", "LABEL")
	fmt.Println(strings.Repeat("-", 80))
	for _, p := range rows {
		fmt.Printf("%-6d %-12s %-9s %-10s %s\n",
			p.ID, verseRange(&p), p.Status, deref(p.LlmDifficulty), truncate(p.Label, 40))
	}
	fmt.Printf("\n%d passage(s).\n", len(rows))
}

func printDetail(p *db.GamePassageRow) {
	fmt.Printf("ID:            %d\n", p.ID)
	fmt.Printf("Range:         %s\n", verseRange(p))
	fmt.Printf("Label:         %s\n", p.Label)
	fmt.Printf("Themes:        %s\n", strings.Join(p.Themes, ", "))
	fmt.Printf("Status:        %s\n", p.Status)
	fmt.Printf("Source:        %s\n", p.Source)
	fmt.Printf("LLM difficulty:%s\n", spacedDeref(p.LlmDifficulty))
	if p.LlmBlankHint != nil {
		fmt.Printf("LLM blanks:    %d\n", *p.LlmBlankHint)
	}
	fmt.Printf("Rationale:     %s\n", p.Rationale)
	if p.ApprovedAt != nil {
		fmt.Printf("Approved at:   %s\n", p.ApprovedAt.UTC().Format("2006-01-02 15:04:05 MST"))
	}
	if p.ApprovedBy != nil {
		fmt.Printf("Approved by:   user #%d\n", *p.ApprovedBy)
	}
}

func verseRange(p *db.GamePassageRow) string {
	if p.ChapterStart == p.ChapterEnd {
		return fmt.Sprintf("%d:%d-%d", p.ChapterStart, p.VerseStart, p.VerseEnd)
	}
	return fmt.Sprintf("%d:%d-%d:%d", p.ChapterStart, p.VerseStart, p.ChapterEnd, p.VerseEnd)
}

func deref(s *string) string {
	if s == nil {
		return "-"
	}
	return *s
}

func spacedDeref(s *string) string {
	if s == nil {
		return " -"
	}
	return " " + *s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
