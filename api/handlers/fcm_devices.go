package handlers

// FCM device registration for the Capacitor mobile app. Registration is
// anonymous-friendly: the routes run behind auth.OptionalAuth, so auth_id is
// attached when the caller presents a valid bearer token and left NULL
// otherwise. Outside the OpenAPI surface (registered before the validator).

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/db"
)

// Anonymous write endpoint: bound the input sizes and rate-limit per IP on top
// of the global limiter.
const (
	fcmMaxTokenLen    = 4096
	fcmMaxCategories  = 20
	fcmMaxCategoryLen = 64
	fcmMaxLocaleLen   = 16
	fcmMaxVersionLen  = 32
)

var fcmPlatforms = map[string]struct{}{"ios": {}, "android": {}, "web": {}}

type FCMDeviceHandler struct {
	DB      *db.DB
	limiter *keyedLimiter // per-IP fixed window on the write endpoints
}

func NewFCMDeviceHandler(database *db.DB) FCMDeviceHandler {
	return FCMDeviceHandler{
		DB:      database,
		limiter: newKeyedLimiter(10, time.Minute),
	}
}

type fcmRegisterRequest struct {
	Token      string   `json:"token"`
	Platform   string   `json:"platform"`
	Locale     *string  `json:"locale,omitempty"`
	Timezone   *string  `json:"timezone,omitempty"`
	Categories []string `json:"categories"`
	AppVersion *string  `json:"app_version,omitempty"`
}

// Register stores or refreshes a device token.
func (h FCMDeviceHandler) Register(c *gin.Context) {
	if !h.limiter.Allow(c.ClientIP()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"message": "rate limit exceeded — please slow down"})
		return
	}

	var req fcmRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}
	if req.Token == "" || len(req.Token) > fcmMaxTokenLen {
		c.JSON(http.StatusBadRequest, gin.H{"message": "token is required"})
		return
	}
	if _, ok := fcmPlatforms[req.Platform]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "platform must be one of ios, android, web"})
		return
	}
	if len(req.Categories) > fcmMaxCategories {
		c.JSON(http.StatusBadRequest, gin.H{"message": "too many categories"})
		return
	}
	for _, cat := range req.Categories {
		if cat == "" || len(cat) > fcmMaxCategoryLen {
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid category"})
			return
		}
	}
	if req.Locale != nil && len(*req.Locale) > fcmMaxLocaleLen {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid locale"})
		return
	}
	if req.Timezone != nil && *req.Timezone != "" {
		if _, err := time.LoadLocation(*req.Timezone); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid timezone"})
			return
		}
	}
	if req.AppVersion != nil && len(*req.AppVersion) > fcmMaxVersionLen {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid app_version"})
		return
	}

	var authID *string
	if id, ok := authIDFromGin(c); ok && id != "" {
		authID = &id
	}

	row, err := h.DB.UpsertFCMDevice(
		c.Request.Context(),
		req.Token, authID, req.Platform,
		req.Locale, req.Timezone, req.Categories, req.AppVersion,
	)
	if err != nil {
		slog.Error("fcm: UpsertFCMDevice", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"categories": row.Categories, "registered": true}})
}

type fcmUnregisterRequest struct {
	Token string `json:"token"`
}

// Unregister removes a device token (app-side notification opt-out).
func (h FCMDeviceHandler) Unregister(c *gin.Context) {
	if !h.limiter.Allow(c.ClientIP()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"message": "rate limit exceeded — please slow down"})
		return
	}

	var req fcmUnregisterRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" || len(req.Token) > fcmMaxTokenLen {
		c.JSON(http.StatusBadRequest, gin.H{"message": "token is required"})
		return
	}

	if _, err := h.DB.DeleteFCMDevice(c.Request.Context(), req.Token); err != nil {
		slog.Error("fcm: DeleteFCMDevice", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"ok": true}})
}
