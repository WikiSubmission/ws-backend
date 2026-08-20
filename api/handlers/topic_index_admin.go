package handlers

// Admin-gated reseed for the Quran's printed topical index. Plain Gin handlers
// (not the OpenAPI strict server) registered under /api/v1/admin/topic-index and
// protected by auth.RequireAuth + auth.RequireAdmin, the same shape
// offline_admin.go uses.
//
// Why this exists rather than just `just seed-topic-index`: the seed JSON is
// embedded in the server binary via //go:embed in db/topic_index.go, and db/ is
// copied into the image — but api/cmd/seed_topic_index is not, because the
// Dockerfile builds only ./api/. So on a deployed environment the data is
// already present in the running process and there is no way to ask it to load.
// A CLI seed instead needs direct network reach to that environment's Postgres,
// which is exactly what a managed deploy tends not to grant.
//
// The reseed is a full replace per language inside one transaction (see
// db.SeedTopicIndex), so re-running it is the documented content-update path
// after a new seed file ships. It is synchronous: ~15k rows in one transaction
// takes well under the timeout, so this needs none of the background job
// machinery an offline bundle rebuild does.

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/db"
)

// reseedTimeout bounds the seed transaction. Generous relative to the real cost
// so a slow link to Postgres does not abort a half-finished replace — the
// transaction would roll back cleanly, but the operator would be left guessing.
const reseedTimeout = 2 * time.Minute

// TopicIndexAdminHandler serves the admin topical-index routes.
//
// It deliberately does not hold the public Server, and so does not refresh its
// TopicIndexLangs cache after a reseed. The cache only ever *rejects* an unknown
// lang: an empty one is permissive by design (see resolveTopicIndexLang), which
// is exactly the state a first reseed starts from, so seeding into an empty
// database needs no restart. A second edition would need one before its code is
// accepted, and shipping a second edition means a deploy regardless.
type TopicIndexAdminHandler struct {
	DB *db.DB
}

func NewTopicIndexAdminHandler(database *db.DB) *TopicIndexAdminHandler {
	return &TopicIndexAdminHandler{DB: database}
}

// Status reports the current row census, so an operator can tell an empty
// database from a seeded one before and after reseeding.
// GET /api/v1/admin/topic-index
func (h *TopicIndexAdminHandler) Status(c *gin.Context) {
	counts, err := h.DB.CountTopicIndex(c.Request.Context(), "")
	if err != nil {
		slog.Error("topic index census failed", slog.Any("error", err))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "could not count the topical index"})
		return
	}

	langs, err := h.DB.FetchTopicIndexLangs(c.Request.Context())
	if err != nil {
		slog.Error("topic index langs failed", slog.Any("error", err))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "could not list the index editions"})
		return
	}
	editions := make([]string, 0, len(langs))
	for lang := range langs {
		editions = append(editions, lang)
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"entries":    counts.Entries,
		"subentries": counts.Subentries,
		"refs":       counts.Refs,
		"editions":   editions,
	}})
}

// Reseed loads the embedded seed for every edition, replacing what is there.
// POST /api/v1/admin/topic-index/reseed
func (h *TopicIndexAdminHandler) Reseed(c *gin.Context) {
	// Detached from the request context on purpose: an operator closing the tab
	// must not abort the transaction partway and leave the index empty.
	ctx, cancel := context.WithTimeout(context.Background(), reseedTimeout)
	defer cancel()

	before, err := h.DB.CountTopicIndex(ctx, "")
	if err != nil {
		slog.Warn("topic index census before reseed failed", slog.Any("error", err))
	}

	seeded, err := h.DB.SeedTopicIndex(ctx)
	if err != nil {
		slog.Error("topic index reseed failed", slog.Any("error", err))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "reseed failed: " + err.Error()})
		return
	}

	after, err := h.DB.CountTopicIndex(ctx, "")
	if err != nil {
		slog.Warn("topic index census after reseed failed", slog.Any("error", err))
	}

	slog.Info("topic index reseeded",
		"entries_seeded", seeded,
		"entries_before", before.Entries,
		"entries_after", after.Entries,
	)

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"seeded": seeded,
		"before": gin.H{"entries": before.Entries, "subentries": before.Subentries, "refs": before.Refs},
		"after":  gin.H{"entries": after.Entries, "subentries": after.Subentries, "refs": after.Refs},
	}})
}
