package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/db"
)

// PushMessage is the notification payload delivered to the service worker.
// Mirrors the PushPayload interface in the frontend app/sw.ts.
type PushMessage struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url,omitempty"`
	Tag   string `json:"tag,omitempty"`
	Icon  string `json:"icon,omitempty"`
}

// PushSender delivers a PushMessage to a single subscription. `gone` is true
// when the push service reports the endpoint as permanently invalid (404/410)
// so the caller can prune it.
type PushSender interface {
	Send(ctx context.Context, sub db.PushSubscriptionRow, msg PushMessage) (gone bool, err error)
}

// WebPushSender signs payloads with VAPID and posts them to the push service.
// Keys come from env (never hardcoded):
//
//	VAPID_PUBLIC_KEY, VAPID_PRIVATE_KEY  - the application server key pair
//	VAPID_SUBJECT                        - a mailto: or https: contact URI
type WebPushSender struct {
	publicKey  string
	privateKey string
	subject    string
}

func NewWebPushSender() WebPushSender {
	subject := os.Getenv("VAPID_SUBJECT")
	if subject == "" {
		subject = "mailto:admin@wikisubmission.org"
	}
	return WebPushSender{
		publicKey:  os.Getenv("VAPID_PUBLIC_KEY"),
		privateKey: os.Getenv("VAPID_PRIVATE_KEY"),
		subject:    subject,
	}
}

// Configured reports whether VAPID keys are present. Sending is skipped (not an
// error) when unconfigured, so the subscribe/unsubscribe API stays usable in
// environments without keys.
func (s WebPushSender) Configured() bool {
	return s.publicKey != "" && s.privateKey != ""
}

func (s WebPushSender) Send(ctx context.Context, sub db.PushSubscriptionRow, msg PushMessage) (bool, error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return false, err
	}
	resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.AuthKey},
	}, &webpush.Options{
		Subscriber:      s.subject,
		VAPIDPublicKey:  s.publicKey,
		VAPIDPrivateKey: s.privateKey,
		TTL:             60,
	})
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	gone := resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone
	return gone, nil
}

// PushHandler exposes subscription management under /api/v1/me/push. Like the
// other me_* routes it lives outside the OpenAPI surface and relies on the
// RequireAuth middleware to populate the auth context.
type PushHandler struct {
	DB     *db.DB
	Sender WebPushSender
}

type pushKeys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

type subscribeRequest struct {
	Endpoint   string   `json:"endpoint"`
	Keys       pushKeys `json:"keys"`
	Categories []string `json:"categories"`
	Locale     *string  `json:"locale,omitempty"`
	Timezone   *string  `json:"timezone,omitempty"`
}

// Subscribe stores or refreshes the caller's push endpoint.
func (h PushHandler) Subscribe(c *gin.Context) {
	authID, ok := authIDFromGin(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "not authenticated"})
		return
	}

	var req subscribeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}

	// Validate at the boundary: a push endpoint must be an https URL and the
	// encryption keys must be present, else the subscription is unusable.
	if !strings.HasPrefix(req.Endpoint, "https://") || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "endpoint and keys are required"})
		return
	}

	var userAgent *string
	if ua := c.Request.UserAgent(); ua != "" {
		userAgent = &ua
	}

	row, err := h.DB.UpsertPushSubscription(
		c.Request.Context(),
		authID, req.Endpoint, req.Keys.P256dh, req.Keys.Auth,
		req.Categories, req.Locale, req.Timezone, userAgent,
	)
	if err != nil {
		slog.Error("push: UpsertPushSubscription", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": row.ID, "categories": row.Categories})
}

type unsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

// Unsubscribe removes one of the caller's push endpoints.
func (h PushHandler) Unsubscribe(c *gin.Context) {
	authID, ok := authIDFromGin(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "not authenticated"})
		return
	}

	var req unsubscribeRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Endpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "endpoint is required"})
		return
	}

	if _, err := h.DB.DeletePushSubscription(c.Request.Context(), authID, req.Endpoint); err != nil {
		slog.Error("push: DeletePushSubscription", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// SendTest delivers a test notification to all of the caller's endpoints, so a
// user can confirm delivery after enabling notifications. Prunes endpoints the
// push service reports as gone.
func (h PushHandler) SendTest(c *gin.Context) {
	authID, ok := authIDFromGin(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "not authenticated"})
		return
	}
	if !h.Sender.Configured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "push is not configured"})
		return
	}

	subs, err := h.DB.FetchPushSubscriptionsByAuthID(c.Request.Context(), authID)
	if err != nil {
		slog.Error("push: FetchPushSubscriptionsByAuthID", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}

	msg := PushMessage{
		Title: "WikiSubmission",
		Body:  "Notifications are enabled.",
		URL:   "/",
		Tag:   "push-test",
	}

	var sent int
	for _, sub := range subs {
		gone, sendErr := h.Sender.Send(c.Request.Context(), sub, msg)
		if gone {
			_ = h.DB.DeletePushSubscriptionByEndpoint(c.Request.Context(), sub.Endpoint)
			continue
		}
		if sendErr != nil {
			slog.Warn("push: send failed", "error", sendErr)
			continue
		}
		sent++
	}

	c.JSON(http.StatusOK, gin.H{"sent": sent})
}
