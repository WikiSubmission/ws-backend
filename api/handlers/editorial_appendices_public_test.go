package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/db"
)

func sampleAppendixRow() db.PublishedAppendix {
	published := time.Date(2026, 4, 27, 18, 46, 24, 0, time.UTC)
	return db.PublishedAppendix{
		ID:          7,
		Code:        "19",
		Title:       "Hadith and Sunna: Satanic Innovations",
		Snippet:     "The Quran is complete, perfect and fully detailed.",
		Body:        "## Heading\n\nBody markdown.",
		VideoID:     "2UxqrtHtYd0",
		VideoTitle:  "Appendix 19 — Hadith & Sunna: Satanic Innovations",
		Language:    "en",
		VersionSlug: "authorized-translation",
		VersionName: "Authorized Translation",
		Direction:   "ltr",
		PublishedAt: &published,
		UpdatedAt:   published,
	}
}

// Listings must not carry the markdown body; the single-appendix view must.
func TestPublicAppendixFromRowBodyGating(t *testing.T) {
	row := sampleAppendixRow()

	if got := publicAppendixFromRow(row, false); got.Body != "" {
		t.Errorf("listing DTO leaked the body: %q", got.Body)
	}
	full := publicAppendixFromRow(row, true)
	if full.Body != row.Body {
		t.Errorf("Body = %q, want %q", full.Body, row.Body)
	}
	if full.Code != row.Code || full.Title != row.Title || full.Snippet != row.Snippet {
		t.Errorf("scalar fields did not round-trip: %+v", full)
	}
	if full.Language != "en" || full.VersionSlug != "authorized-translation" || full.Direction != "ltr" {
		t.Errorf("version fields did not round-trip: %+v", full)
	}
}

// `body,omitempty` keeps the key out of listing payloads entirely rather than
// serializing an empty string.
func TestPublicAppendixListingOmitsBodyKey(t *testing.T) {
	raw, err := json.Marshal(publicAppendixFromRow(sampleAppendixRow(), false))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := decoded["body"]; ok {
		t.Errorf("listing JSON contains a body key: %s", raw)
	}
	for _, key := range []string{"code", "title", "snippet", "language", "version_slug", "direction"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("listing JSON is missing %q: %s", key, raw)
		}
	}
}

// The single trailing YouTube embed is metadata, so it survives on listings as
// well as on the full view, unlike the body.
func TestPublicAppendixCarriesVideoOnBothViews(t *testing.T) {
	row := sampleAppendixRow()

	for _, withBody := range []bool{false, true} {
		got := publicAppendixFromRow(row, withBody)
		if got.VideoID != row.VideoID || got.VideoTitle != row.VideoTitle {
			t.Errorf("withBody=%v: video = %q/%q, want %q/%q",
				withBody, got.VideoID, got.VideoTitle, row.VideoID, row.VideoTitle)
		}
	}
}

// 7 of the 38 appendices have no video; those rows must not carry empty video
// keys on the wire, so the reader can branch on presence alone.
func TestPublicAppendixOmitsVideoKeysWhenAbsent(t *testing.T) {
	row := sampleAppendixRow()
	row.VideoID, row.VideoTitle = "", ""

	raw, err := json.Marshal(publicAppendixFromRow(row, true))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"video_id", "video_title"} {
		if _, ok := decoded[key]; ok {
			t.Errorf("JSON contains an empty %q: %s", key, raw)
		}
	}

	raw, err = json.Marshal(publicAppendixFromRow(sampleAppendixRow(), true))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["video_id"] != "2UxqrtHtYd0" {
		t.Errorf("video_id = %v, want 2UxqrtHtYd0: %s", decoded["video_id"], raw)
	}
}

// The appendix routes sit beside the article routes under the same prefix, so
// registration must not trip gin's wildcard conflict check, and :lang/:code
// must bind.
func TestAppendixRoutesRegisterBesideArticles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	r.GET("/api/v1/editorial/public/articles", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/api/v1/editorial/public/articles/:lang/:slug", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/api/v1/editorial/public/search", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/api/v1/editorial/public/appendices", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/api/v1/editorial/public/appendices/:lang/:code", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"lang": c.Param("lang"), "code": c.Param("code")})
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/editorial/public/appendices/en/19", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["lang"] != "en" || got["code"] != "19" {
		t.Errorf("params = %v, want lang=en code=19", got)
	}
}

// Missing path params answer 400 rather than reaching the database.
func TestGetByCodeRejectsEmptyParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/editorial/public/appendices//", nil)

	EditorialAppendicesPublicHandler{}.GetByCode(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}
