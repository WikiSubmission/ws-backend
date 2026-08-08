package handlers

// Public, unauthenticated reads of published editorial articles for the public
// blog (Sanity replacement). Lives outside the OpenAPI surface (registered
// before the validator) and needs no auth — only the `published` snapshot is
// ever exposed. Managed through the authenticated /editorial admin surface.

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/db"
)

type EditorialPublicHandler struct {
	DB *db.DB
}

type publicArticle struct {
	ID                  int             `json:"id"`
	TranslationGroup    string          `json:"translation_group"`
	Slug                string          `json:"slug"`
	Language            string          `json:"language"`
	Title               string          `json:"title"`
	Excerpt             string          `json:"excerpt"`
	ThumbnailURL        string          `json:"thumbnail_url"`
	EnableScriptureRefs bool            `json:"enable_scripture_refs"`
	ThumbnailText       json.RawMessage `json:"thumbnail_text"`
	Body                json.RawMessage `json:"body,omitempty"`
	Category            string          `json:"category"`
	CategorySlug        string          `json:"category_slug"`
	AuthorName          string          `json:"author_name"`
	AuthorPhotoURL      string          `json:"author_photo_url"`
	AuthorSlug          string          `json:"author_slug"`
	PublishedAt         *time.Time      `json:"published_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

func publicArticleFromRow(a db.PublishedArticle, withBody bool) publicArticle {
	out := publicArticle{
		ID:                  a.ID,
		TranslationGroup:    a.TranslationGroup,
		Slug:                a.Slug,
		Language:            a.Language,
		Title:               a.Title,
		Excerpt:             a.Excerpt,
		ThumbnailURL:        a.ThumbnailURL,
		EnableScriptureRefs: a.EnableScriptureRefs,
		ThumbnailText:       a.ThumbnailText,
		Category:            a.CategoryName,
		CategorySlug:        a.CategorySlug,
		AuthorName:          a.AuthorName,
		AuthorPhotoURL:      a.AuthorPhotoURL,
		AuthorSlug:          a.AuthorSlug,
		PublishedAt:         a.PublishedAt,
		UpdatedAt:           a.UpdatedAt,
	}
	if withBody {
		out.Body = a.Body
	}
	return out
}

func publicArticleList(rows []db.PublishedArticle) []publicArticle {
	out := make([]publicArticle, 0, len(rows))
	for _, r := range rows {
		out = append(out, publicArticleFromRow(r, false))
	}
	return out
}

// clampLimit parses ?limit with a default and hard ceiling.
func clampLimit(raw string, def, max int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

// List returns published articles, newest first, optionally scoped to ?language.
func (h EditorialPublicHandler) List(c *gin.Context) {
	limit := clampLimit(c.Query("limit"), 100, 200)
	offset := 0
	if n, err := strconv.Atoi(c.Query("offset")); err == nil && n > 0 {
		offset = n
	}
	rows, err := h.DB.ListPublishedArticles(c.Request.Context(), c.Query("language"), limit, offset)
	if err != nil {
		slog.Error("editorial public: ListPublishedArticles", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, gin.H{"data": publicArticleList(rows)})
}

// GetBySlug returns one published article by language code and slug.
func (h EditorialPublicHandler) GetBySlug(c *gin.Context) {
	lang, slug := c.Param("lang"), c.Param("slug")
	if lang == "" || slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "language and slug are required"})
		return
	}
	a, err := h.DB.GetPublishedArticleBySlug(c.Request.Context(), lang, slug)
	if errors.Is(err, db.ErrContentNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"message": "article not found"})
		return
	}
	if err != nil {
		slog.Error("editorial public: GetPublishedArticleBySlug", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, gin.H{"data": publicArticleFromRow(*a, true)})
}

// Related returns other published articles sharing the primary category and
// language of the article at :lang/:slug.
func (h EditorialPublicHandler) Related(c *gin.Context) {
	lang, slug := c.Param("lang"), c.Param("slug")
	if lang == "" || slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "language and slug are required"})
		return
	}
	a, err := h.DB.GetPublishedArticleBySlug(c.Request.Context(), lang, slug)
	if errors.Is(err, db.ErrContentNotFound) {
		c.JSON(http.StatusOK, gin.H{"data": []publicArticle{}})
		return
	}
	if err != nil {
		slog.Error("editorial public: Related lookup", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	limit := clampLimit(c.Query("limit"), 4, 12)
	rows, err := h.DB.ListRelatedPublishedArticles(c.Request.Context(), lang, a.CategorySlug, a.ID, limit)
	if err != nil {
		slog.Error("editorial public: ListRelatedPublishedArticles", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, gin.H{"data": publicArticleList(rows)})
}

// Search matches ?q against title, excerpt and body text, optionally scoped to
// ?language.
func (h EditorialPublicHandler) Search(c *gin.Context) {
	limit := clampLimit(c.Query("limit"), 30, 100)
	rows, err := h.DB.SearchPublishedArticles(c.Request.Context(), c.Query("q"), c.Query("language"), limit)
	if err != nil {
		slog.Error("editorial public: SearchPublishedArticles", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	// Include body so the caller can build match snippets.
	out := make([]publicArticle, 0, len(rows))
	for _, r := range rows {
		out = append(out, publicArticleFromRow(r, true))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}
