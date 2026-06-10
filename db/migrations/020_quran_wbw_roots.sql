-- 020_quran_wbw_roots.sql
--
-- Phase 2 of the editorial app port: Quran word-by-word editing + root meanings.
--
-- Editors edit per-word text against a Quran version (word_text_drafts already
-- carries version_id since migration 018) and a per-version, per-root MEANING.
-- A root's meaning is stored ONCE per (version, root); the word table renders it
-- by JOIN, so editing one root meaning "broadcasts" to every word sharing that
-- root by construction -- atomically, with no multi-row write (this replaces the
-- Studio's non-transactional client-side multi-doc patch).
--
-- Roots are normalized out of the denormalized word_text.word_root text column
-- into a dedicated master table (quran_roots), so meanings can FK a stable id.
--
-- Word drafts publish WITH their chapter (folded into the existing chapter
-- publish workflow from migration 019). Root meanings span chapters, so they
-- publish via their own dedicated approver action -- their drafts sync into
-- quran_root_meanings_published.
--
-- IMPORTANT: fully ADDITIVE. Touches NONE of the live reader tables
-- (word_text / verse_text / chapter_text). The editorial published corpus stays
-- isolated behind the db layer (see docs/quran-version-keying-migration.md).
--
-- Idempotent: guarded backfill, CREATE ... IF NOT EXISTS, partial unique indexes.

-- ── roots master ────────────────────────────────────────────────────────────
-- One row per distinct Arabic root. Backfilled once from the denormalized
-- word_text.word_root column; thereafter the rectification/seed paths keep it in
-- step. The guard (NOT EXISTS) makes the backfill a no-op on every startup after
-- the first populate, so migrations stay cheap.

CREATE TABLE IF NOT EXISTS public.quran_roots (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    letters text NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS quran_roots_letters_key
    ON public.quran_roots (letters);

INSERT INTO public.quran_roots (letters)
SELECT DISTINCT word_root
FROM   public.word_text
WHERE  word_root IS NOT NULL
  AND  word_root <> ''
  AND  NOT EXISTS (SELECT 1 FROM public.quran_roots)
ON CONFLICT (letters) DO NOTHING;

-- ── per-version root meaning DRAFTS ───────────────────────────────────────────
-- One editable meaning per (version, root). Reuses the existing translation_status
-- enum (draft / published / archived) so the lifecycle matches verse drafts.

CREATE TABLE IF NOT EXISTS public.quran_root_meaning_drafts (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    version_id integer NOT NULL REFERENCES public.quran_versions(id) ON DELETE CASCADE,
    root_id integer NOT NULL REFERENCES public.quran_roots(id) ON DELETE CASCADE,
    meaning text,
    status public.translation_status NOT NULL DEFAULT 'draft',
    author_id integer REFERENCES public.users(id),
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT uq_quran_root_meaning_drafts_version_root UNIQUE (version_id, root_id)
);

CREATE INDEX IF NOT EXISTS idx_quran_root_meaning_drafts_version
    ON public.quran_root_meaning_drafts (version_id);

-- ── per-version root meaning PUBLISHED ────────────────────────────────────────
-- Synced from drafts by the dedicated "publish root meanings" approver action.

CREATE TABLE IF NOT EXISTS public.quran_root_meanings_published (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    version_id integer NOT NULL REFERENCES public.quran_versions(id) ON DELETE CASCADE,
    root_id integer NOT NULL REFERENCES public.quran_roots(id) ON DELETE CASCADE,
    meaning text,
    published_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT uq_quran_root_meanings_published_version_root UNIQUE (version_id, root_id)
);

CREATE INDEX IF NOT EXISTS idx_quran_root_meanings_published_version
    ON public.quran_root_meanings_published (version_id);

-- ── editorial published per-word text (version-keyed, additive) ───────────────
-- Distinct from the live reader's word_text. Approving a chapter publish request
-- syncs that chapter's word drafts here (see migration 019 for the verse/chapter
-- equivalents). The reader corpus is never touched.

CREATE TABLE IF NOT EXISTS public.quran_word_text_published (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    version_id integer NOT NULL REFERENCES public.quran_versions(id) ON DELETE CASCADE,
    word_id integer NOT NULL REFERENCES public.words(id) ON DELETE CASCADE,
    lang_id smallint NOT NULL REFERENCES public.languages(id),
    word text,
    meaning text,
    published_request_id integer REFERENCES public.quran_publish_requests(id) ON DELETE SET NULL,
    published_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT uq_quran_word_published_version_word UNIQUE (version_id, word_id)
);

CREATE INDEX IF NOT EXISTS idx_quran_word_published_version
    ON public.quran_word_text_published (version_id);

-- ── word draft upsert key ─────────────────────────────────────────────────────
-- One word draft per (version, word). Partial so legacy rows predating versions
-- (version_id IS NULL) are unaffected. This is the ON CONFLICT target for the
-- editorial word-draft upsert. The version fixes the language, so (version, word)
-- is sufficient even though word_text_drafts also carries lang_id.

CREATE UNIQUE INDEX IF NOT EXISTS uq_word_text_drafts_version_word
    ON public.word_text_drafts (version_id, word_id)
    WHERE version_id IS NOT NULL;
