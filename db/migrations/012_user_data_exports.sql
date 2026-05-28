CREATE TABLE IF NOT EXISTS user_data_exports (
  id bigserial PRIMARY KEY,
  user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status text NOT NULL CHECK (status IN ('queued','running','sent','failed')),
  drive_file_id text,
  drive_url text,
  error text,
  requested_at timestamptz NOT NULL DEFAULT now(),
  sent_at timestamptz
);

CREATE INDEX IF NOT EXISTS user_data_exports_user_requested_idx
  ON user_data_exports (user_id, requested_at DESC);
