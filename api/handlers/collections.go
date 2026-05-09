package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
)

// CollectionsHandler handles /me/collections and /collections/share routes.
type CollectionsHandler struct {
	DB *db.DB
}

// ── Response types ────────────────────────────────────────────────────────

type CollectionResponse struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	IsPublic    bool    `json:"is_public"`
	ShareToken  *string `json:"share_token,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type CollectionVerseResponse struct {
	ID           int64  `json:"id"`
	CollectionID int64  `json:"collection_id"`
	Scripture    string `json:"scripture"`
	VerseKey     string `json:"verse_key"`
	Note         string `json:"note"`
	Position     int    `json:"position"`
	CreatedAt    string `json:"created_at"`
}

type CollectionDetailResponse struct {
	CollectionResponse
	Verses []CollectionVerseResponse `json:"verses"`
}

func collectionRowToResponse(c *db.CollectionRow) CollectionResponse {
	return CollectionResponse{
		ID:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		IsPublic:    c.IsPublic,
		ShareToken:  c.ShareToken,
		CreatedAt:   c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   c.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func verseRowToResponse(v *db.CollectionVerseRow) CollectionVerseResponse {
	return CollectionVerseResponse{
		ID:           v.ID,
		CollectionID: v.CollectionID,
		Scripture:    v.Scripture,
		VerseKey:     v.VerseKey,
		Note:         v.Note,
		Position:     v.Position,
		CreatedAt:    v.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// ── Request types ─────────────────────────────────────────────────────────

type CreateCollectionRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	IsPublic    bool   `json:"is_public"`
}

type UpdateCollectionRequest struct {
	Name              string `json:"name" binding:"required"`
	Description       string `json:"description"`
	IsPublic          bool   `json:"is_public"`
	RegenerateToken   bool   `json:"regenerate_token"`
}

type AddVerseRequest struct {
	Scripture string `json:"scripture" binding:"required"`
	VerseKey  string `json:"verse_key" binding:"required"`
	Note      string `json:"note"`
}

// ── Handlers ──────────────────────────────────────────────────────────────

// ListCollections handles GET /api/v1/me/collections
func (h *CollectionsHandler) ListCollections(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	cols, err := h.DB.ListCollections(c.Request.Context(), authID)
	if err != nil {
		slog.Error("ListCollections: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	result := make([]CollectionResponse, len(cols))
	for i := range cols {
		result[i] = collectionRowToResponse(&cols[i])
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// CreateCollection handles POST /api/v1/me/collections
func (h *CollectionsHandler) CreateCollection(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	var req CreateCollectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	col, err := h.DB.CreateCollection(c.Request.Context(), authID, req.Name, req.Description, req.IsPublic)
	if err != nil {
		slog.Error("CreateCollection: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": collectionRowToResponse(col)})
}

// UpdateCollection handles PATCH /api/v1/me/collections/:id
func (h *CollectionsHandler) UpdateCollection(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid collection id"})
		return
	}
	var req UpdateCollectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	col, err := h.DB.UpdateCollection(c.Request.Context(), id, authID, req.Name, req.Description, req.IsPublic, req.RegenerateToken)
	if err != nil {
		slog.Error("UpdateCollection: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if col == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": collectionRowToResponse(col)})
}

// DeleteCollection handles DELETE /api/v1/me/collections/:id
func (h *CollectionsHandler) DeleteCollection(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid collection id"})
		return
	}
	deleted, err := h.DB.DeleteCollection(c.Request.Context(), id, authID)
	if err != nil {
		slog.Error("DeleteCollection: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if !deleted {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": nil})
}

// GetCollectionDetail handles GET /api/v1/me/collections/:id (with verses)
func (h *CollectionsHandler) GetCollectionDetail(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid collection id"})
		return
	}
	cols, err := h.DB.ListCollections(c.Request.Context(), authID)
	if err != nil {
		slog.Error("GetCollectionDetail: list error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	var col *db.CollectionRow
	for i := range cols {
		if cols[i].ID == id {
			col = &cols[i]
			break
		}
	}
	if col == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}
	verses, err := h.DB.ListCollectionVerses(c.Request.Context(), id)
	if err != nil {
		slog.Error("GetCollectionDetail: verses error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	vr := make([]CollectionVerseResponse, len(verses))
	for i := range verses {
		vr[i] = verseRowToResponse(&verses[i])
	}
	c.JSON(http.StatusOK, gin.H{"data": CollectionDetailResponse{
		CollectionResponse: collectionRowToResponse(col),
		Verses:             vr,
	}})
}

// AddVerse handles POST /api/v1/me/collections/:id/verses
func (h *CollectionsHandler) AddVerse(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid collection id"})
		return
	}
	var req AddVerseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Scripture != "quran" && req.Scripture != "bible" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scripture must be 'quran' or 'bible'"})
		return
	}
	cv, err := h.DB.AddVerseToCollection(c.Request.Context(), id, authID, req.Scripture, req.VerseKey, req.Note)
	if err != nil {
		slog.Error("AddVerse: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": verseRowToResponse(cv)})
}

// RemoveVerse handles DELETE /api/v1/me/collections/:id/verses/:verseId
func (h *CollectionsHandler) RemoveVerse(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	verseID, err := strconv.ParseInt(c.Param("verseId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid verse id"})
		return
	}
	deleted, err := h.DB.RemoveVerseFromCollection(c.Request.Context(), verseID, authID)
	if err != nil {
		slog.Error("RemoveVerse: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if !deleted {
		c.JSON(http.StatusNotFound, gin.H{"error": "verse not found in collection"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": nil})
}

// GetSharedCollection handles GET /api/v1/collections/share/:token (public, no auth)
func (h *CollectionsHandler) GetSharedCollection(c *gin.Context) {
	token := c.Param("token")
	col, err := h.DB.GetCollectionByShareToken(c.Request.Context(), token)
	if err != nil {
		slog.Error("GetSharedCollection: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if col == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}
	verses, err := h.DB.ListCollectionVerses(c.Request.Context(), col.ID)
	if err != nil {
		slog.Error("GetSharedCollection: verses error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	vr := make([]CollectionVerseResponse, len(verses))
	for i := range verses {
		vr[i] = verseRowToResponse(&verses[i])
	}
	c.JSON(http.StatusOK, gin.H{"data": CollectionDetailResponse{
		CollectionResponse: collectionRowToResponse(col),
		Verses:             vr,
	}})
}
