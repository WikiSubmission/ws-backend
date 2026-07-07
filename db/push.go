package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type PushSubscriptionRow struct {
	ID         int64     `db:"id"`
	AuthID     string    `db:"auth_id"`
	Endpoint   string    `db:"endpoint"`
	P256dh     string    `db:"p256dh"`
	AuthKey    string    `db:"auth_key"`
	Categories []string  `db:"categories"`
	Locale     *string   `db:"locale"`
	Timezone   *string   `db:"timezone"`
	UserAgent  *string   `db:"user_agent"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

const pushCols = `id, auth_id, endpoint, p256dh, auth_key, categories, locale, timezone, user_agent, created_at, updated_at`

// UpsertPushSubscription stores (or refreshes) a browser push endpoint. The
// endpoint is unique, so re-subscribing the same browser updates the keys,
// owner, and preferences in place rather than duplicating.
func (db *DB) UpsertPushSubscription(
	ctx context.Context,
	authID, endpoint, p256dh, authKey string,
	categories []string,
	locale, timezone, userAgent *string,
) (*PushSubscriptionRow, error) {
	if categories == nil {
		categories = []string{}
	}
	query := `INSERT INTO push_subscriptions
	            (auth_id, endpoint, p256dh, auth_key, categories, locale, timezone, user_agent)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	          ON CONFLICT (endpoint) DO UPDATE SET
	            auth_id = EXCLUDED.auth_id,
	            p256dh = EXCLUDED.p256dh,
	            auth_key = EXCLUDED.auth_key,
	            categories = EXCLUDED.categories,
	            locale = EXCLUDED.locale,
	            timezone = EXCLUDED.timezone,
	            user_agent = EXCLUDED.user_agent,
	            updated_at = NOW()
	          RETURNING ` + pushCols
	rows, err := db.Pool.Query(ctx, query, authID, endpoint, p256dh, authKey, categories, locale, timezone, userAgent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[PushSubscriptionRow])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// DeletePushSubscription removes a single endpoint owned by authID. Returns
// false when nothing matched (already gone or not owned by the caller).
func (db *DB) DeletePushSubscription(ctx context.Context, authID, endpoint string) (bool, error) {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM push_subscriptions WHERE auth_id = $1 AND endpoint = $2`,
		authID, endpoint)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// DeletePushSubscriptionByEndpoint removes an endpoint regardless of owner.
// Used by the sender to prune endpoints the push service reports as gone
// (HTTP 404/410).
func (db *DB) DeletePushSubscriptionByEndpoint(ctx context.Context, endpoint string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM push_subscriptions WHERE endpoint = $1`, endpoint)
	return err
}

// FetchPushSubscriptionsByAuthID returns every endpoint for a user (a user may
// install on multiple devices).
func (db *DB) FetchPushSubscriptionsByAuthID(ctx context.Context, authID string) ([]PushSubscriptionRow, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+pushCols+` FROM push_subscriptions WHERE auth_id = $1`, authID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[PushSubscriptionRow])
}

// FetchAllPushSubscriptions returns every stored endpoint. Used by the
// notification dispatcher for schedules addressed to all web-push subscribers.
func (db *DB) FetchAllPushSubscriptions(ctx context.Context) ([]PushSubscriptionRow, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+pushCols+` FROM push_subscriptions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[PushSubscriptionRow])
}

// FetchPushSubscriptionsByCategory returns endpoints opted into a category.
// Used by senders (e.g. prayer-time reminders, new content).
func (db *DB) FetchPushSubscriptionsByCategory(ctx context.Context, category string) ([]PushSubscriptionRow, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+pushCols+` FROM push_subscriptions WHERE $1 = ANY(categories)`, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[PushSubscriptionRow])
}
