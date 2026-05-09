-- 004_bookmark_categories_notes_upsert.sql
-- Replaces per-bookmark name/color model with named, colored categories.
-- Enforces one note per (user, scripture, verse_key) with markdown content.
-- Adds FTS index on notes for search.

-- ── 1. Drop old normal bookmarks (test data only — confirmed by user) ─────────

DELETE FROM bookmarks WHERE kind = 'normal';

-- ── 2. Create bookmark_categories ─────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS bookmark_categories (
    id         BIGSERIAL PRIMARY KEY,
    user_id    INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    color      TEXT NOT NULL DEFAULT 'amber',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS bookmark_categories_user_id_idx
    ON bookmark_categories (user_id);

-- ── 3. Create bookmark_entries ─────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS bookmark_entries (
    id          BIGSERIAL PRIMARY KEY,
    category_id BIGINT NOT NULL REFERENCES bookmark_categories(id) ON DELETE CASCADE,
    scripture   TEXT NOT NULL CHECK (scripture IN ('quran', 'bible')),
    verse_key   TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (category_id, scripture, verse_key)
);

CREATE INDEX IF NOT EXISTS bookmark_entries_category_id_idx
    ON bookmark_entries (category_id);

-- ── 4. Enforce one note per verse ─────────────────────────────────────────────

-- Drop old notes (test data only — confirmed by user).
DELETE FROM notes;

-- Unique constraint: one note per (user, scripture, verse_key).
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'notes_user_scripture_verse_unique'
          AND conrelid = 'notes'::regclass
    ) THEN
        ALTER TABLE notes
            ADD CONSTRAINT notes_user_scripture_verse_unique
            UNIQUE (user_id, scripture, verse_key);
    END IF;
END $$;

-- ── 5. Add full-text search index on notes content ────────────────────────────

ALTER TABLE notes DROP COLUMN IF EXISTS content_tsv;

ALTER TABLE notes
    ADD COLUMN IF NOT EXISTS content_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('english', content)) STORED;

CREATE INDEX IF NOT EXISTS notes_content_tsv_idx
    ON notes USING GIN (content_tsv);
