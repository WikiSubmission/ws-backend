package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
)

// MeActivityHandler serves the user-facing activity feed and consent flag.
// These routes live outside the OpenAPI surface (registered before the
// validator) so they do not require regenerating the strict server.
type MeActivityHandler struct {
	DB *db.DB
}

// ensureUser provisions a users row on first write, mirroring
// Server.ensureCurrentUser but without the default-bookmark-category side
// effect (irrelevant for activity).
func (h MeActivityHandler) ensureUser(c *gin.Context, authID string) bool {
	user, err := h.DB.FetchUserByAuthID(c.Request.Context(), authID)
	if err != nil {
		slog.Error("me_activity: FetchUserByAuthID", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return false
	}
	if user != nil {
		return true
	}
	email := c.GetString(auth.ContextKeyEmail)
	if email == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "email claim required"})
		return false
	}
	if _, err := h.DB.UpsertUser(c.Request.Context(), authID, email, nil); err != nil {
		slog.Error("me_activity: UpsertUser", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return false
	}
	return true
}

func authIDFromGin(c *gin.Context) (string, bool) {
	v := c.GetString(auth.ContextKeyUID)
	return v, v != ""
}

var validActivityKinds = map[string]struct{}{
	"search":             {},
	"browse_chapter":     {},
	"browse_verse":       {},
	"browse_verse_range": {},
}

var validActivityScriptures = map[string]struct{}{
	"quran": {},
	"bible": {},
}

type recordActivityRequest struct {
	Kind      string  `json:"kind"`
	Scripture string  `json:"scripture"`
	VerseKey  *string `json:"verse_key,omitempty"`
	Query     *string `json:"query,omitempty"`
}

// Record stores one activity event. Silently no-ops with 202 Accepted when
// consent is off so the client need not check consent first.
func (h MeActivityHandler) Record(c *gin.Context) {
	authID, ok := authIDFromGin(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "not authenticated"})
		return
	}
	var req recordActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}
	if _, ok := validActivityKinds[req.Kind]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid kind"})
		return
	}
	if _, ok := validActivityScriptures[req.Scripture]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid scripture"})
		return
	}
	if req.Query != nil {
		q := strings.TrimSpace(*req.Query)
		if q == "" {
			req.Query = nil
		} else {
			if len(q) > 500 {
				q = q[:500]
			}
			req.Query = &q
		}
	}
	if req.VerseKey != nil {
		k := strings.TrimSpace(*req.VerseKey)
		if k == "" {
			req.VerseKey = nil
		} else {
			req.VerseKey = &k
		}
	}
	if !h.ensureUser(c, authID) {
		return
	}
	stored, err := h.DB.InsertActivity(c.Request.Context(), authID, req.Kind, req.Scripture, req.VerseKey, req.Query)
	if err != nil {
		slog.Error("me_activity: InsertActivity", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	if !stored {
		c.JSON(http.StatusAccepted, gin.H{"stored": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stored": true})
}

// List returns the user's recent activity, newest first.
func (h MeActivityHandler) List(c *gin.Context) {
	authID, ok := authIDFromGin(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "not authenticated"})
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	rows, err := h.DB.ListActivity(c.Request.Context(), authID, limit, offset)
	if err != nil {
		slog.Error("me_activity: ListActivity", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	if rows == nil {
		rows = []db.ActivityRow{}
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// Clear wipes every activity row for the user.
func (h MeActivityHandler) Clear(c *gin.Context) {
	authID, ok := authIDFromGin(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "not authenticated"})
		return
	}
	n, err := h.DB.ClearActivity(c.Request.Context(), authID)
	if err != nil {
		slog.Error("me_activity: ClearActivity", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": n})
}

// GetConsent returns the current consent flag.
func (h MeActivityHandler) GetConsent(c *gin.Context) {
	authID, ok := authIDFromGin(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "not authenticated"})
		return
	}
	consent, err := h.DB.GetActivityConsent(c.Request.Context(), authID)
	if err != nil {
		slog.Error("me_activity: GetActivityConsent", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"consent": consent})
}

type setConsentRequest struct {
	Consent bool `json:"consent"`
}

// SetConsent updates the consent flag. Revoking does not wipe existing
// activity — the user can clear it separately via DELETE /me/activity.
func (h MeActivityHandler) SetConsent(c *gin.Context) {
	authID, ok := authIDFromGin(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "not authenticated"})
		return
	}
	var req setConsentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}
	if !h.ensureUser(c, authID) {
		return
	}
	v, err := h.DB.SetActivityConsent(c.Request.Context(), authID, req.Consent)
	if err != nil {
		slog.Error("me_activity: SetActivityConsent", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"consent": v})
}
