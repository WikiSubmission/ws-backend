-- Queued, audited per-category deletion of a user's personal data.
-- Mirrors user_data_exports: one row per request, processed asynchronously.
CREATE TABLE IF NOT EXISTS user_data_deletions (
  id bigserial PRIMARY KEY,
  user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status text NOT NULL CHECK (status IN ('queued','running','done','failed')),
  categories text[] NOT NULL,
  counts jsonb,
  error text,
  requested_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz
);

CREATE INDEX IF NOT EXISTS user_data_deletions_user_requested_idx
  ON user_data_deletions (user_id, requested_at DESC);
