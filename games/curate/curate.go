// Package curate proposes Quran Games passages for a chapter using the Groq
// chat completions API and writes them to game_passages with status='proposed'.
//
// It is the importable core shared by the groq_curate CLI and the editorial
// admin endpoint (POST /api/v1/admin/games/curate). Network and DB work live
// here; callers supply configuration and a *db.DB.
package curate

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/wikiSubmission/ws-backend/db"
)

//go:embed prompt.md
var systemPrompt string

const (
	// DefaultModel and DefaultBaseURL match the Groq OpenAI-compatible API.
	DefaultModel   = "llama-3.3-70b-versatile"
	DefaultBaseURL = "https://api.groq.com/openai/v1"
	apiTimeout     = 60 * time.Second
)

// Proposal is one passage the model proposed for a chapter.
type Proposal struct {
	VerseStart    int      `json:"verse_start"`
	VerseEnd      int      `json:"verse_end"`
	Label         string   `json:"label"`
	Themes        []string `json:"themes"`
	Rationale     string   `json:"rationale"`
	LlmDifficulty string   `json:"llm_difficulty"`
	LlmBlankHint  int      `json:"llm_blank_hint"`
}

type proposalEnvelope struct {
	Proposals []Proposal `json:"proposals"`
}

// Dropped is a proposal rejected during validation, with the reason.
type Dropped struct {
	Proposal Proposal
	Reason   string
}

// Config holds the Groq credentials and overrides. Model and BaseURL fall back
// to the package defaults when empty.
type Config struct {
	APIKey  string
	Model   string
	BaseURL string
}

func (c Config) model() string {
	if c.Model != "" {
		return c.Model
	}
	return DefaultModel
}

func (c Config) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultBaseURL
}

// Result summarizes a curation run.
type Result struct {
	Chapter  int
	Proposed int       // passages inserted
	Dropped  []Dropped // proposals rejected during validation
	Refine   bool
	// Skipped is true in refine mode when the chapter has nothing to refine,
	// so no model call was made.
	Skipped bool
}

// ErrNoChapterText means the chapter has no English verses to curate.
var ErrNoChapterText = fmt.Errorf("chapter has no English verses")

// ErrNoProposals means the model returned (or validation kept) zero proposals.
var ErrNoProposals = fmt.Errorf("no usable proposals")

// Run curates one chapter end to end: fetch text, call the model, validate, and
// insert kept proposals as status='proposed'. When refine is true it only
// proceeds if the chapter has rejected/needs_refinement passages, replacing
// them. The Groq call surfaces as an error so callers can map it to 502.
func Run(ctx context.Context, database *db.DB, cfg Config, chapter int, refine bool) (*Result, error) {
	chapterText, err := FetchChapterText(ctx, database, chapter)
	if err != nil {
		return nil, fmt.Errorf("fetch chapter text: %w", err)
	}
	if chapterText == "" {
		return nil, ErrNoChapterText
	}

	if refine {
		n, err := countRefinable(ctx, database, chapter)
		if err != nil {
			return nil, fmt.Errorf("count refinable: %w", err)
		}
		if n == 0 {
			return &Result{Chapter: chapter, Refine: true, Skipped: true}, nil
		}
	}

	proposals, err := ProposePassages(ctx, cfg, chapter, chapterText)
	if err != nil {
		return nil, err
	}
	if len(proposals) == 0 {
		return nil, ErrNoProposals
	}

	valid, dropped := ValidateProposals(proposals)
	if len(valid) == 0 {
		return &Result{Chapter: chapter, Dropped: dropped, Refine: refine}, ErrNoProposals
	}

	if err := InsertProposals(ctx, database, chapter, valid, refine); err != nil {
		return nil, fmt.Errorf("insert proposals: %w", err)
	}

	return &Result{Chapter: chapter, Proposed: len(valid), Dropped: dropped, Refine: refine}, nil
}

// ── DB helpers ──────────────────────────────────────────────────────────────

// FetchChapterText renders a chapter's English verses as "[c:v] text" lines.
func FetchChapterText(ctx context.Context, database *db.DB, chapter int) (string, error) {
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

// InsertProposals writes kept proposals as status='proposed'. In refine mode it
// first clears the chapter's rejected/needs_refinement rows in the same tx.
func InsertProposals(ctx context.Context, database *db.DB, chapter int, props []Proposal, refine bool) error {
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
		`, chapter, p.VerseStart, p.VerseEnd, p.Label, p.Themes, p.Rationale, NormalizeDifficulty(p.LlmDifficulty), p.LlmBlankHint)
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

// NormalizeDifficulty lowercases and validates the advisory difficulty, or
// returns nil for an unrecognized value.
func NormalizeDifficulty(s string) *string {
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

// ProposePassages asks the model to propose passages for one chapter.
func ProposePassages(ctx context.Context, cfg Config, chapter int, chapterText string) ([]Proposal, error) {
	user := fmt.Sprintf("Chapter %d (Rashad Khalifa translation, English):\n\n%s\n\nReturn JSON as specified.", chapter, chapterText)

	reqBody := chatRequest{
		Model: cfg.model(),
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

	req, err := http.NewRequestWithContext(httpCtx, http.MethodPost, cfg.baseURL()+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

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
		var direct []Proposal
		if err2 := json.Unmarshal([]byte(content), &direct); err2 == nil {
			return direct, nil
		}
		return nil, fmt.Errorf("decode model content: %w (raw: %s)", err, content)
	}
	return env.Proposals, nil
}

// ── Validation ──────────────────────────────────────────────────────────────

// ValidateProposals enforces basic invariants and resolves overlaps in favor of
// the first proposal that claims a verse. Returns the kept proposals and the
// dropped ones with a reason.
func ValidateProposals(props []Proposal) (kept []Proposal, dropped []Dropped) {
	used := make(map[int]bool)
	for _, p := range props {
		switch {
		case p.VerseStart < 1 || p.VerseEnd < p.VerseStart:
			dropped = append(dropped, Dropped{p, "invalid verse range"})
			continue
		case p.VerseEnd-p.VerseStart+1 < 3:
			dropped = append(dropped, Dropped{p, "passage shorter than 3 verses"})
			continue
		case p.VerseEnd-p.VerseStart+1 > 40:
			dropped = append(dropped, Dropped{p, "passage longer than 40 verses"})
			continue
		case strings.TrimSpace(p.Label) == "":
			dropped = append(dropped, Dropped{p, "empty label"})
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
			dropped = append(dropped, Dropped{p, "overlapping verse range"})
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
