-- Web Push subscriptions for PWA / Trusted Web Activity (Google Play) clients.
-- One row per browser push endpoint. Keyed by auth_id (TEXT) like game_sessions
-- rather than the users FK, so a subscription survives before a users row is
-- provisioned and is trivial to prune. The endpoint is globally unique, so a
-- re-subscribe upserts in place.

CREATE TABLE IF NOT EXISTS push_subscriptions (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  auth_id    TEXT        NOT NULL,
  endpoint   TEXT        NOT NULL UNIQUE,
  p256dh     TEXT        NOT NULL,
  auth_key   TEXT        NOT NULL,
  categories TEXT[]      NOT NULL DEFAULT '{}',
  locale     TEXT,
  timezone   TEXT,
  user_agent TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS push_subscriptions_auth_id
  ON push_subscriptions (auth_id);
