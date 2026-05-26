// groq_curate proposes Quran Games passages for a chapter using the Groq
// chat completions API and writes them to game_passages with status='proposed'.
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
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/wikiSubmission/ws-backend/db"
)

//go:embed prompt.md
var systemPrompt string

const (
	defaultModel   = "llama-3.3-70b-versatile"
	defaultBaseURL = "https://api.groq.com/openai/v1"
	apiTimeout     = 60 * time.Second
)

type proposal struct {
	VerseStart    int      `json:"verse_start"`
	VerseEnd      int      `json:"verse_end"`
	Label         string   `json:"label"`
	Themes        []string `json:"themes"`
	Rationale     string   `json:"rationale"`
	LlmDifficulty string   `json:"llm_difficulty"`
	LlmBlankHint  int      `json:"llm_blank_hint"`
}

type proposalEnvelope struct {
	Proposals []proposal `json:"proposals"`
}

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
	model := envOr("GROQ_MODEL", defaultModel)
	baseURL := envOr("GROQ_BASE_URL", defaultBaseURL)

	ctx := context.Background()
	database, err := db.DbSetup()
	if err != nil {
		slog.Error("db setup failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer database.Close()

	chapterText, err := fetchChapterText(ctx, database, *chapter)
	if err != nil {
		slog.Error("fetch chapter text", slog.Any("error", err))
		os.Exit(1)
	}
	if chapterText == "" {
		slog.Error("chapter has no English verses", "chapter", *chapter)
		os.Exit(1)
	}

	// In refine mode, only proceed if there are passages to retry.
	if *refine {
		count, err := countRefinable(ctx, database, *chapter)
		if err != nil {
			slog.Error("count refinable", slog.Any("error", err))
			os.Exit(1)
		}
		if count == 0 {
			fmt.Printf("Chapter %d has no rejected or needs_refinement passages.\n", *chapter)
			return
		}
	}

	proposals, err := proposePassages(ctx, baseURL, apiKey, model, *chapter, chapterText)
	if err != nil {
		slog.Error("propose passages", slog.Any("error", err))
		os.Exit(1)
	}
	if len(proposals) == 0 {
		slog.Error("LLM returned zero proposals")
		os.Exit(1)
	}

	valid, dropped := validateProposals(proposals)
	for _, d := range dropped {
		slog.Warn("dropped proposal", "reason", d.reason, "verse_start", d.p.VerseStart, "verse_end", d.p.VerseEnd, "label", d.p.Label)
	}
	if len(valid) == 0 {
		slog.Error("all proposals dropped during validation")
		os.Exit(1)
	}

	if err := insertProposals(ctx, database, *chapter, valid, *refine); err != nil {
		slog.Error("insert proposals", slog.Any("error", err))
		os.Exit(1)
	}

	verb := "Proposed"
	if *refine {
		verb = "Refined"
	}
	fmt.Printf("%s %d passage(s) for chapter %d.\n", verb, len(valid), *chapter)
}

// ── DB helpers ──────────────────────────────────────────────────────────────

func fetchChapterText(ctx context.Context, database *db.DB, chapter int) (string, error) {
	contents, err := database.FetchChapterVerseContents(ctx, "en", chapter)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for i, c := range contents {
		fmt.Fprintf(&b, "[%d:%d] %s\n", chapter, i+1, c)
	}
	return b.String(), nil
}

func countRefinable(ctx context.Context, database *db.DB, chapter int) (int, error) {
	var n int
	err := database.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM game_passages
		WHERE chapter_start = $1 AND status IN ('rejected', 'needs_refinement')
	`, chapter).Scan(&n)
	return n, err
}

func insertProposals(ctx context.Context, database *db.DB, chapter int, props []proposal, refine bool) error {
	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if refine {
		if _, err := tx.Exec(ctx, `
			DELETE FROM game_passages
			WHERE chapter_start = $1 AND status IN ('rejected', 'needs_refinement')
		`, chapter); err != nil {
			return fmt.Errorf("clear refinable: %w", err)
		}
	}

	batch := &pgx.Batch{}
	for _, p := range props {
		batch.Queue(`
			INSERT INTO game_passages
			  (chapter_start, verse_start, chapter_end, verse_end,
			   label, themes, rationale, llm_difficulty, llm_blank_hint,
			   status, source)
			VALUES ($1, $2, $1, $3, $4, $5, $6, $7, $8, 'proposed', 'groq')
		`, chapter, p.VerseStart, p.VerseEnd, p.Label, p.Themes, p.Rationale, normalizeDifficulty(p.LlmDifficulty), p.LlmBlankHint)
	}
	br := tx.SendBatch(ctx, batch)
	for range props {
		if _, err := br.Exec(); err != nil {
			br.Close() //nolint:errcheck
			return fmt.Errorf("insert proposal: %w", err)
		}
	}
	if err := br.Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func normalizeDifficulty(s string) *string {
	v := strings.ToLower(strings.TrimSpace(s))
	switch v {
	case "easy", "moderate", "challenging":
		return &v
	}
	return nil
}

// ── Groq HTTP ───────────────────────────────────────────────────────────────

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	Temperature    float64        `json:"temperature"`
	ResponseFormat map[string]any `json:"response_format,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func proposePassages(ctx context.Context, baseURL, apiKey, model string, chapter int, chapterText string) ([]proposal, error) {
	user := fmt.Sprintf("Chapter %d (Rashad Khalifa translation, English):\n\n%s\n\nReturn JSON as specified.", chapter, chapterText)

	reqBody := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: user},
		},
		Temperature:    0.3,
		ResponseFormat: map[string]any{"type": "json_object"},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpCtx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(httpCtx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call groq: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("groq %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("groq error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("groq returned no choices")
	}
	content := parsed.Choices[0].Message.Content

	var env proposalEnvelope
	if err := json.Unmarshal([]byte(content), &env); err != nil {
		// Some models wrap the array directly. Try that too.
		var direct []proposal
		if err2 := json.Unmarshal([]byte(content), &direct); err2 == nil {
			return direct, nil
		}
		return nil, fmt.Errorf("decode model content: %w (raw: %s)", err, content)
	}
	return env.Proposals, nil
}

