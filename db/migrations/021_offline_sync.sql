-- Offline user-data sync: idempotency keys make POST /me/sync safe to replay.
-- Each client mutation carries a UUID; the server records the outcome per key so
-- a re-sent batch (flaky reconnect) returns "duplicate" instead of re-applying.
-- Keys are retained ~30 days (see db.CleanupIdempotencyKeys).

CREATE TABLE IF NOT EXISTS idempotency_keys (
    key        TEXT PRIMARY KEY,
    user_id    INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status     TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idempotency_keys_created_at_idx
    ON idempotency_keys (created_at);
