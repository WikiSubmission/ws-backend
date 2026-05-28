// Package curate proposes Quran Games passages for a chapter using the Groq
// chat completions API and writes them to game_passages with status='proposed'.
//
// It is the importable core shared by the groq_curate CLI and the editorial
// admin endpoint (POST /api/v1/admin/games/curate). Network and DB work live
// here; callers supply configuration and a *db.DB.
//
// Long chapters exceed Groq's per-minute token limit in a single request, so a
// run is split into verse windows sized under a token budget and paced to
// respect the limit. Each window is inserted as it completes, and verses
// already covered by existing proposed/approved passages are skipped, so a run
// interrupted by a rate limit resumes cleanly on the next invocation.
package curate

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
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
	apiTimeout     = 180 * time.Second

	// maxCompletionTokens caps the model's output, which counts toward the
	// per-request token total Groq enforces.
	maxCompletionTokens = 3000
	// tpmBudget is the tokens-per-minute ceiling we pace against. Set below
	// Groq's 12k free-tier TPM to leave headroom for estimation error.
	tpmBudget = 11000
	// maxVerseTokensPerWindow bounds the verse text in one request so prompt +
	// completion stays comfortably under tpmBudget.
	maxVerseTokensPerWindow = 6000
	// minWindowVerses is the smallest passage the model can form, so a window
	// shorter than this cannot yield a proposal.
	minWindowVerses = 3
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
	// Skipped is true in refine mode when the chapter has nothing to refine, so
	// no model call was made.
	Skipped bool
	// Partial is true when a rate limit stopped the run before all windows were
	// curated. Re-running resumes from the uncovered verses.
	Partial bool
}

// ErrNoChapterText means the chapter has no English verses to curate.
var ErrNoChapterText = fmt.Errorf("chapter has no English verses")

// ErrNoProposals means the model returned (or validation kept) zero proposals.
var ErrNoProposals = fmt.Errorf("no usable proposals")

// ErrRateLimited wraps a Groq token/rate-limit rejection so callers can tell it
// apart from other failures and back off before retrying. Callers that need
// the server-supplied wait hint should use errors.As with *RateLimitError.
var ErrRateLimited = errors.New("groq rate limit")

// RateLimitError carries the upstream wait hint (Retry-After header or a
// duration parsed from the JSON error body) alongside the rate-limit signal.
// RetryAfter is zero when no hint was provided; callers should fall back to
// their own backoff in that case.
type RateLimitError struct {
	RetryAfter time.Duration
	Status     int
	Body       string
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("groq %d (retry after %s): %s", e.Status, e.RetryAfter, e.Body)
	}
	return fmt.Sprintf("groq %d: %s", e.Status, e.Body)
}

func (e *RateLimitError) Is(target error) bool {
	return target == ErrRateLimited
}

// WindowResult reports the outcome of curating a single verse window, with a
// cursor (NextVerse) and Done flag so a caller can drive the chapter to
// completion one window at a time and show progress.
type WindowResult struct {
	Chapter     int
	WindowStart int
	WindowEnd   int
	Proposed    int
	Dropped     []Dropped
	NextVerse   int // pass as afterVerse on the next call
	Done        bool
	VersesTotal int
}

