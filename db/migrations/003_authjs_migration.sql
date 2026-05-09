-- 003_authjs_migration.sql
-- Replace Supabase auth with Auth.js (NextAuth v5).
-- Renames supabase_uid → auth_id (TEXT), adds email uniqueness, and creates
-- tables for bookmarks, notes, reading progress, streaks, preferences, and
-- verse collections.

-- ── 1. Rename supabase_uid → auth_id ─────────────────────────────────────────

ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_id TEXT;

-- Ensure auth_id is TEXT even on legacy databases where auth_id may already
-- exist as UUID (e.g. from an earlier partial migration/rename).
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'users'
      AND column_name = 'auth_id'
      AND data_type <> 'text'
  ) THEN
    ALTER TABLE public.users
      ALTER COLUMN auth_id TYPE TEXT
      USING auth_id::TEXT;
  END IF;
EXCEPTION WHEN others THEN NULL;
END $$;

-- Copy from supabase_uid only if that column still exists (idempotent).
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'users' AND column_name = 'supabase_uid'
  ) THEN
    UPDATE users SET auth_id = supabase_uid::TEXT WHERE auth_id IS NULL;
  END IF;
END $$;

-- SET NOT NULL is safe once every row has a value (or no rows exist yet).
DO $$
BEGIN
  ALTER TABLE users ALTER COLUMN auth_id SET NOT NULL;
EXCEPTION WHEN others THEN NULL;
END $$;

-- Unique constraint on auth_id (replaces supabase_uid unique constraint).
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'users_auth_id_key'
      AND conrelid = 'public.users'::regclass
  ) THEN
    NULL;
  ELSIF EXISTS (
    SELECT 1
    FROM pg_class
    WHERE relname = 'users_auth_id_key'
      AND relkind = 'i'
  ) THEN
    ALTER TABLE public.users
      ADD CONSTRAINT users_auth_id_key UNIQUE USING INDEX users_auth_id_key;
  ELSE
    ALTER TABLE public.users
      ADD CONSTRAINT users_auth_id_key UNIQUE (auth_id);
  END IF;
EXCEPTION WHEN duplicate_object OR duplicate_table THEN NULL;
END $$;

-- Unique constraint on email (enables email-keyed upsert across providers).
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'users_email_key'
      AND conrelid = 'public.users'::regclass
  ) THEN
    NULL;
  ELSIF EXISTS (
    SELECT 1
    FROM pg_class
    WHERE relname = 'users_email_key'
      AND relkind = 'i'
  ) THEN
    ALTER TABLE public.users
      ADD CONSTRAINT users_email_key UNIQUE USING INDEX users_email_key;
  ELSE
    ALTER TABLE public.users
      ADD CONSTRAINT users_email_key UNIQUE (email);
  END IF;
EXCEPTION WHEN duplicate_object OR duplicate_table THEN NULL;
END $$;

-- Remove the old column and its index.
DROP INDEX IF EXISTS idx_users_supabase_uid;
ALTER TABLE users DROP COLUMN IF EXISTS supabase_uid;
CREATE INDEX IF NOT EXISTS idx_users_auth_id ON users (auth_id);

-- ── 2. Bookmarks ──────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS bookmarks (
    id         BIGSERIAL PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scripture  TEXT NOT NULL CHECK (scripture IN ('quran', 'bible')),
    verse_key  TEXT NOT NULL,
    name       TEXT NOT NULL DEFAULT '',
    color      TEXT NOT NULL DEFAULT 'amber',
    kind       TEXT NOT NULL DEFAULT 'normal' CHECK (kind IN ('normal', 'cover_to_cover')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Exactly one cover-to-cover bookmark per (user, scripture)
CREATE UNIQUE INDEX IF NOT EXISTS bookmarks_one_c2c
    ON bookmarks (user_id, scripture)
    WHERE kind = 'cover_to_cover';

CREATE INDEX IF NOT EXISTS bookmarks_user_scripture
    ON bookmarks (user_id, scripture, created_at DESC);

-- ── 3. Notes ──────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS notes (
    id         BIGSERIAL PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scripture  TEXT NOT NULL CHECK (scripture IN ('quran', 'bible')),
    verse_key  TEXT NOT NULL,
    lang       TEXT NOT NULL DEFAULT 'en',
    content    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS notes_user_scripture_verse
    ON notes (user_id, scripture, verse_key);

-- ── 4. Reading progress ───────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS reading_progress (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scripture  TEXT NOT NULL CHECK (scripture IN ('quran', 'bible')),
    verse_key  TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, scripture)
);

-- ── 5. Reading log (streak source) ───────────────────────────────────────────

CREATE TABLE IF NOT EXISTS reading_log (
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scripture   TEXT NOT NULL CHECK (scripture IN ('quran', 'bible')),
    day         DATE NOT NULL,
    verses_read INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, scripture, day)
);

-- ── 6. Cross-device preferences ──────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS preferences (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scripture  TEXT NOT NULL CHECK (scripture IN ('quran', 'bible')),
    payload    JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, scripture)
);

-- ── 7. Verse collections ──────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS collections (
    id          BIGSERIAL PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    is_public   BOOLEAN NOT NULL DEFAULT FALSE,
    share_token TEXT UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS collections_user
    ON collections (user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS collection_verses (
    id            BIGSERIAL PRIMARY KEY,
    collection_id BIGINT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    scripture     TEXT NOT NULL CHECK (scripture IN ('quran', 'bible')),
    verse_key     TEXT NOT NULL,
    note          TEXT NOT NULL DEFAULT '',
    position      INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (collection_id, scripture, verse_key)
);
