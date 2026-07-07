package notify

import (
	"context"
	"log/slog"
	"os"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

// androidChannelID must match the channel the mobile app creates client-side
// (apps/mobile/lib/notifications/channels.ts); Android 8+ drops notifications
// addressed to a channel that does not exist on the device.
const androidChannelID = "announcements"

// fcmBatchLimit is FCM's maximum tokens per multicast call.
const fcmBatchLimit = 500

// NewFCMSenderFromEnv builds the FCM sender from FIREBASE_SERVICE_ACCOUNT_JSON
// (the raw service-account JSON). When the variable is absent it returns an
// unconfigured no-op sender so the server still boots with FCM disabled, the
// same pattern as the VAPID web-push sender.
func NewFCMSenderFromEnv(ctx context.Context) FCMSender {
	raw := os.Getenv("FIREBASE_SERVICE_ACCOUNT_JSON")
	if raw == "" {
		slog.Warn("FIREBASE_SERVICE_ACCOUNT_JSON not set — FCM notifications disabled")
		return noopFCMSender{}
	}
	app, err := firebase.NewApp(ctx, nil, option.WithCredentialsJSON([]byte(raw)))
	if err != nil {
		slog.Error("FCM init failed — FCM notifications disabled", "error", err)
		return noopFCMSender{}
	}
	client, err := app.Messaging(ctx)
	if err != nil {
		slog.Error("FCM messaging client failed — FCM notifications disabled", "error", err)
		return noopFCMSender{}
	}
	slog.Info("FCM sender configured")
	return &firebaseFCMSender{client: client}
}

type firebaseFCMSender struct {
	client *messaging.Client
}

func (s *firebaseFCMSender) Configured() bool { return true }

// SendMulticast delivers msg to the tokens in batches of 500. Tokens FCM
// reports as unregistered or invalid come back in dead for pruning; transient
// per-token failures are logged and skipped.
func (s *firebaseFCMSender) SendMulticast(ctx context.Context, tokens []string, msg Message) (int, []string, error) {
	var sent int
	var dead []string

	for start := 0; start < len(tokens); start += fcmBatchLimit {
		end := start + fcmBatchLimit
		if end > len(tokens) {
			end = len(tokens)
		}
		batch := tokens[start:end]

		data := map[string]string{}
		if msg.URL != "" {
			// `route` is the key the Capacitor app reads for tap navigation;
			// `url` mirrors the web push payload for any web-side consumers.
			data["route"] = msg.URL
			data["url"] = msg.URL
		}
		if msg.Tag != "" {
			data["tag"] = msg.Tag
		}

		res, err := s.client.SendEachForMulticast(ctx, &messaging.MulticastMessage{
			Tokens:       batch,
			Notification: &messaging.Notification{Title: msg.Title, Body: msg.Body},
			Data:         data,
			Android: &messaging.AndroidConfig{
				Notification: &messaging.AndroidNotification{ChannelID: androidChannelID, Tag: msg.Tag},
			},
		})
		if err != nil {
			return sent, dead, err
		}

		for i, resp := range res.Responses {
			if resp.Success {
				sent++
				continue
			}
			if messaging.IsUnregistered(resp.Error) || messaging.IsInvalidArgument(resp.Error) {
				dead = append(dead, batch[i])
				continue
			}
			slog.Warn("fcm: send failed", "error", resp.Error)
		}
	}

	return sent, dead, nil
}

type noopFCMSender struct{}

func (noopFCMSender) Configured() bool { return false }

func (noopFCMSender) SendMulticast(context.Context, []string, Message) (int, []string, error) {
	return 0, nil, nil
}
