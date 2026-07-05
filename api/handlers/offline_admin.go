package handlers

// Admin-gated handlers for rebuilding and publishing the offline content
// bundles. Plain Gin handlers (not the OpenAPI strict server) registered under
// /api/v1/admin/offline and protected by auth.RequireAuth + auth.RequireAdmin.
//
// A rebuild runs the same pipeline as cmd/build-offline-bundles (see
// offline/builder): render every bundle from Postgres into a temp dir, then
// publish bundles first and the manifest last via ws-lib. The data_version is
// bumped automatically from the currently published manifest, so admins never
// manage version numbers. One rebuild may run at a time; progress is polled
// via GET on the same path.

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/db"
	"github.com/wikiSubmission/ws-backend/offline/builder"
	"github.com/wikiSubmission/ws-backend/offline/bundle"
	"github.com/wikiSubmission/ws-backend/offline/publish"
)

// Bundle set every rebuild produces. Kept server-side (not request input) so
// the button cannot publish a partial manifest: the manifest is written as a
// whole, so omitting a language would drop its bundles from the live manifest.
// Extend these lists as more translations gain offline support.
var (
	offlineTextLangs = []string{"ar", "en"}
	offlineWordLangs = []string{"en"}
)

// rebuildTimeout bounds a stuck rebuild (DB stall, CDN outage) so the single
// job slot is always reclaimed.
const rebuildTimeout = 15 * time.Minute

// OfflineRebuildStatus is the JSON shape reported to the admin UI.
type OfflineRebuildStatus struct {
	Status      string                 `json:"status"` // idle | running | done | failed
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	FinishedAt  *time.Time             `json:"finished_at,omitempty"`
	DataVersion int                    `json:"data_version,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Bundles     []bundle.ManifestEntry `json:"bundles,omitempty"`
}

// OfflineAdminHandler serves the admin offline-bundle routes. Store is nil
// when WSLIB_BASE_URL / STORE_AUTH_SECRET are absent; rebuilds are then
// rejected with 503 instead of silently building without publishing.
type OfflineAdminHandler struct {
	DB      *db.DB
	Store   *publish.WSLibPublicStore
	BaseURL string // public bundle base URL, e.g. https://cdn.wikisubmission.org/offline

	mu    sync.Mutex
	state OfflineRebuildStatus
}

// NewOfflineAdminHandler wires the handler; state starts idle.
func NewOfflineAdminHandler(database *db.DB, store *publish.WSLibPublicStore, baseURL string) *OfflineAdminHandler {
	return &OfflineAdminHandler{
		DB:      database,
		Store:   store,
		BaseURL: baseURL,
		state:   OfflineRebuildStatus{Status: "idle"},
	}
}

// RebuildStatus reports the current (or last) rebuild.
// GET /api/v1/admin/offline/rebuild
func (h *OfflineAdminHandler) RebuildStatus(c *gin.Context) {
	h.mu.Lock()
	state := h.state
	h.mu.Unlock()
	c.JSON(http.StatusOK, gin.H{"data": state})
}

// Rebuild starts an asynchronous build+publish of every offline bundle.
// POST /api/v1/admin/offline/rebuild
func (h *OfflineAdminHandler) Rebuild(c *gin.Context) {
	if h.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"message": "bundle publishing is not configured on this server (WSLIB_BASE_URL / STORE_AUTH_SECRET missing)",
		})
		return
	}

	// Resolve the next data_version before claiming the job slot, so a CDN
	// hiccup surfaces as a request error rather than a failed background job.
	version, err := builder.NextDataVersion(
		c.Request.Context(),
		&http.Client{Timeout: 15 * time.Second},
		h.BaseURL+"/manifest.json",
	)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "could not read the published manifest: " + err.Error()})
		return
	}

	h.mu.Lock()
	if h.state.Status == "running" {
		state := h.state
		h.mu.Unlock()
		c.JSON(http.StatusConflict, gin.H{"message": "a rebuild is already running", "data": state})
		return
	}
	now := time.Now().UTC()
	h.state = OfflineRebuildStatus{Status: "running", StartedAt: &now, DataVersion: version}
	h.mu.Unlock()

	go h.runRebuild(version)

	h.mu.Lock()
	state := h.state
	h.mu.Unlock()
	c.JSON(http.StatusAccepted, gin.H{"data": state})
}

// runRebuild executes the build in the background and records the outcome.
// Detached from the request context on purpose: closing the admin tab must not
// abort a half-published rebuild.
func (h *OfflineAdminHandler) runRebuild(version int) {
	ctx, cancel := context.WithTimeout(context.Background(), rebuildTimeout)
	defer cancel()

	finish := func(errMsg string, bundles []bundle.ManifestEntry) {
		now := time.Now().UTC()
		h.mu.Lock()
		h.state.FinishedAt = &now
		if errMsg != "" {
			h.state.Status = "failed"
			h.state.Error = errMsg
		} else {
			h.state.Status = "done"
			h.state.Bundles = bundles
		}
		h.mu.Unlock()
	}

	outDir, err := os.MkdirTemp("", "offline-bundles-*")
	if err != nil {
		finish("create temp dir: "+err.Error(), nil)
		return
	}
	defer os.RemoveAll(outDir)

	manifest, err := builder.Run(ctx, h.DB, builder.Options{
		Langs:       offlineTextLangs,
		WordLangs:   offlineWordLangs,
		OutDir:      outDir,
		BaseURL:     h.BaseURL,
		DataVersion: version,
		Store:       h.Store,
	})
	if err != nil {
		slog.Error("offline bundle rebuild failed", "data_version", version, "error", err)
		finish(err.Error(), nil)
		return
	}

	slog.Info("offline bundle rebuild published", "data_version", version, "bundles", len(manifest.Bundles))
	finish("", manifest.Bundles)
}
