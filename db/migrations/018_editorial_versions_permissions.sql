-- 018_editorial_versions_permissions.sql
--
-- Phase 0 of the editorial app port (Sanity Studio -> first-party Next.js editor).
--
-- Introduces two foundations the new editor depends on:
--
--   1. A VERSION REGISTRY (quran_versions, bible_versions). Postgres previously
--      keyed all scripture content by lang_id alone, so two translations in the
--      same language collided (last write won). Sanity models each translation
--      as a distinct "version". These tables give Postgres the same notion so
--      multiple versions per language can coexist.
--
--   2. A FULL-PARITY EDITOR PERMISSION MODEL. The backend previously knew only
--      role=admin and a games_editor flag. The Studio enforced per-module and
--      per-version (read/write) access via editorProfile docs. These tables
--      reproduce that model relationally:
--        - editor_module_grants          : per-module read/write (article, quran, ...)
--        - editor_quran_version_grants   : per-Quran-version read/write
--        - editor_bible_version_grants   : per-Bible-version read/write
--        - editor_quran_reference_pref   : per-editor word-by-word reference version
--
-- Admins (users.role = 'admin') bypass all grants and are never rows here.
-- "My Profile" self-service (an editor editing their own linked author doc) is
-- an application-level rule, not a grant, and is intentionally not modeled here.
--
-- Idempotent: safe to run repeatedly (CREATE ... IF NOT EXISTS, guarded enums,
-- ADD COLUMN IF NOT EXISTS).

-- ── enums ─────────────────────────────────────────────────────────────────────

DO $WS_018$ BEGIN
    CREATE TYPE public.editor_module AS ENUM (
        'article',
        'quran',
        'bible',
        'community',
        'author',
        'category',
        'appendix'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $WS_018$;

-- ── version registry: Quran ────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS public.quran_versions (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug text NOT NULL,
    name text NOT NULL,
    lang_id smallint NOT NULL REFERENCES public.languages(id),
    authors jsonb NOT NULL DEFAULT '[]'::jsonb,
    direction character varying(3) NOT NULL DEFAULT 'ltr',
    is_canonical_english boolean NOT NULL DEFAULT false,
    -- the source/reference version translators read beside their own work
    reference_version_id integer REFERENCES public.quran_versions(id) ON DELETE SET NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS quran_versions_slug_key
    ON public.quran_versions (slug);

-- At most one canonical English source version.
CREATE UNIQUE INDEX IF NOT EXISTS quran_versions_one_canonical_english
    ON public.quran_versions ((is_canonical_english))
    WHERE is_canonical_english;

CREATE INDEX IF NOT EXISTS idx_quran_versions_lang ON public.quran_versions (lang_id);

-- ── version registry: Bible ─────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS public.bible_versions (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug text NOT NULL,
    name text NOT NULL,
    lang_id smallint NOT NULL REFERENCES public.languages(id),
    authors jsonb NOT NULL DEFAULT '[]'::jsonb,
    direction character varying(3) NOT NULL DEFAULT 'ltr',
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS bible_versions_slug_key
    ON public.bible_versions (slug);

CREATE INDEX IF NOT EXISTS idx_bible_versions_lang ON public.bible_versions (lang_id);

-- Bible versions may declare several reference versions (Sanity: referenceVersions[]).
CREATE TABLE IF NOT EXISTS public.bible_version_references (
    version_id integer NOT NULL REFERENCES public.bible_versions(id) ON DELETE CASCADE,
    reference_version_id integer NOT NULL REFERENCES public.bible_versions(id) ON DELETE CASCADE,
    PRIMARY KEY (version_id, reference_version_id),
    CONSTRAINT bible_version_references_no_self CHECK (version_id <> reference_version_id)
);

-- ── editor permissions: per-module ──────────────────────────────────────────────
-- Presence of a row grants READ on the module; can_write additionally grants WRITE.

CREATE TABLE IF NOT EXISTS public.editor_module_grants (
    user_id integer NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    module public.editor_module NOT NULL,
    can_write boolean NOT NULL DEFAULT false,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, module)
);

CREATE INDEX IF NOT EXISTS idx_editor_module_grants_user
    ON public.editor_module_grants (user_id);

-- ── editor permissions: per-Quran-version ───────────────────────────────────────
-- Refines the 'quran' module grant. A user needs both the module grant and a
-- version grant to act on a specific version.

CREATE TABLE IF NOT EXISTS public.editor_quran_version_grants (
    user_id integer NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    version_id integer NOT NULL REFERENCES public.quran_versions(id) ON DELETE CASCADE,
    can_write boolean NOT NULL DEFAULT false,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, version_id)
);

CREATE INDEX IF NOT EXISTS idx_editor_quran_version_grants_user
    ON public.editor_quran_version_grants (user_id);

-- ── editor permissions: per-Bible-version ───────────────────────────────────────

CREATE TABLE IF NOT EXISTS public.editor_bible_version_grants (
    user_id integer NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    version_id integer NOT NULL REFERENCES public.bible_versions(id) ON DELETE CASCADE,
    can_write boolean NOT NULL DEFAULT false,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, version_id)
);

CREATE INDEX IF NOT EXISTS idx_editor_bible_version_grants_user
    ON public.editor_bible_version_grants (user_id);

-- ── editor permissions: per-editor word-by-word reference preference ─────────────

CREATE TABLE IF NOT EXISTS public.editor_quran_reference_pref (
    user_id integer NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    reference_version_id integer REFERENCES public.quran_versions(id) ON DELETE SET NULL,
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id)
);

-- ── attach version_id to the (dormant) editorial draft tables ────────────────────
-- Nullable: legacy rows predate versions. New editorial writes set it.

ALTER TABLE public.verse_text_drafts
    ADD COLUMN IF NOT EXISTS version_id integer REFERENCES public.quran_versions(id) ON DELETE SET NULL;
ALTER TABLE public.chapter_text_drafts
    ADD COLUMN IF NOT EXISTS version_id integer REFERENCES public.quran_versions(id) ON DELETE SET NULL;
ALTER TABLE public.word_text_drafts
    ADD COLUMN IF NOT EXISTS version_id integer REFERENCES public.quran_versions(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_verse_text_drafts_version
    ON public.verse_text_drafts (version_id);
CREATE INDEX IF NOT EXISTS idx_chapter_text_drafts_version
    ON public.chapter_text_drafts (version_id);
CREATE INDEX IF NOT EXISTS idx_word_text_drafts_version
    ON public.word_text_drafts (version_id);