// ── Validation ──────────────────────────────────────────────────────────────

type droppedProposal struct {
	p      proposal
	reason string
}

// validateProposals enforces basic invariants and resolves overlaps in favor
// of the first proposal that claims a verse. Returns the kept proposals and
// the dropped ones with a reason.
func validateProposals(props []proposal) (kept []proposal, dropped []droppedProposal) {
	used := make(map[int]bool)
	for _, p := range props {
		switch {
		case p.VerseStart < 1 || p.VerseEnd < p.VerseStart:
			dropped = append(dropped, droppedProposal{p, "invalid verse range"})
			continue
		case p.VerseEnd-p.VerseStart+1 < 3:
			dropped = append(dropped, droppedProposal{p, "passage shorter than 3 verses"})
			continue
		case p.VerseEnd-p.VerseStart+1 > 40:
			dropped = append(dropped, droppedProposal{p, "passage longer than 40 verses"})
			continue
		case strings.TrimSpace(p.Label) == "":
			dropped = append(dropped, droppedProposal{p, "empty label"})
			continue
		}
		// Reject any overlap with an already-accepted proposal.
		conflict := false
		for v := p.VerseStart; v <= p.VerseEnd; v++ {
			if used[v] {
				conflict = true
				break
			}
		}
		if conflict {
			dropped = append(dropped, droppedProposal{p, "overlapping verse range"})
			continue
		}
		for v := p.VerseStart; v <= p.VerseEnd; v++ {
			used[v] = true
		}
		if p.Themes == nil {
			p.Themes = []string{}
		}
		kept = append(kept, p)
	}
	return kept, dropped
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
