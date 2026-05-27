package handlers

// Editor-gated admin handlers for the Quran Games catalog. These are plain Gin
// handlers (not the OpenAPI strict server) registered under
// /api/v1/admin/games and protected by auth.RequireAuth + auth.RequireEditor.
// They back the editorial review web UI; see docs/games-review-web-spec.md.

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
)

// GamesAdminHandler holds the DB reference for the editorial routes.
type GamesAdminHandler struct {
	DB *db.DB
}

// reviewPassage is the editor-facing passage shape. Unlike the public
// GamePassage it includes the editorial fields (rationale, llm hints, status,
// approval metadata).
type reviewPassage struct {
	ID            int64      `json:"id"`
	ChapterStart  int        `json:"chapter_start"`
	VerseStart    int        `json:"verse_start"`
	ChapterEnd    int        `json:"chapter_end"`
	VerseEnd      int        `json:"verse_end"`
	Label         string     `json:"label"`
	Themes        []string   `json:"themes"`
	Rationale     string     `json:"rationale"`
	LlmDifficulty *string    `json:"llm_difficulty,omitempty"`
	LlmBlankHint  *int       `json:"llm_blank_hint,omitempty"`
	Status        string     `json:"status"`
	Source        string     `json:"source"`
	ApprovedAt    *time.Time `json:"approved_at,omitempty"`
	ApprovedBy    *int       `json:"approved_by,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func reviewPassageFromRow(p *db.GamePassageRow) reviewPassage {
	themes := p.Themes
	if themes == nil {
		themes = []string{}
	}
	return reviewPassage{
		ID:            p.ID,
		ChapterStart:  p.ChapterStart,
		VerseStart:    p.VerseStart,
		ChapterEnd:    p.ChapterEnd,
		VerseEnd:      p.VerseEnd,
		Label:         p.Label,
		Themes:        themes,
		Rationale:     p.Rationale,
		LlmDifficulty: p.LlmDifficulty,
		LlmBlankHint:  p.LlmBlankHint,
		Status:        p.Status,
		Source:        p.Source,
		ApprovedAt:    p.ApprovedAt,
		ApprovedBy:    p.ApprovedBy,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}

var reviewStatuses = map[string]struct{}{
	"approved":         {},
	"rejected":         {},
	"needs_refinement": {},
}

// ListPassages handles GET /admin/games/passages?status=&chapter=
func (h *GamesAdminHandler) ListPassages(c *gin.Context) {
	var statusFilter *string
	if s := c.Query("status"); s != "" && s != "all" {
		statusFilter = &s
	}
	var chapterFilter *int
	if ch := c.Query("chapter"); ch != "" {
		n, err := strconv.Atoi(ch)
		if err != nil || n < 1 || n > 114 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "chapter must be 1-114"})
			return
		}
		chapterFilter = &n
	}

	rows, err := h.DB.FetchPassagesForReview(c.Request.Context(), statusFilter, chapterFilter)
	if err != nil {
		slog.Error("admin ListPassages: db error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	out := make([]reviewPassage, len(rows))
	for i := range rows {
		out[i] = reviewPassageFromRow(&rows[i])
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// GetPassage handles GET /admin/games/passages/:id
func (h *GamesAdminHandler) GetPassage(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
		return
	}
	p, err := h.DB.FetchPassageByID(c.Request.Context(), id)
	if err != nil {
		slog.Error("admin GetPassage: db error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "passage not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": reviewPassageFromRow(p)})
}

type setStatusRequest struct {
	Status string `json:"status"`
}

// SetStatus handles POST /admin/games/passages/:id/status
func (h *GamesAdminHandler) SetStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
		return
	}
	var req setStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}
	if _, ok := reviewStatuses[req.Status]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "status must be one of approved, rejected, needs_refinement"})
		return
	}

	var editor *string
	if req.Status == "approved" {
		if e := c.GetString(auth.ContextKeyEmail); e != "" {
			editor = &e
		}
	}
	ok, err := h.DB.SetPassageStatus(c.Request.Context(), id, req.Status, editor)
	if err != nil {
		slog.Error("admin SetStatus: db error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"message": "passage not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": id, "status": req.Status}})
}

// SeedFrequency handles POST /admin/games/maintenance/seed-frequency
// Recomputes game_word_frequency from verse_text in-process. Safe to re-run.
func (h *GamesAdminHandler) SeedFrequency(c *gin.Context) {
	count, err := h.DB.RebuildWordFrequency(c.Request.Context(), "en")
	if err != nil {
		slog.Error("admin SeedFrequency: failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "frequency rebuild failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"language": "en", "tokens": count}})
}

// LoadLemmas handles POST /admin/games/maintenance/load-lemmas
// Loads the lemma fixture embedded in the running binary into game_lemma.
// Generating the fixture requires the offline spaCy sidecar; this endpoint only
// loads whatever ships with the current deployment.
func (h *GamesAdminHandler) LoadLemmas(c *gin.Context) {
	count, err := h.DB.LoadLemmaFixture(c.Request.Context())
	if err != nil {
		slog.Error("admin LoadLemmas: failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "lemma load failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"language": "en", "lemma_rows": count}})
}
