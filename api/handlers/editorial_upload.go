package handlers

// Authenticated image upload for the first-party content editor. Editors POST a
// multipart image; the file is validated and forwarded to ws-lib's public store
// under the editorial/ prefix, which returns a stable CloudFront URL. Like the
// offline publish path, all S3/CloudFront access stays in ws-lib — ws-backend
// holds only the ws-lib base URL and the shared HMAC secret.
//
// Lives outside the OpenAPI surface (registered before the validator) and is
// gated by RequireAuth plus an in-process editorial permission check.

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/db"
	"github.com/wikiSubmission/ws-backend/offline/publish"
)

const (
	// maxEditorialImageBytes bounds a single uploaded image.
	maxEditorialImageBytes = 12 << 20 // 12 MiB
	// editorialImageCacheControl — keys are content-unique, so cache forever.
	editorialImageCacheControl = "public, max-age=31536000, immutable"
)

// allowedImageExt maps accepted MIME types to a file extension.
var allowedImageExt = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
	"image/gif":  "gif",
	"image/avif": "avif",
}

// EditorialUploadHandler stores editor image uploads via ws-lib's public store.
type EditorialUploadHandler struct {
	DB    *db.DB
	Store *publish.WSLibPublicStore
}

// Upload accepts a multipart "file" image and returns { "url": ... }.
func (h EditorialUploadHandler) Upload(c *gin.Context) {
	if h.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "uploads not configured"})
		return
	}

	if !h.authorizeEditor(c) {
		return // authorizeEditor already wrote the response
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "a multipart 'file' field is required"})
		return
	}
	if fileHeader.Size > maxEditorialImageBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"message": "image exceeds the 12 MiB limit"})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		slog.Error("editorial upload: open multipart file", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	defer file.Close()

	payload, err := io.ReadAll(io.LimitReader(file, maxEditorialImageBytes+1))
	if err != nil {
		slog.Error("editorial upload: read file", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	if len(payload) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "empty file"})
		return
	}
	if len(payload) > maxEditorialImageBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"message": "image exceeds the 12 MiB limit"})
		return
	}

	// Trust the sniffed content type over the client-declared one.
	contentType := http.DetectContentType(payload)
	ext, ok := allowedImageExt[contentType]
	if !ok {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"message": "unsupported image type; allowed: jpeg, png, webp, gif, avif"})
		return
	}

	key, err := editorialImageKey(ext)
	if err != nil {
		slog.Error("editorial upload: key generation", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}

	url, err := h.Store.Upload(c.Request.Context(), key, contentType, editorialImageCacheControl, payload)
	if err != nil {
		slog.Error("editorial upload: store", "error", err, "key", key)
		c.JSON(http.StatusBadGateway, gin.H{"message": "failed to store image"})
		return
	}

	slog.Info("editorial upload: image stored", "key", key, "bytes", len(payload), "content_type", contentType)
	c.JSON(http.StatusOK, gin.H{"url": url, "content_type": contentType})
}

// authorizeEditor denies callers without article write access (or admin),
// writing the error response itself. Returns true when the caller may upload.
func (h EditorialUploadHandler) authorizeEditor(c *gin.Context) bool {
	authID, ok := authIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "authentication required"})
		return false
	}
	ctx := c.Request.Context()
	user, err := h.DB.FetchUserByAuthID(ctx, authID)
	if err != nil {
		slog.Error("editorial upload: fetch user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return false
	}
	if user == nil || !user.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"message": "editorial access required"})
		return false
	}
	perms, err := h.DB.FetchEditorPermissions(ctx, user.ID, user.Role == "admin")
	if err != nil {
		slog.Error("editorial upload: fetch permissions", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return false
	}
	if perms == nil || !perms.CanWriteModule(db.ModuleArticle) {
		c.JSON(http.StatusForbidden, gin.H{"message": "article editing access required"})
		return false
	}
	return true
}

// editorialImageKey builds a collision-safe, publicly-served key partitioned by
// month, e.g. editorial/images/2026/07/9f3a…c1.webp.
func editorialImageKey(ext string) (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	return fmt.Sprintf("editorial/images/%04d/%02d/%s.%s", now.Year(), int(now.Month()), hex.EncodeToString(buf[:]), ext), nil
}
