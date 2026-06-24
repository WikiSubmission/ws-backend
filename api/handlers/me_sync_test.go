package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
)

// One migrated pool shared across every handler integration test in this
// package, mirroring db.setupTestDB (which is unexported in package db).
var (
	syncDBOnce sync.Once
	syncDB     *db.DB
	syncDBErr  error
)

func setupSyncDB(t *testing.T) *db.DB {
	t.Helper()
	_ = godotenv.Load()
	if os.Getenv("DATABASE_NAME") == "" {
		t.Skip("Skipping integration test: no DATABASE_NAME env var set")
	}
	syncDBOnce.Do(func() { syncDB, syncDBErr = db.DbSetup() })
	if syncDBErr != nil {
		t.Fatalf("DbSetup failed: %v", syncDBErr)
	}
	return syncDB
}

// seedSyncUser creates an isolated user + bookmark category for one test.
func seedSyncUser(t *testing.T, database *db.DB) (string, int64) {
	t.Helper()
	ctx := context.Background()
	authID := "test-mesync-" + time.Now().Format("150405.000000000")
	if _, err := database.UpsertUser(ctx, authID, authID+"@example.com", nil); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Pool.Exec(ctx, `DELETE FROM users WHERE auth_id = $1`, authID)
	})
	cat, err := database.CreateBookmarkCategory(ctx, authID, "Test", "amber")
	if err != nil {
		t.Fatalf("CreateBookmarkCategory: %v", err)
	}
	return authID, cat.ID
}

// syncRouter wires the handler behind a middleware that injects the auth
// context the real RequireAuth middleware would set, so the test exercises the
// full HTTP path (bind, validate, apply) without standing up real auth.
func syncRouter(database *db.DB, authID, email string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/me/sync", func(c *gin.Context) {
		if authID != "" {
			c.Set(auth.ContextKeyUID, authID)
		}
		if email != "" {
			c.Set(auth.ContextKeyEmail, email)
		}
		MeSyncHandler{DB: database}.Sync(c)
	})
	return r
}

func postSync(t *testing.T, r *gin.Engine, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/me/sync", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodeResults(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var resp struct {
		Results []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  string `json:"error"`
		} `json:"results"`
		ServerTime int64 `json:"server_time"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
	}
	if resp.ServerTime == 0 {
		t.Error("expected non-zero server_time")
	}
	byID := make(map[string]string, len(resp.Results))
	for _, r := range resp.Results {
		byID[r.ID] = r.Status
	}
	return byID
}

func mutation(id, entity, op string, payload map[string]any) map[string]any {
	return map[string]any{
		"id":         id,
		"entity":     entity,
		"op":         op,
		"created_at": time.Now().UnixMilli(),
		"payload":    payload,
	}
}

func TestMeSyncMixedBatch(t *testing.T) {
	database := setupSyncDB(t)
	authID, catID := seedSyncUser(t, database)
	r := syncRouter(database, authID, authID+"@example.com")

	body := map[string]any{"mutations": []map[string]any{
		mutation("m-bm", "bookmark_entry", "create", map[string]any{
			"category_id": catID, "scripture": "quran", "verse_key": "1:1",
		}),
		mutation("m-note", "note", "upsert", map[string]any{
			"scripture": "quran", "verse_key": "2:255", "content": "Ayat al-Kursi",
		}),
		mutation("m-rp", "reading_progress", "upsert", map[string]any{
			"scripture": "quran", "verse_key": "18:10",
		}),
	}}

	rec := postSync(t, r, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	got := decodeResults(t, rec)
	for _, id := range []string{"m-bm", "m-note", "m-rp"} {
		if got[id] != "applied" {
			t.Errorf("mutation %s: expected applied, got %q", id, got[id])
		}
	}

	// Replaying the same batch returns duplicate for every id (idempotency keys).
	rec = postSync(t, r, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("replay: expected 200, got %d", rec.Code)
	}
	got = decodeResults(t, rec)
	for _, id := range []string{"m-bm", "m-note", "m-rp"} {
		if got[id] != "duplicate" {
			t.Errorf("replay %s: expected duplicate, got %q", id, got[id])
		}
	}
}

func TestMeSyncBatchTooLarge(t *testing.T) {
	database := setupSyncDB(t)
	authID, _ := seedSyncUser(t, database)
	r := syncRouter(database, authID, authID+"@example.com")

	muts := make([]map[string]any, 0, maxSyncBatch+1)
	for i := 0; i <= maxSyncBatch; i++ {
		muts = append(muts, mutation(fmt.Sprintf("m-%d", i), "reading_progress", "upsert",
			map[string]any{"scripture": "quran", "verse_key": "1:1"}))
	}
	rec := postSync(t, r, map[string]any{"mutations": muts})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized batch, got %d", rec.Code)
	}
}

func TestMeSyncPerMutationErrorsAreIsolated(t *testing.T) {
	database := setupSyncDB(t)
	authID, _ := seedSyncUser(t, database)
	r := syncRouter(database, authID, authID+"@example.com")

	body := map[string]any{"mutations": []map[string]any{
		// Valid alongside the bad rows: must still apply.
		mutation("ok", "reading_progress", "upsert", map[string]any{
			"scripture": "quran", "verse_key": "1:1",
		}),
		// Unknown entity/op -> error, not a panic, not a batch failure.
		mutation("bad-entity", "collection", "upsert", map[string]any{}),
		// Missing id -> error.
		mutation("", "reading_progress", "upsert", map[string]any{
			"scripture": "quran", "verse_key": "1:2",
		}),
	}}

	rec := postSync(t, r, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	got := decodeResults(t, rec)
	if got["ok"] != "applied" {
		t.Errorf("valid mutation should apply despite sibling errors, got %q", got["ok"])
	}
	if got["bad-entity"] != "error" {
		t.Errorf("unknown entity should error, got %q", got["bad-entity"])
	}
	if got[""] != "error" {
		t.Errorf("missing id should error, got %q", got[""])
	}
}

func TestMeSyncRejectsUnauthenticated(t *testing.T) {
	database := setupSyncDB(t)
	r := syncRouter(database, "", "") // no auth context set

	rec := postSync(t, r, map[string]any{"mutations": []map[string]any{}})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", rec.Code)
	}
}

func TestMeSyncRejectsMalformedBody(t *testing.T) {
	database := setupSyncDB(t)
	authID, _ := seedSyncUser(t, database)
	r := syncRouter(database, authID, authID+"@example.com")

	req := httptest.NewRequest(http.MethodPost, "/me/sync", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed body, got %d", rec.Code)
	}
}
