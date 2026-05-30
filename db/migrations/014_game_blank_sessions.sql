-- Per-round session tracking for multi-attempt blank enforcement.
-- A session is created when a player starts a variant and lives for the
-- duration of that play. game_blank_attempts tracks wrong attempts per
-- blank within the session so the submit path can apply a graduated penalty.

CREATE TABLE IF NOT EXISTS game_sessions (
  session_id TEXT        NOT NULL,
  variant_id TEXT        NOT NULL,
  auth_id    TEXT        NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (session_id)
);

CREATE INDEX IF NOT EXISTS game_sessions_variant_auth
  ON game_sessions (variant_id, auth_id);

CREATE TABLE IF NOT EXISTS game_blank_attempts (
  session_id  TEXT    NOT NULL,
  blank_index INTEGER NOT NULL,
  wrong_count INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (session_id, blank_index)
);
