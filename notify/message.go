// Package notify implements the scheduled-notification pipeline: an in-process
// scheduler claims due rows from scheduled_notifications and a dispatcher fans
// them out over web push (VAPID) and FCM. It deliberately does not import
// api/handlers — the existing WebPushSender there is adapted to the interface
// below at wiring time in api/server.go.
package notify

import (
	"context"

	"github.com/wikiSubmission/ws-backend/db"
)

// Message is the channel-agnostic notification payload. Web push serializes it
// as the service-worker PushPayload; FCM maps it onto a notification message
// with url/tag data keys.
type Message struct {
	Title string
	Body  string
	URL   string
	Tag   string
}

// WebPushSender delivers a Message to one stored subscription. gone reports an
// endpoint the push service considers permanently invalid (404/410) so the
// caller can prune it.
type WebPushSender interface {
	Configured() bool
	Send(ctx context.Context, sub db.PushSubscriptionRow, msg Message) (gone bool, err error)
}

// FCMSender delivers a Message to a batch of device tokens. dead lists tokens
// FCM reported as unregistered or invalid so the caller can prune them.
type FCMSender interface {
	Configured() bool
	SendMulticast(ctx context.Context, tokens []string, msg Message) (sent int, dead []string, err error)
}