// RunWindow curates the first uncovered verse window that starts after
// afterVerse (pass 0 to begin), inserts its kept proposals, and returns a
// cursor for the next call. It performs at most one model request and does not
// pace internally — the caller paces between calls and backs off on
// ErrRateLimited. Verses already covered by proposed/approved passages are
// skipped, and the cursor advances even when a window yields nothing, so
// repeated calls always make progress and never loop.
func RunWindow(ctx context.Context, database *db.DB, cfg Config, chapter, afterVerse int) (*WindowResult, error) {
	contents, err := database.FetchChapterVerseContents(ctx, "en", chapter)
	if err != nil {
		return nil, fmt.Errorf("fetch chapter text: %w", err)
	}
	versesTotal := len(contents)
	if versesTotal == 0 {
		return nil, ErrNoChapterText
	}

	covered, err := fetchCoveredVerses(ctx, database, chapter)
	if err != nil {
		return nil, fmt.Errorf("fetch covered verses: %w", err)
	}
	windows := buildWindows(contents, covered)

	idx := -1
	for i, w := range windows {
		if w.start > afterVerse {
			idx = i
			break
		}
	}
	if idx == -1 {
		slog.Info("curate window: nothing left", "chapter", chapter, "after_verse", afterVerse, "verses_total", versesTotal)
		return &WindowResult{Chapter: chapter, NextVerse: afterVerse, Done: true, VersesTotal: versesTotal}, nil
	}

	w := windows[idx]
	text := renderWindow(chapter, w, contents)
	slog.Info("curate window: requesting", "chapter", chapter, "window", fmt.Sprintf("%d-%d", w.start, w.end), "est_tokens", requestTokenEstimate(text))

	props, err := ProposePassages(ctx, cfg, chapter, w.start, w.end, text)
	if err != nil {
		if errors.Is(err, ErrRateLimited) {
			slog.Warn("curate window: rate limited", "chapter", chapter, "window", fmt.Sprintf("%d-%d", w.start, w.end))
		} else {
			slog.Error("curate window: model failed", "chapter", chapter, "window", fmt.Sprintf("%d-%d", w.start, w.end), "error", err)
		}
		return nil, err
	}

	kept, dropped := ValidateProposals(props)
	inRange := make([]Proposal, 0, len(kept))
	for _, p := range kept {
		if p.VerseStart >= w.start && p.VerseEnd <= w.end {
			inRange = append(inRange, p)
		} else {
			dropped = append(dropped, Dropped{p, fmt.Sprintf("outside window %d-%d", w.start, w.end)})
		}
	}
	if len(inRange) > 0 {
		if err := InsertProposals(ctx, database, chapter, inRange, false); err != nil {
			return nil, fmt.Errorf("insert proposals: %w", err)
		}
	}

	done := idx == len(windows)-1
	slog.Info("curate window: done",
		"chapter", chapter,
		"window", fmt.Sprintf("%d-%d", w.start, w.end),
		"proposed", len(inRange),
		"dropped", len(dropped),
		"done", done,
	)
	return &WindowResult{
		Chapter:     chapter,
		WindowStart: w.start,
		WindowEnd:   w.end,
		Proposed:    len(inRange),
		Dropped:     dropped,
		NextVerse:   w.end,
		Done:        done,
		VersesTotal: versesTotal,
	}, nil
}

// Run curates one chapter end to end: fetch text, split into paced windows,
// call the model per window, validate, and insert kept proposals as
// status='proposed'. When refine is true it first clears the chapter's
// rejected/needs_refinement passages (only if any exist) and re-curates the
// freed verses. Verses already covered by proposed/approved passages are
// skipped, so the operation is resumable. A Groq failure surfaces as an error
// when nothing was inserted yet, or as Partial=true once progress was made.
func Run(ctx context.Context, database *db.DB, cfg Config, chapter int, refine bool) (*Result, error) {
	contents, err := database.FetchChapterVerseContents(ctx, "en", chapter)
	if err != nil {
		return nil, fmt.Errorf("fetch chapter text: %w", err)
	}
	if len(contents) == 0 {
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
		if err := clearRefinable(ctx, database, chapter); err != nil {
			return nil, fmt.Errorf("clear refinable: %w", err)
		}
	}

	covered, err := fetchCoveredVerses(ctx, database, chapter)
	if err != nil {
		return nil, fmt.Errorf("fetch covered verses: %w", err)
	}

	windows := buildWindows(contents, covered)
	if len(windows) == 0 {
		// Nothing left to curate (whole chapter already covered).
		return &Result{Chapter: chapter, Refine: refine}, nil
	}

	pacer := newPacer(tpmBudget)
	var totalProposed int
	var allDropped []Dropped

	for _, w := range windows {
		text := renderWindow(chapter, w, contents)
		if err := pacer.wait(ctx, requestTokenEstimate(text)); err != nil {
			// Context cancelled while pacing; keep what we inserted.
			return &Result{Chapter: chapter, Proposed: totalProposed, Dropped: allDropped, Refine: refine, Partial: totalProposed > 0}, nil
		}

		props, err := ProposePassages(ctx, cfg, chapter, w.start, w.end, text)
		if err != nil {
			if totalProposed > 0 {
				// A rate limit or transient failure stopped us mid-chapter.
				// Earlier windows are already committed; re-running resumes.
				return &Result{Chapter: chapter, Proposed: totalProposed, Dropped: allDropped, Refine: refine, Partial: true}, nil
			}
			return nil, err
		}

		kept, dropped := ValidateProposals(props)
		allDropped = append(allDropped, dropped...)

		inRange := make([]Proposal, 0, len(kept))
		for _, p := range kept {
			if p.VerseStart >= w.start && p.VerseEnd <= w.end {
				inRange = append(inRange, p)
			} else {
				allDropped = append(allDropped, Dropped{p, fmt.Sprintf("outside window %d-%d", w.start, w.end)})
			}
		}
		if len(inRange) > 0 {
			if err := InsertProposals(ctx, database, chapter, inRange, false); err != nil {
				return nil, fmt.Errorf("insert proposals: %w", err)
			}
			totalProposed += len(inRange)
		}
	}

	if totalProposed == 0 {
		return &Result{Chapter: chapter, Dropped: allDropped, Refine: refine}, ErrNoProposals
	}
	return &Result{Chapter: chapter, Proposed: totalProposed, Dropped: allDropped, Refine: refine}, nil
}

