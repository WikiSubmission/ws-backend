package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type FCMDeviceRow struct {
	Token      string    `db:"token"`
	AuthID     *string   `db:"auth_id"`
	Platform   string    `db:"platform"`
	Locale     *string   `db:"locale"`
	Timezone   *string   `db:"timezone"`
	Categories []string  `db:"categories"`
	AppVersion *string   `db:"app_version"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

const fcmDeviceCols = `token, auth_id, platform, locale, timezone, categories, app_version, created_at, updated_at`

// UpsertFCMDevice stores (or refreshes) a device token. The token is the
// primary key, so re-registration after a token refresh or preference change
// updates the row in place. authID is nil for anonymous devices.
func (db *DB) UpsertFCMDevice(
	ctx context.Context,
	token string,
	authID *string,
	platform string,
	locale, timezone *string,
	categories []string,
	appVersion *string,
) (*FCMDeviceRow, error) {
	if categories == nil {
		categories = []string{}
	}
	query := `INSERT INTO fcm_devices
	            (token, auth_id, platform, locale, timezone, categories, app_version)
	          VALUES ($1, $2, $3, $4, $5, $6, $7)
	          ON CONFLICT (token) DO UPDATE SET
	            auth_id = COALESCE(EXCLUDED.auth_id, fcm_devices.auth_id),
	            platform = EXCLUDED.platform,
	            locale = EXCLUDED.locale,
	            timezone = EXCLUDED.timezone,
	            categories = EXCLUDED.categories,
	            app_version = EXCLUDED.app_version,
	            updated_at = NOW()
	          RETURNING ` + fcmDeviceCols
	rows, err := db.Pool.Query(ctx, query, token, authID, platform, locale, timezone, categories, appVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[FCMDeviceRow])
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// DeleteFCMDevice removes a token (app opt-out). Returns false when nothing
// matched.
func (db *DB) DeleteFCMDevice(ctx context.Context, token string) (bool, error) {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM fcm_devices WHERE token = $1`, token)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// DeleteFCMDevicesByTokens batch-prunes tokens FCM reported as unregistered.
func (db *DB) DeleteFCMDevicesByTokens(ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	_, err := db.Pool.Exec(ctx, `DELETE FROM fcm_devices WHERE token = ANY($1)`, tokens)
	return err
}

// FetchFCMDevices returns registered devices, optionally restricted to those
// opted into a category. Used by the notification dispatcher.
func (db *DB) FetchFCMDevices(ctx context.Context, categoryFilter *string) ([]FCMDeviceRow, error) {
	query := `SELECT ` + fcmDeviceCols + ` FROM fcm_devices WHERE $1::text IS NULL OR $1 = ANY(categories)`
	rows, err := db.Pool.Query(ctx, query, categoryFilter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[FCMDeviceRow])
}
