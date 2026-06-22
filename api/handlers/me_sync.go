package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
)

// MeSyncHandler serves POST /me/sync: a batch of offline user-data mutations
// replayed when the client reconnects. It lives outside the OpenAPI surface
// (registered before the validator) like the other custom /me handlers.
type MeSyncHandler struct {
	DB *db.DB
}

const maxSyncBatch = 500

type syncMutation struct {
	ID        string          `json:"id"`
	Entity    string          `json:"entity"`
	Op        string          `json:"op"`
	CreatedAt int64           `json:"created_at"`
	Payload   json.RawMessage `json:"payload"`
}

type syncRequest struct {
	Mutations []syncMutation `json:"mutations"`
}

type syncResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func (h MeSyncHandler) ensureUser(c *gin.Context, authID string) bool {
	user, err := h.DB.FetchUserByAuthID(c.Request.Context(), authID)
	if err != nil {
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
	if _, err := h.DB.UpsertUser(c.Request.Context(), authID, email, displayNameFromGin(c)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return false
	}
	return true
}

// Sync applies an ordered batch of mutations and returns a per-mutation result.
// Each mutation's UUID is an idempotency key: a re-sent mutation returns
// "duplicate" without re-applying. Per-mutation failures are isolated — one bad
// row does not fail the batch.
func (h MeSyncHandler) Sync(c *gin.Context) {
	authID, ok := authIDFromGin(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "not authenticated"})
		return
	}

	var req syncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}
	if len(req.Mutations) > maxSyncBatch {
		c.JSON(http.StatusBadRequest, gin.H{"message": "batch too large"})
		return
	}
	if !h.ensureUser(c, authID) {
		return
	}

	ctx := c.Request.Context()
	results := make([]syncResult, 0, len(req.Mutations))
	for _, m := range req.Mutations {
		results = append(results, h.process(ctx, authID, m))
	}

	c.JSON(http.StatusOK, gin.H{
		"results":     results,
		"server_time": time.Now().UnixMilli(),
	})
}

func (h MeSyncHandler) process(ctx context.Context, authID string, m syncMutation) syncResult {
	if m.ID == "" {
		return syncResult{ID: m.ID, Status: "error", Error: "missing id"}
	}

	if _, found, err := h.DB.CheckIdempotency(ctx, authID, m.ID); err != nil {
		return syncResult{ID: m.ID, Status: "error", Error: "idempotency check failed"}
	} else if found {
		return syncResult{ID: m.ID, Status: "duplicate"}
	}

	status, err := h.apply(ctx, authID, m)
	if err != nil {
		return syncResult{ID: m.ID, Status: "error", Error: err.Error()}
	}

	if err := h.DB.RecordIdempotency(ctx, authID, m.ID, status); err != nil {
		// The mutation applied; failing to record the key only risks a future
		// duplicate replay, which the operations themselves tolerate. Log and move on.
		slog.Warn("me_sync: record idempotency failed", "key", m.ID, "error", err)
	}
	return syncResult{ID: m.ID, Status: status}
}

func (h MeSyncHandler) apply(ctx context.Context, authID string, m syncMutation) (string, error) {
	switch {
	case m.Entity == "bookmark_entry" && m.Op == "create":
		var p struct {
			CategoryID int64  `json:"category_id"`
			Scripture  string `json:"scripture"`
			VerseKey   string `json:"verse_key"`
		}
		if err := json.Unmarshal(m.Payload, &p); err != nil {
			return "", fmt.Errorf("bad payload: %w", err)
		}
		if _, err := h.DB.CreateBookmarkEntry(ctx, p.CategoryID, authID, p.Scripture, p.VerseKey); err != nil {
			return "", err
		}
		return "applied", nil

	case m.Entity == "bookmark_entry" && m.Op == "delete":
		var p struct {
			CategoryID int64  `json:"category_id"`
			Scripture  string `json:"scripture"`
			VerseKey   string `json:"verse_key"`
		}
		if err := json.Unmarshal(m.Payload, &p); err != nil {
			return "", fmt.Errorf("bad payload: %w", err)
		}
		if _, err := h.DB.DeleteBookmarkEntryByVerse(ctx, authID, p.CategoryID, p.Scripture, p.VerseKey); err != nil {
			return "", err
		}
		return "applied", nil

	case m.Entity == "note" && m.Op == "upsert":
		var p struct {
			Scripture string   `json:"scripture"`
			VerseKey  string   `json:"verse_key"`
			Content   string   `json:"content"`
			Tags      []string `json:"tags"`
		}
		if err := json.Unmarshal(m.Payload, &p); err != nil {
			return "", fmt.Errorf("bad payload: %w", err)
		}
		applied, err := h.DB.UpsertNoteLWW(ctx, authID, p.Scripture, p.VerseKey, p.Content, p.Tags, m.CreatedAt)
		if err != nil {
			return "", err
		}
		if applied {
			return "applied", nil
		}
		return "superseded", nil

	case m.Entity == "note" && m.Op == "delete":
		var p struct {
			Scripture string `json:"scripture"`
			VerseKey  string `json:"verse_key"`
		}
		if err := json.Unmarshal(m.Payload, &p); err != nil {
			return "", fmt.Errorf("bad payload: %w", err)
		}
		if _, err := h.DB.DeleteNoteByVerse(ctx, authID, p.Scripture, p.VerseKey); err != nil {
			return "", err
		}
		return "applied", nil

	case m.Entity == "reading_progress" && m.Op == "upsert":
		var p struct {
			Scripture string `json:"scripture"`
			VerseKey  string `json:"verse_key"`
		}
		if err := json.Unmarshal(m.Payload, &p); err != nil {
			return "", fmt.Errorf("bad payload: %w", err)
		}
		if _, err := h.DB.UpsertReadingProgress(ctx, authID, p.Scripture, p.VerseKey); err != nil {
			return "", err
		}
		return "applied", nil

	default:
		return "", fmt.Errorf("unsupported mutation %s/%s", m.Entity, m.Op)
	}
}
