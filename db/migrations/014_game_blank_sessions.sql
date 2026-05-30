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

-- Expand game_variants language constraint to all 15 supported translation codes.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'game_variants_language_values'
      AND conrelid = 'game_variants'::regclass
  ) THEN
    ALTER TABLE game_variants DROP CONSTRAINT game_variants_language_values;
  END IF;
  ALTER TABLE game_variants
    ADD CONSTRAINT game_variants_language_values
    CHECK (language IN ('en','ar','ac','fa','ur','fr','de','es','sv','tr','id','tl','ru','bn','ta'));
END
$$;
