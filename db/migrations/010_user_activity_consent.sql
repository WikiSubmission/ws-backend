-- Activity feed + consent flag.
--
-- A new consent column governs whether browsing/search activity is recorded.
-- Default is true (consent granted) so existing users keep working without an
-- opt-in prompt; they can revoke at any time from the settings page. Writes
-- are dropped server-side when consent is false.
--
-- user_activity records discrete events (search query, chapter browse, verse
-- range browse). It is read by the user-facing activity page and can be wiped
-- by the user via DELETE.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS activity_consent boolean NOT NULL DEFAULT true;

CREATE TABLE IF NOT EXISTS user_activity (
    id          bigserial PRIMARY KEY,
    user_id     integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        text NOT NULL CHECK (kind IN ('search', 'browse_chapter', 'browse_verse', 'browse_verse_range')),
    scripture   text NOT NULL CHECK (scripture IN ('quran', 'bible')),
    verse_key   text,
    query       text,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS user_activity_user_created_idx
    ON user_activity (user_id, created_at DESC);
