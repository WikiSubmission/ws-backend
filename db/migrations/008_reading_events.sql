-- 008_reading_events.sql
-- Timestamped reading events for hour-of-day analytics.
-- Complements the day-aggregated `reading_log` (which powers streak rollups).

CREATE TABLE IF NOT EXISTS reading_events (
  id          BIGSERIAL PRIMARY KEY,
  user_id     INTEGER     NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  scripture   TEXT        NOT NULL CHECK (scripture IN ('quran', 'bible')),
  ts          TIMESTAMPTZ NOT NULL DEFAULT now(),
  verses_read INTEGER     NOT NULL DEFAULT 1 CHECK (verses_read > 0)
);

CREATE INDEX IF NOT EXISTS idx_reading_events_user_scripture_ts
  ON reading_events (user_id, scripture, ts DESC);

CREATE INDEX IF NOT EXISTS idx_reading_events_user_ts
  ON reading_events (user_id, ts DESC);
