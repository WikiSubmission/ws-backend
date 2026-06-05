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

// ExportUploader stores a user's data-export payload and returns a stable
// object key plus a time-limited download URL. UploadUserExport overwrites any
// previous export for the same user.
type ExportUploader interface {
	UploadUserExport(ctx context.Context, userID int, payload []byte) (objectKey string, downloadURL string, err error)
}

type MeExportHandler struct {
	DB       *db.DB
	Uploader ExportUploader
}

type NoopExportUploader struct{}

func (NoopExportUploader) UploadUserExport(context.Context, int, []byte) (string, string, error) {
	return "", "", fmt.Errorf("export uploader not configured")
}

func computeRetryAfterSeconds(row *db.UserDataExportRow, now time.Time) int64 {
	if row == nil {
		return 0
	}
	var eligibleAt time.Time
	switch row.Status {
	case "queued", "running":
		eligibleAt = row.RequestedAt.Add(15 * time.Minute)
	case "sent":
		if row.SentAt != nil {
			eligibleAt = row.SentAt.Add(24 * time.Hour)
		}
	}
	if eligibleAt.IsZero() || !eligibleAt.After(now) {
		return 60
	}
	return int64(eligibleAt.Sub(now).Seconds())
}

func (h MeExportHandler) PostExport(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	email := c.GetString(auth.ContextKeyEmail)
	if authID == "" || email == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "not authenticated"})
		return
	}
	if _, err := h.DB.UpsertUser(c.Request.Context(), authID, email, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	user, err := h.DB.FetchUserByAuthID(c.Request.Context(), authID)
	if err != nil || user == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}

	now := time.Now().UTC()
	blocker, err := h.DB.FindExportBlocker(c.Request.Context(), user.ID, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	if blocker != nil {
		c.JSON(http.StatusTooManyRequests, gin.H{"retry_after_seconds": computeRetryAfterSeconds(blocker, now)})
		return
	}

	row, err := h.DB.CreateQueuedUserDataExport(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}

	go h.runExport(row.ID, user.ID, user.AuthID, user.Email)
	c.JSON(http.StatusAccepted, gin.H{"status": "queued", "estimated_minutes": 15})
}

func (h MeExportHandler) GetStatus(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	email := c.GetString(auth.ContextKeyEmail)
	if authID == "" || email == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "not authenticated"})
		return
	}
	if _, err := h.DB.UpsertUser(c.Request.Context(), authID, email, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	user, err := h.DB.FetchUserByAuthID(c.Request.Context(), authID)
	if err != nil || user == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	row, err := h.DB.LatestUserDataExport(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": row})
}

func (h MeExportHandler) runExport(exportID int64, userID int, authID, email string) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("me-export panic", "export_id", exportID, "panic", rec)
			_ = h.DB.SetUserDataExportFailed(context.Background(), exportID, fmt.Sprintf("panic: %v", rec))
		}
	}()

	ctx := context.Background()
	_ = h.DB.SetUserDataExportRunning(ctx, exportID)

	payload := map[string]any{
		"profile":        map[string]any{"user_id": userID, "auth_id": authID, "email": email},
		"bookmarks":      map[string]any{},
		"notes":          map[string]any{},
		"cover_to_cover": map[string]any{},
		"activity":       map[string]any{},
		"game_attempts":  map[string]any{},
		"consent":        map[string]any{},
	}
	raw, _ := json.Marshal(payload)

	objectKey, url, err := h.Uploader.UploadUserExport(ctx, userID, raw)
	if err != nil {
		errMsg := err.Error()
		slog.Error("me-export upload failed", "export_id", exportID, "error", errMsg)
		_ = h.DB.SetUserDataExportFailed(ctx, exportID, errMsg)
		return
	}

	// The signed download URL is persisted on the row and surfaced to the user
	// through GET /export/status, which the settings page polls and renders as a
	// "Download your data" link. No email is sent.
	slog.Info("me-export ready", "export_id", exportID, "object_key", objectKey)
	_ = h.DB.SetUserDataExportSent(ctx, exportID, objectKey, url)
}
