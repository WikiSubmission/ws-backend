package handlers

// Public, unauthenticated reads of published editorial appendices for the
// public appendices reader. Same shape and constraints as the article
// endpoints in editorial_public.go: registered before the OpenAPI validator,
// deliberately absent from openapi.yaml, and only ever serving the `published`
// snapshot. Editing happens through the authenticated /editorial admin surface.

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/db"
)

type EditorialAppendicesPublicHandler struct {
	DB *db.DB
}

// publicAppendix is the wire shape. Two body carriers ride together while the
// Portable Text migration is in flight: `body` is the original markdown and
// `body_pt` is a Portable Text block array. Readers prefer `body_pt` and fall
// back to `body`, so a partly converted corpus still reads correctly. Both are
// omitted from listings, since a converted body runs to tens of kilobytes.
//
// `body_pt` is passed straight through as stored JSON. The block schema lives
// with the renderer (packages/shared/lib/appendix-portable-text.ts) and is what
// the admin write path is validated against; nothing here inspects it.
//
// `video_id` / `video_title` are the appendix's single trailing YouTube embed,
// carried as metadata rather than as body syntax. Both are omitted when the
// appendix has no video, which is the case for 7 of the 38.
type publicAppendix struct {
	ID          int             `json:"id"`
	Code        string          `json:"code"`
	Title       string          `json:"title"`
	Snippet     string          `json:"snippet"`
	Body        string          `json:"body,omitempty"`
	BodyPT      json.RawMessage `json:"body_pt,omitempty"`
	VideoID     string          `json:"video_id,omitempty"`
	VideoTitle  string          `json:"video_title,omitempty"`
	Language    string          `json:"language"`
	VersionSlug string          `json:"version_slug"`
	VersionName string          `json:"version_name"`
	Direction   string          `json:"direction"`
	PublishedAt *time.Time      `json:"published_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

func publicAppendixFromRow(a db.PublishedAppendix, withBody bool) publicAppendix {
	out := publicAppendix{
		ID:          a.ID,
		Code:        a.Code,
		Title:       a.Title,
		Snippet:     a.Snippet,
		VideoID:     a.VideoID,
		VideoTitle:  a.VideoTitle,
		Language:    a.Language,
		VersionSlug: a.VersionSlug,
		VersionName: a.VersionName,
		Direction:   a.Direction,
		PublishedAt: a.PublishedAt,
		UpdatedAt:   a.UpdatedAt,
	}
	if withBody {
		out.Body = a.Body
		// A published appendix that has never been converted stores no
		// `body_pt`; a JSON `null` would reach the client as a body it then has
		// to test for, so it is dropped instead and `omitempty` hides the key.
		if len(a.BodyPT) > 0 && string(a.BodyPT) != "null" {
			out.BodyPT = a.BodyPT
		}
	}
	return out
}

// List returns published appendices in reading order, optionally scoped to
// ?language (the owning Quran version's language code) and/or ?version (an
// exact quran_versions slug). Bodies are omitted.
func (h EditorialAppendicesPublicHandler) List(c *gin.Context) {
	filter := db.AppendixFilter{
		Language:    c.Query("language"),
		VersionSlug: c.Query("version"),
	}
	rows, err := h.DB.ListPublishedAppendices(c.Request.Context(), filter)
	if err != nil {
		slog.Error("editorial public: ListPublishedAppendices", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	out := make([]publicAppendix, 0, len(rows))
	for _, r := range rows {
		out = append(out, publicAppendixFromRow(r, false))
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// GetByCode returns one published appendix by language code and appendix code,
// including both of its bodies. ?version pins an exact Quran version when a
// language carries more than one.
func (h EditorialAppendicesPublicHandler) GetByCode(c *gin.Context) {
	lang, code := c.Param("lang"), c.Param("code")
	if lang == "" || code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "language and code are required"})
		return
	}
	filter := db.AppendixFilter{Language: lang, VersionSlug: c.Query("version")}
	a, err := h.DB.GetPublishedAppendixByCode(c.Request.Context(), code, filter)
	if errors.Is(err, db.ErrContentNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"message": "appendix not found"})
		return
	}
	if err != nil {
		slog.Error("editorial public: GetPublishedAppendixByCode", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, gin.H{"data": publicAppendixFromRow(*a, true)})
}
