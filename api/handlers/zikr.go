package handlers

// Public zikr endpoint: the mobile app fetches the active reminder texts for
// its rotating display strip and startup splash. Lives outside the OpenAPI
// surface (registered before the validator) and needs no auth; entries are
// managed through the admin notifications routes (notifications_admin.go).

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/db"
)

type ZikrHandler struct {
	DB *db.DB
}

type zikrEntry struct {
	ID        int       `json:"id"`
	Text      string    `json:"text"`
	Lang      string    `json:"lang"`
	Source    *string   `json:"source,omitempty"`
	Weight    int       `json:"weight"`
	Active    bool      `json:"active"`
	UpdatedAt time.Time `json:"updated_at"`
}

func zikrEntryFromRow(r db.ZikrRow) zikrEntry {
	return zikrEntry{
		ID:        r.ID,
		Text:      r.Text,
		Lang:      r.Lang,
		Source:    r.Source,
		Weight:    r.Weight,
		Active:    r.Active,
		UpdatedAt: r.UpdatedAt,
	}
}

// List returns the active zikr entries ordered for display. Optional ?lang=
// filter. Responses are cache-friendly: the list changes only through admin
// edits, so an hour of CDN/browser caching is fine.
func (h ZikrHandler) List(c *gin.Context) {
	rows, err := h.DB.ListZikr(c.Request.Context(), true, c.Query("lang"))
	if err != nil {
		slog.Error("zikr: ListZikr", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	out := make([]zikrEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, zikrEntryFromRow(r))
	}
	c.Header("Cache-Control", "public, max-age=3600")
	c.JSON(http.StatusOK, gin.H{"data": out})
}
