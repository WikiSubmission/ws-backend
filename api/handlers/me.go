package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

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

// ── Note response types ────────────────────────────────────────────────────

type NoteResponse struct {
	ID        int64  `json:"id"`
	Scripture string `json:"scripture"`
	VerseKey  string `json:"verse_key"`
	Lang      string `json:"lang"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type CreateNoteRequest struct {
	Scripture string `json:"scripture" binding:"required"`
	VerseKey  string `json:"verse_key" binding:"required"`
	Lang      string `json:"lang"`
	Content   string `json:"content" binding:"required"`
}

type UpdateNoteRequest struct {
	Content string `json:"content" binding:"required"`
}

type ScriptureStateResponse struct {
	Bookmarks map[string]BookmarkResponse   `json:"bookmarks"`
	Notes     map[string][]NoteResponse     `json:"notes"`
}

func noteRowToResponse(n *db.NoteRow) NoteResponse {
	return NoteResponse{
		ID:        n.ID,
		Scripture: n.Scripture,
		VerseKey:  n.VerseKey,
		Lang:      n.Lang,
		Content:   n.Content,
		CreatedAt: n.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: n.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
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
		slog.Error("GetScriptureState: bookmarks DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	notes, err := h.DB.FetchChapterNotes(c.Request.Context(), authID, scripture, chapter)
	if err != nil {
		slog.Error("GetScriptureState: notes DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	bmMap := make(map[string]BookmarkResponse, len(bookmarks))
	for i := range bookmarks {
		bm := &bookmarks[i]
		bmMap[bm.VerseKey] = bookmarkRowToResponse(bm)
	}
	noteMap := make(map[string][]NoteResponse)
	for i := range notes {
		n := &notes[i]
		noteMap[n.VerseKey] = append(noteMap[n.VerseKey], noteRowToResponse(n))
	}
	c.JSON(http.StatusOK, ScriptureStateResponse{Bookmarks: bmMap, Notes: noteMap})
}

// ── Note handlers ─────────────────────────────────────────────────────────

// GetNotes handles GET /api/v1/me/notes?scripture=quran[&verse_key=1:1][&lang=en]
func (h *MeHandler) GetNotes(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	scripture := c.DefaultQuery("scripture", "quran")
	if scripture != "quran" && scripture != "bible" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scripture must be 'quran' or 'bible'"})
		return
	}
	var verseKey, lang *string
	if v := c.Query("verse_key"); v != "" {
		verseKey = &v
	}
	if l := c.Query("lang"); l != "" {
		lang = &l
	}
	notes, err := h.DB.FetchNotes(c.Request.Context(), authID, scripture, verseKey, lang)
	if err != nil {
		slog.Error("GetNotes: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	result := make([]NoteResponse, len(notes))
	for i := range notes {
		result[i] = noteRowToResponse(&notes[i])
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// CreateNote handles POST /api/v1/me/notes
func (h *MeHandler) CreateNote(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	var req CreateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Scripture != "quran" && req.Scripture != "bible" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scripture must be 'quran' or 'bible'"})
		return
	}
	if req.Lang == "" {
		req.Lang = "en"
	}
	n, err := h.DB.CreateNote(c.Request.Context(), authID, req.Scripture, req.VerseKey, req.Lang, req.Content)
	if err != nil {
		slog.Error("CreateNote: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": noteRowToResponse(n)})
}

// UpdateNote handles PATCH /api/v1/me/notes/:id
func (h *MeHandler) UpdateNote(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid note id"})
		return
	}
	var req UpdateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	n, err := h.DB.UpdateNote(c.Request.Context(), id, authID, req.Content)
	if err != nil {
		slog.Error("UpdateNote: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if n == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "note not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": noteRowToResponse(n)})
}

// DeleteNote handles DELETE /api/v1/me/notes/:id
func (h *MeHandler) DeleteNote(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid note id"})
		return
	}
	deleted, err := h.DB.DeleteNote(c.Request.Context(), id, authID)
	if err != nil {
		slog.Error("DeleteNote: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if !deleted {
		c.JSON(http.StatusNotFound, gin.H{"error": "note not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": nil})
}

// ── Reading progress handlers ─────────────────────────────────────────────

type ReadingProgressResponse struct {
	Scripture string `json:"scripture"`
	VerseKey  string `json:"verse_key"`
	UpdatedAt string `json:"updated_at"`
}

type UpsertReadingProgressRequest struct {
	Scripture string `json:"scripture" binding:"required"`
	VerseKey  string `json:"verse_key" binding:"required"`
}

// GetReadingProgress handles GET /api/v1/me/reading-progress?scripture=quran|bible
func (h *MeHandler) GetReadingProgress(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	scripture := c.DefaultQuery("scripture", "quran")
	if scripture != "quran" && scripture != "bible" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scripture must be 'quran' or 'bible'"})
		return
	}
	rp, err := h.DB.GetReadingProgress(c.Request.Context(), authID, scripture)
	if err != nil {
		slog.Error("GetReadingProgress: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if rp == nil {
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": ReadingProgressResponse{
		Scripture: rp.Scripture,
		VerseKey:  rp.VerseKey,
		UpdatedAt: rp.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}})
}

// PutReadingProgress handles PUT /api/v1/me/reading-progress
func (h *MeHandler) PutReadingProgress(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	var req UpsertReadingProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Scripture != "quran" && req.Scripture != "bible" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scripture must be 'quran' or 'bible'"})
		return
	}
	rp, err := h.DB.UpsertReadingProgress(c.Request.Context(), authID, req.Scripture, req.VerseKey)
	if err != nil {
		slog.Error("PutReadingProgress: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": ReadingProgressResponse{
		Scripture: rp.Scripture,
		VerseKey:  rp.VerseKey,
		UpdatedAt: rp.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}})
}

// ── Streak handlers ───────────────────────────────────────────────────────

type StreakResponse struct {
	CurrentStreak   int    `json:"current_streak"`
	LongestStreak   int    `json:"longest_streak"`
	LastActiveDay   string `json:"last_active_day,omitempty"`
	TotalVersesRead int    `json:"total_verses_read"`
}

type IncrementReadingLogRequest struct {
	Scripture  string `json:"scripture" binding:"required"`
	VersesRead int    `json:"verses_read"`
	Day        string `json:"day"` // optional, ISO date e.g. "2026-05-09"
}

// GetStreak handles GET /api/v1/me/streak?scripture=quran|bible
func (h *MeHandler) GetStreak(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	scripture := c.DefaultQuery("scripture", "quran")
	if scripture != "quran" && scripture != "bible" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scripture must be 'quran' or 'bible'"})
		return
	}
	streak, err := h.DB.GetStreak(c.Request.Context(), authID, scripture)
	if err != nil {
		slog.Error("GetStreak: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": StreakResponse{
		CurrentStreak:   streak.CurrentStreak,
		LongestStreak:   streak.LongestStreak,
		LastActiveDay:   streak.LastActiveDay,
		TotalVersesRead: streak.TotalVersesRead,
	}})
}

// PostReadingLog handles POST /api/v1/me/reading-log
func (h *MeHandler) PostReadingLog(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	var req IncrementReadingLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Scripture != "quran" && req.Scripture != "bible" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scripture must be 'quran' or 'bible'"})
		return
	}
	if req.VersesRead <= 0 {
		req.VersesRead = 1
	}
	if req.Day == "" {
		req.Day = time.Now().UTC().Format("2006-01-02")
	}
	if err := h.DB.IncrementReadingLog(c.Request.Context(), authID, req.Scripture, req.Day, req.VersesRead); err != nil {
		slog.Error("PostReadingLog: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": nil})
}