// ── Windowing ─────────────────────────────────────────────────────────────

type window struct {
	start int
	end   int
}

// estimateTokens is a rough char-based token estimate, good enough for sizing
// windows below the API's token limit (English averages ~4 chars/token).
func estimateTokens(s string) int {
	return len(s)/4 + 1
}

// requestTokenEstimate approximates the tokens one window request consumes:
// system prompt + verse text + the reserved completion.
func requestTokenEstimate(verseText string) int {
	return estimateTokens(systemPrompt) + estimateTokens(verseText) + maxCompletionTokens + 32
}

// buildWindows splits a chapter into contiguous runs of uncovered verses, each
// bounded by maxVerseTokensPerWindow. A long uncovered run is split across
// several windows; runs shorter than minWindowVerses are skipped.
func buildWindows(contents []string, covered map[int]bool) []window {
	n := len(contents)
	var windows []window
	v := 1
	for v <= n {
		for v <= n && covered[v] {
			v++
		}
		if v > n {
			break
		}
		start := v
		tokens := 0
		for v <= n && !covered[v] {
			// +5 approximates the "[c:v] " prefix and newline per line.
			lineTokens := estimateTokens(contents[v-1]) + 5
			if v > start && tokens+lineTokens > maxVerseTokensPerWindow {
				break
			}
			tokens += lineTokens
			v++
		}
		end := v - 1
		if end-start+1 >= minWindowVerses {
			windows = append(windows, window{start, end})
		}
	}
	return windows
}

func renderWindow(chapter int, w window, contents []string) string {
	var b strings.Builder
	for v := w.start; v <= w.end; v++ {
		fmt.Fprintf(&b, "[%d:%d] %s\n", chapter, v, contents[v-1])
	}
	return b.String()
}

// ── Pacing ──────────────────────────────────────────────────────────────────

// pacer throttles requests to stay under a tokens-per-minute budget. It tracks
// tokens spent in the current rolling minute and sleeps when the next request
// would exceed the budget.
type pacer struct {
	budget      int
	spent       int
	windowStart time.Time
}

func newPacer(budget int) *pacer {
	return &pacer{budget: budget, windowStart: time.Now()}
}

func (p *pacer) wait(ctx context.Context, cost int) error {
	now := time.Now()
	if now.Sub(p.windowStart) >= time.Minute {
		p.spent = 0
		p.windowStart = now
	}
	if p.spent > 0 && p.spent+cost > p.budget {
		sleep := time.Minute - now.Sub(p.windowStart)
		if sleep > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sleep):
			}
		}
		p.spent = 0
		p.windowStart = time.Now()
	}
	p.spent += cost
	return nil
}

// ── DB helpers ──────────────────────────────────────────────────────────────

