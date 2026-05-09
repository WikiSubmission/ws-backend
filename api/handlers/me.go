package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
)

// MeHandler holds the DB reference for /me/* route handlers.
type MeHandler struct {
	DB *db.DB
}

// ── Bookmark response types ────────────────────────────────────────────────

type BookmarkResponse struct {
	ID        int64  `json:"id"`
	Scripture string `json:"scripture"`
	VerseKey  string `json:"verse_key"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Kind      string `json:"kind"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type CreateBookmarkRequest struct {
	Scripture string `json:"scripture" binding:"required"`
	VerseKey  string `json:"verse_key" binding:"required"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Kind      string `json:"kind"`
}

type UpdateBookmarkRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

type CoverToCoverRequest struct {
	Scripture string `json:"scripture" binding:"required"`
	VerseKey  string `json:"verse_key" binding:"required"`
}

type ScriptureStateResponse struct {
	Bookmarks map[string]BookmarkResponse `json:"bookmarks"`
}

func bookmarkRowToResponse(b *db.BookmarkRow) BookmarkResponse {
	return BookmarkResponse{
		ID:        b.ID,
		Scripture: b.Scripture,
		VerseKey:  b.VerseKey,
		Name:      b.Name,
		Color:     b.Color,
		Kind:      b.Kind,
		CreatedAt: b.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: b.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// ── Handlers ──────────────────────────────────────────────────────────────

// GetBookmarks handles GET /api/v1/me/bookmarks?scripture=quran|bible
func (h *MeHandler) GetBookmarks(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	scripture := c.DefaultQuery("scripture", "quran")
	if scripture != "quran" && scripture != "bible" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scripture must be 'quran' or 'bible'"})
		return
	}
	bookmarks, err := h.DB.FetchBookmarks(c.Request.Context(), authID, scripture)
	if err != nil {
		slog.Error("GetBookmarks: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	result := make([]BookmarkResponse, len(bookmarks))
	for i := range bookmarks {
		result[i] = bookmarkRowToResponse(&bookmarks[i])
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// CreateBookmark handles POST /api/v1/me/bookmarks
func (h *MeHandler) CreateBookmark(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	var req CreateBookmarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Scripture != "quran" && req.Scripture != "bible" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scripture must be 'quran' or 'bible'"})
		return
	}
	if req.Color == "" {
		req.Color = "amber"
	}
	if req.Kind == "" {
		req.Kind = "normal"
	}
	if req.Kind != "normal" && req.Kind != "cover_to_cover" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kind must be 'normal' or 'cover_to_cover'"})
		return
	}
	b, err := h.DB.CreateBookmark(c.Request.Context(), authID, req.Scripture, req.VerseKey, req.Name, req.Color, req.Kind)
	if err != nil {
		slog.Error("CreateBookmark: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": bookmarkRowToResponse(b)})
}

// UpdateBookmark handles PATCH /api/v1/me/bookmarks/:id
func (h *MeHandler) UpdateBookmark(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmark id"})
		return
	}
	var req UpdateBookmarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Fetch existing first so we can apply partial update.
	existing, err := h.DB.FetchBookmarks(c.Request.Context(), authID, "quran")
	if err != nil {
		slog.Error("UpdateBookmark: fetch error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	// Also check bible.
	existingBible, err := h.DB.FetchBookmarks(c.Request.Context(), authID, "bible")
	if err != nil {
		slog.Error("UpdateBookmark: fetch bible error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	existing = append(existing, existingBible...)

	var currentName, currentColor string
	for _, bm := range existing {
		if bm.ID == id {
			currentName = bm.Name
			currentColor = bm.Color
			break
		}
	}

	if req.Name != nil {
		currentName = *req.Name
	}
	if req.Color != nil {
		currentColor = *req.Color
	}

	b, err := h.DB.UpdateBookmark(c.Request.Context(), id, authID, currentName, currentColor)
	if err != nil {
		slog.Error("UpdateBookmark: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if b == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bookmark not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": bookmarkRowToResponse(b)})
}

// DeleteBookmark handles DELETE /api/v1/me/bookmarks/:id
func (h *MeHandler) DeleteBookmark(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmark id"})
		return
	}
	deleted, err := h.DB.DeleteBookmark(c.Request.Context(), id, authID)
	if err != nil {
		slog.Error("DeleteBookmark: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if !deleted {
		c.JSON(http.StatusNotFound, gin.H{"error": "bookmark not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": nil})
}

// PutCoverToCover handles PUT /api/v1/me/cover-to-cover
func (h *MeHandler) PutCoverToCover(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	var req CoverToCoverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Scripture != "quran" && req.Scripture != "bible" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scripture must be 'quran' or 'bible'"})
		return
	}
	b, err := h.DB.UpsertCoverToCover(c.Request.Context(), authID, req.Scripture, req.VerseKey)
	if err != nil {
		slog.Error("PutCoverToCover: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": bookmarkRowToResponse(b)})
}

// GetScriptureState handles GET /api/v1/me/scripture-state?scripture=quran&chapter=1
// Returns bookmarks keyed by verse_key for O(1) lookup in the verse list.
func (h *MeHandler) GetScriptureState(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	scripture := c.DefaultQuery("scripture", "quran")
	if scripture != "quran" && scripture != "bible" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scripture must be 'quran' or 'bible'"})
		return
	}
	chapterStr := c.Query("chapter")
	if chapterStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapter is required"})
		return
	}
	chapter, err := strconv.Atoi(chapterStr)
	if err != nil || chapter < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chapter"})
		return
	}

	bookmarks, err := h.DB.FetchChapterBookmarks(c.Request.Context(), authID, scripture, chapter)
	if err != nil {
		slog.Error("GetScriptureState: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	bmMap := make(map[string]BookmarkResponse, len(bookmarks))
	for i := range bookmarks {
		bm := &bookmarks[i]
		bmMap[bm.VerseKey] = bookmarkRowToResponse(bm)
	}
	c.JSON(http.StatusOK, ScriptureStateResponse{Bookmarks: bmMap})
}
