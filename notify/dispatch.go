package notify

import (
	"context"
	"log/slog"
	"sync"

	"github.com/wikiSubmission/ws-backend/db"
)

// webPushWorkers bounds concurrent web-push HTTP requests during a fan-out.
const webPushWorkers = 16

// Dispatcher fans a scheduled notification out to its destination channels.
// Either sender may be unconfigured; the corresponding channel is skipped.
type Dispatcher struct {
	DB  *db.DB
	Web WebPushSender
	FCM FCMSender
}

// Dispatch sends one scheduled notification and returns per-channel delivery
// counts. err carries the first channel-level failure (per-recipient failures
// are logged and reflected in the counts instead).
func (d *Dispatcher) Dispatch(ctx context.Context, row db.ScheduledNotificationRow) (webSent, fcmSent int, err error) {
	msg := Message{Title: row.Title, Body: row.Body}
	if row.URL != nil {
		msg.URL = *row.URL
	}
	if row.Category != nil {
		msg.Tag = *row.Category
	}

	if row.Destination == "web_push" || row.Destination == "all" {
		webSent, err = d.dispatchWebPush(ctx, row.CategoryFilter, msg)
	}

	if row.Destination == "fcm" || row.Destination == "all" {
		fcmSent, err = d.dispatchFCM(ctx, row.CategoryFilter, msg, err)
	}

	return webSent, fcmSent, err
}

func (d *Dispatcher) dispatchWebPush(ctx context.Context, categoryFilter *string, msg Message) (int, error) {
	if d.Web == nil || !d.Web.Configured() {
		return 0, nil
	}

	var subs []db.PushSubscriptionRow
	var err error
	if categoryFilter != nil && *categoryFilter != "" {
		subs, err = d.DB.FetchPushSubscriptionsByCategory(ctx, *categoryFilter)
	} else {
		subs, err = d.DB.FetchAllPushSubscriptions(ctx)
	}
	if err != nil {
		return 0, err
	}

	var (
		mu   sync.Mutex
		sent int
		wg   sync.WaitGroup
		sem  = make(chan struct{}, webPushWorkers)
	)
	for _, sub := range subs {
		wg.Add(1)
		sem <- struct{}{}
		go func(sub db.PushSubscriptionRow) {
			defer wg.Done()
			defer func() { <-sem }()
			gone, sendErr := d.Web.Send(ctx, sub, msg)
			if gone {
				_ = d.DB.DeletePushSubscriptionByEndpoint(ctx, sub.Endpoint)
				return
			}
			if sendErr != nil {
				slog.Warn("notify: web push send failed", "error", sendErr)
				return
			}
			mu.Lock()
			sent++
			mu.Unlock()
		}(sub)
	}
	wg.Wait()

	return sent, nil
}

// dispatchFCM keeps a prior web-push error unless FCM itself fails, so the
// recorded error reflects the first channel-level failure.
func (d *Dispatcher) dispatchFCM(ctx context.Context, categoryFilter *string, msg Message, priorErr error) (int, error) {
	if d.FCM == nil || !d.FCM.Configured() {
		return 0, priorErr
	}

	var filter *string
	if categoryFilter != nil && *categoryFilter != "" {
		filter = categoryFilter
	}
	devices, err := d.DB.FetchFCMDevices(ctx, filter)
	if err != nil {
		return 0, err
	}
	if len(devices) == 0 {
		return 0, priorErr
	}

	tokens := make([]string, len(devices))
	for i, dev := range devices {
		tokens[i] = dev.Token
	}

	sent, dead, err := d.FCM.SendMulticast(ctx, tokens, msg)
	if len(dead) > 0 {
		if pruneErr := d.DB.DeleteFCMDevicesByTokens(ctx, dead); pruneErr != nil {
			slog.Warn("notify: pruning dead FCM tokens failed", "error", pruneErr)
		} else {
			slog.Info("notify: pruned dead FCM tokens", "count", len(dead))
		}
	}
	if err != nil {
		return sent, err
	}
	return sent, priorErr
}