// FetchChapterText renders a chapter's English verses as "[c:v] text" lines.
// Retained for callers that want the whole chapter as one string.
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

func clearRefinable(ctx context.Context, database *db.DB, chapter int) error {
	_, err := database.Pool.Exec(ctx, `
		DELETE FROM game_passages
		WHERE chapter_start = $1 AND status IN ('rejected', 'needs_refinement')
	`, chapter)
	return err
}

// fetchCoveredVerses returns the verses already claimed by proposed or approved
// passages in the chapter, so curation does not re-propose over them.
func fetchCoveredVerses(ctx context.Context, database *db.DB, chapter int) (map[int]bool, error) {
	rows, err := database.Pool.Query(ctx, `
		SELECT verse_start, verse_end FROM game_passages
		WHERE chapter_start = $1 AND status IN ('proposed', 'approved')
	`, chapter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	covered := make(map[int]bool)
	for rows.Next() {
		var start, end int
		if err := rows.Scan(&start, &end); err != nil {
			return nil, err
		}
		for v := start; v <= end; v++ {
			covered[v] = true
		}
	}
	return covered, rows.Err()
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
	Model               string         `json:"model"`
	Messages            []chatMessage  `json:"messages"`
	Temperature         float64        `json:"temperature"`
	MaxCompletionTokens int            `json:"max_completion_tokens,omitempty"`
	ResponseFormat      map[string]any `json:"response_format,omitempty"`
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

// ProposePassages asks the model to propose passages for one verse window of a
// chapter. verseText holds the rendered "[c:v] text" lines for verseStart..End.
func ProposePassages(ctx context.Context, cfg Config, chapter, verseStart, verseEnd int, verseText string) ([]Proposal, error) {
	user := fmt.Sprintf(
		"Chapter %d (Rashad Khalifa translation, English), verses %d-%d:\n\n%s\nOnly propose passages whose verse_start and verse_end fall within %d-%d. Return JSON as specified.",
		chapter, verseStart, verseEnd, verseText, verseStart, verseEnd,
	)

	reqBody := chatRequest{
		Model: cfg.model(),
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: user},
		},
		Temperature:         0.3,
		MaxCompletionTokens: maxCompletionTokens,
		ResponseFormat:      map[string]any{"type": "json_object"},
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
		// 413 (request too large) and 429 are Groq's token/rate-limit responses;
		// flag them so the caller can pace and retry rather than give up.
		if resp.StatusCode == http.StatusRequestEntityTooLarge ||
			resp.StatusCode == http.StatusTooManyRequests ||
			strings.Contains(string(respBody), "rate_limit_exceeded") {
			return nil, &RateLimitError{
				RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), respBody),
				Status:     resp.StatusCode,
				Body:       string(respBody),
			}
		}
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

// parseRetryAfter extracts a wait hint from the Retry-After response header or,
// failing that, the JSON error body where Groq sometimes embeds phrasing like
// "Please try again in 47.123s". Returns 0 when nothing parseable is found, so
// callers know to use their own backoff.
func parseRetryAfter(headerValue string, body []byte) time.Duration {
	if d := parseRetryAfterHeader(headerValue); d > 0 {
		return d
	}
	return parseRetryAfterBody(body)
}

func parseRetryAfterHeader(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	// RFC 7231 allows either a delta in seconds or an HTTP-date. Groq uses
	// delta-seconds, possibly fractional.
	if secs, err := strconv.ParseFloat(v, 64); err == nil && secs > 0 {
		return time.Duration(secs * float64(time.Second))
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

var retryBodyRe = regexp.MustCompile(`(?i)try again in ([0-9]+(?:\.[0-9]+)?)(ms|s|m)`)

func parseRetryAfterBody(body []byte) time.Duration {
	m := retryBodyRe.FindSubmatch(body)
	if len(m) != 3 {
		return 0
	}
	n, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil || n <= 0 {
		return 0
	}
	switch string(m[2]) {
	case "ms":
		return time.Duration(n * float64(time.Millisecond))
	case "s":
		return time.Duration(n * float64(time.Second))
	case "m":
		return time.Duration(n * float64(time.Minute))
	}
	return 0
}
