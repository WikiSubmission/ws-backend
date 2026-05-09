package main

import (
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/sanity"
)

// registerSanityRoutes mounts the webhook receiver and image proxy on r.
// Both routes are intentionally outside the OpenAPI surface: the webhook is
// internal (HMAC-verified) and the image proxy serves binary passthrough that
// the spec-validator cannot describe.
//
// Must be called before middleware.OapiRequestValidatorWithOptions is added
// so the validator does not 404 these paths.
func registerSanityRoutes(r *gin.Engine, client *sanity.Client) {
	if client == nil {
		slog.Info("Sanity proxy disabled (SANITY_PROJECT_ID not set)")
		return
	}

	r.POST("/webhooks/sanity", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "could not read body"})
			return
		}
		payload, err := client.VerifyWebhook(c.GetHeader(sanity.SignatureHeader), body)
		if err != nil {
			slog.Warn("Sanity webhook rejected", "error", err)
			c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid signature"})
			return
		}
		// Always 200 on accepted signatures so Sanity does not retry.
		client.InvalidateByID(payload.ID)
		client.InvalidateByType(payload.Type)
		slog.Info("Sanity cache invalidated",
			"_id", payload.ID,
			"_type", payload.Type,
			"operation", payload.Operation,
		)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.GET(sanity.ImageProxyPrefix+"*subPath", func(c *gin.Context) {
		subPath := c.Param("subPath")
		body, contentType, contentLength, err := client.FetchImage(
			c.Request.Context(), subPath, c.Request.URL.RawQuery,
		)
		if err != nil {
			slog.Warn("Sanity image proxy failed", "subPath", subPath, "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"message": "image upstream unavailable"})
			return
		}
		defer func() { _ = body.Close() }()

		if contentType != "" {
			c.Header("Content-Type", contentType)
		}
		if contentLength > 0 {
			c.Header("Content-Length", strconv.FormatInt(contentLength, 10))
		}
		// Public images change rarely; let the CDN cache aggressively.
		c.Header("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
		c.Status(http.StatusOK)
		if _, err := io.Copy(c.Writer, body); err != nil {
			slog.Warn("Sanity image proxy stream copy failed", "error", err)
		}
	})
}
