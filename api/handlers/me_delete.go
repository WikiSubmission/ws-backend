package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
)

// exportFreshness is how recent a completed export must be for a deletion to be
// allowed. It matches the export download-link validity, so a user always has a
// usable copy of their data before erasing it.
const exportFreshness = 30 * 24 * time.Hour

type MeDeleteHandler struct {
	DB *db.DB
}

type deleteContentRequest struct {
	Categories []string `json:"categories"`
}

// exportGateSatisfied reports whether a completed, still-fresh export exists, so
// the user has a usable copy of their data before any irreversible deletion.
func exportGateSatisfied(latest *db.UserDataExportRow, now time.Time) bool {
	if latest == nil || latest.Status != "sent" || latest.SentAt == nil || latest.DownloadURL == nil {
		return false
	}
	return now.Sub(*latest.SentAt) <= exportFreshness
}

func (h MeDeleteHandler) resolveUser(c *gin.Context) (*db.UserRow, bool) {
	authID := c.GetString(auth.ContextKeyUID)
	email := c.GetString(auth.ContextKeyEmail)
	if authID == "" || email == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "not authenticated"})
		return nil, false
	}
	if _, err := h.DB.UpsertUser(c.Request.Context(), authID, email, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return nil, false
	}
	user, err := h.DB.FetchUserByAuthID(c.Request.Context(), authID)
	if err != nil || user == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return nil, false
	}
	return user, true
}

func (h MeDeleteHandler) PostDelete(c *gin.Context) {
	user, ok := h.resolveUser(c)
	if !ok {
		return
	}

	var req deleteContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}

	// Validate: non-empty, no unknown or duplicate categories.
	if len(req.Categories) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "no categories selected"})
		return
	}
	seen := make(map[string]bool, len(req.Categories))
	for _, cat := range req.Categories {
		if !db.IsValidDeletionCategory(cat) {
			c.JSON(http.StatusBadRequest, gin.H{"message": fmt.Sprintf("unknown category: %s", cat)})
			return
		}
		if seen[cat] {
			c.JSON(http.StatusBadRequest, gin.H{"message": fmt.Sprintf("duplicate category: %s", cat)})
			return
		}
		seen[cat] = true
	}

	ctx := c.Request.Context()

	// Export gate: the user must have a recent, completed export so a copy of
	// their data exists before any irreversible deletion. Enforced here (not
	// just in the UI) so it cannot be bypassed.
	latestExport, err := h.DB.LatestUserDataExport(ctx, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	if !exportGateSatisfied(latestExport, time.Now().UTC()) {
		c.JSON(http.StatusConflict, gin.H{"reason": "export_required"})
		return
	}

	// One deletion at a time per user.
	blocker, err := h.DB.FindDeletionBlocker(ctx, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	if blocker != nil {
		c.JSON(http.StatusTooManyRequests, gin.H{"reason": "deletion_in_progress"})
		return
	}

	row, err := h.DB.CreateQueuedDeletion(ctx, user.ID, req.Categories)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}

	go h.runDeletion(row.ID, user.AuthID, req.Categories)
	c.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}

func (h MeDeleteHandler) GetDeleteStatus(c *gin.Context) {
	user, ok := h.resolveUser(c)
	if !ok {
		return
	}
	row, err := h.DB.LatestDeletion(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": row})
}

func (h MeDeleteHandler) runDeletion(deletionID int64, authID string, categories []string) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("me-delete panic", "deletion_id", deletionID, "panic", rec)
			if err := h.DB.SetDeletionFailed(context.Background(), deletionID, fmt.Sprintf("panic: %v", rec)); err != nil {
				slog.Error("me-delete set failed after panic failed", "deletion_id", deletionID, "error", err)
			}
		}
	}()

	ctx := context.Background()
	if err := h.DB.SetDeletionRunning(ctx, deletionID); err != nil {
		// Not fatal, but log it: a row stuck in 'queued' blocks the user's
		// future deletions via FindDeletionBlocker.
		slog.Error("me-delete set running failed", "deletion_id", deletionID, "error", err)
	}

	counts, err := h.DB.PurgeUserCategories(ctx, authID, categories)
	if err != nil {
		slog.Error("me-delete purge failed", "deletion_id", deletionID, "error", err)
		if ferr := h.DB.SetDeletionFailed(ctx, deletionID, err.Error()); ferr != nil {
			slog.Error("me-delete set failed status failed", "deletion_id", deletionID, "error", ferr)
		}
		return
	}

	slog.Info("me-delete complete", "deletion_id", deletionID, "categories", categories, "counts", counts)
	if err := h.DB.SetDeletionDone(ctx, deletionID, counts); err != nil {
		// The data is gone but the row stays 'running', which would block
		// further deletions. Surface it so the stuck row can be cleared.
		slog.Error("me-delete set done failed", "deletion_id", deletionID, "error", err)
	}
}
