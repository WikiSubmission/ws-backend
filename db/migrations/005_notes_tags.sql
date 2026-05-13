-- 005_notes_tags.sql
-- Adds a tags column to notes for the editorial /me/notes index.
-- Idempotent — safe to re-run.

ALTER TABLE notes
    ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';

-- GIN index supports tag filtering: WHERE tags && ARRAY['tafsir']
CREATE INDEX IF NOT EXISTS notes_tags_gin_idx ON notes USING GIN (tags);
