package handlers

// Admin-gated reseed for the site catalogue that backs /site/search and the
// command menu. Plain Gin handlers under /api/v1/admin/site-docs, protected by
// auth.RequireAuth + auth.RequireAdmin — the same shape offline_admin.go and
// topic_index_admin.go use.
//
// Exists for the same reason the topical index reseed does: db/seeds/site_docs_*
// is embedded in this binary via //go:embed in db/site.go, and db/ is copied into
// the image, but api/cmd/seed_site is not — the Dockerfile builds only ./api/. So
// a deployed environment holds the data and had no way to load it, and
// `just seed-site` needs direct network reach to that environment's Postgres.
//
// This matters more than a one-off: adding a route to the manifest in
// wikisubmission-org regenerates these seeds, and the web build now fails until
// the route is registered (apps/web/scripts/check-site-routes.ts). So the
// catalogue changes on a normal cadence, and every change needs a way to reach
// production.
//
// Runs both halves of `just seed-site`: the embedded page rows, then the chapter
// and appendix rows derived in SQL from chapter_text and appendices. Each is a
// full replace inside its own `source` scope, so editorial rows are untouched.

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/db"
)

// siteReseedTimeout bounds the two transactions together. Generous relative to
// the real cost so a slow link to Postgres does not abort one partway.
const siteReseedTimeout = 3 * time.Minute

// SiteAdminHandler serves the admin site-catalogue routes.
type SiteAdminHandler struct {
	DB *db.DB
}

func NewSiteAdminHandler(database *db.DB) *SiteAdminHandler {
	return &SiteAdminHandler{DB: database}
}

func (h *SiteAdminHandler) census(ctx context.Context) gin.H {
	counts, err := h.DB.CountSiteDocuments(ctx)
	if err != nil {
		slog.Warn("site documents census failed", slog.Any("error", err))
		return nil
	}
	return gin.H{
		"seed":      counts.Seed,
		"derived":   counts.Derived,
		"editorial": counts.Editorial,
		"sections":  counts.Sections,
	}
}

// Status reports the row census by source scope, so an operator can see whether
// a reseed landed and whether the crawler's section rows survived it.
// GET /api/v1/admin/site-docs
func (h *SiteAdminHandler) Status(c *gin.Context) {
	counts, err := h.DB.CountSiteDocuments(c.Request.Context())
	if err != nil {
		slog.Error("site documents census failed", slog.Any("error", err))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "could not count the site catalogue"})
		return
	}

	langs, err := h.DB.FetchSiteLangs(c.Request.Context())
	if err != nil {
		slog.Error("site langs failed", slog.Any("error", err))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "could not list the indexed locales"})
		return
	}
	locales := make([]string, 0, len(langs))
	for lang := range langs {
		locales = append(locales, lang)
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"seed":      counts.Seed,
		"derived":   counts.Derived,
		"editorial": counts.Editorial,
		"sections":  counts.Sections,
		"locales":   locales,
	}})
}

// Reseed loads the embedded page rows, then rebuilds the derived ones.
// POST /api/v1/admin/site-docs/reseed
func (h *SiteAdminHandler) Reseed(c *gin.Context) {
	// Detached from the request context on purpose: an operator closing the tab
	// must not abort a replace partway.
	ctx, cancel := context.WithTimeout(context.Background(), siteReseedTimeout)
	defer cancel()

	before := h.census(ctx)

	seeded, err := h.DB.SeedSiteDocuments(ctx)
	if err != nil {
		slog.Error("site catalogue reseed failed", slog.Any("error", err))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "seed failed: " + err.Error()})
		return
	}

	derived, err := h.DB.RebuildDerivedSiteDocuments(ctx)
	if err != nil {
		// The page rows are already committed. Say so rather than implying the
		// whole reseed rolled back, so the operator knows what to retry.
		slog.Error("derived site rows failed after a successful seed", slog.Any("error", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "seeded " + strconv.Itoa(seeded) +
				" page rows, but rebuilding the derived chapter/appendix rows failed: " + err.Error(),
			"data": gin.H{"seeded": seeded},
		})
		return
	}

	slog.Info("site catalogue reseeded", "page_rows", seeded, "derived_rows", derived)

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"seeded":  seeded,
		"derived": derived,
		"before":  before,
		"after":   h.census(ctx),
	}})
}
