-- Stores live subscriptions (grabs) to public collections owned by other users.
CREATE TABLE IF NOT EXISTS collection_subscriptions (
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  collection_id BIGINT  NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, collection_id)
);

CREATE INDEX IF NOT EXISTS collection_subscriptions_user
  ON collection_subscriptions (user_id, created_at DESC);
