-- 019_quran_publish_workflow.sql
--
-- Phase 1 of the editorial app port: the Quran write path.
--
-- Editors save per-verse / per-chapter DRAFTS against a Quran version (the
-- draft tables already carry version_id since migration 018), then submit a
-- CHAPTER for publishing. A chapter publish request stays `pending` until an
-- approver (admin, or a holder of the per-version approve grant added here)
-- approves it, at which point that chapter's drafts are synced into the
-- EDITORIAL published tables introduced here. Every save / request / decision
-- is recorded in an append-only changelog.
--
-- IMPORTANT: this migration is fully ADDITIVE and touches NONE of the live
-- public-reader tables (verse_text / chapter_text / word_text). Publishing
-- writes to the new version-keyed editorial tables below, never the reader
-- corpus. Folding the editorial corpus into a version-keyed reader corpus is a
-- separate, deferred, staged migration -- see
-- docs/quran-version-keying-migration.md. Keeping publish isolated here is what
-- lets that decision be switched later without contract or handler changes.
--
-- Idempotent: safe to run repeatedly (guarded enum, CREATE ... IF NOT EXISTS,
-- ADD COLUMN IF NOT EXISTS).

-- ── enums ─────────────────────────────────────────────────────────────────────

DO $WS_019$ BEGIN
    CREATE TYPE public.quran_publish_status AS ENUM (
        'pending',
        'approved',
        'rejected',
        'cancelled'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $WS_019$;

-- ── approver grant ──────────────────────────────────────────────────────────
-- Refines the per-Quran-version grant with a dedicated APPROVE capability,
-- separate from write. Admins approve every version regardless of this column.

ALTER TABLE public.editor_quran_version_grants
    ADD COLUMN IF NOT EXISTS can_approve boolean NOT NULL DEFAULT false;

-- ── chapter publish requests (per version + chapter) ──────────────────────────

CREATE TABLE IF NOT EXISTS public.quran_publish_requests (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    version_id integer NOT NULL REFERENCES public.quran_versions(id) ON DELETE CASCADE,
    chapter_number integer NOT NULL CHECK (chapter_number BETWEEN 1 AND 114),
    status public.quran_publish_status NOT NULL DEFAULT 'pending',
    note text,
    requested_by integer NOT NULL REFERENCES public.users(id),
    requested_at timestamp with time zone NOT NULL DEFAULT now(),
    reviewed_by integer REFERENCES public.users(id),
    reviewed_at timestamp with time zone,
    review_note text,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now()
);

-- At most one OPEN (pending) request per version+chapter. Resolved requests
-- (approved/rejected/cancelled) accumulate as history.
CREATE UNIQUE INDEX IF NOT EXISTS quran_publish_requests_one_pending
    ON public.quran_publish_requests (version_id, chapter_number)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_quran_publish_requests_status
    ON public.quran_publish_requests (status);
CREATE INDEX IF NOT EXISTS idx_quran_publish_requests_version
    ON public.quran_publish_requests (version_id);
CREATE INDEX IF NOT EXISTS idx_quran_publish_requests_requested_by
    ON public.quran_publish_requests (requested_by);

-- ── append-only editorial changelog ───────────────────────────────────────────
-- One row per editorial event. `action` is a free-text label (verse_draft_saved,
-- chapter_draft_saved, publish_requested, publish_approved, publish_rejected,
-- publish_cancelled, published). `detail` carries event-specific extras.

CREATE TABLE IF NOT EXISTS public.quran_editorial_changelog (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    version_id integer NOT NULL REFERENCES public.quran_versions(id) ON DELETE CASCADE,
    chapter_number integer,
    verse_number integer,
    action text NOT NULL,
    actor_id integer NOT NULL REFERENCES public.users(id),
    publish_request_id integer REFERENCES public.quran_publish_requests(id) ON DELETE SET NULL,
    detail jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamp with time zone NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_quran_changelog_version_chapter
    ON public.quran_editorial_changelog (version_id, chapter_number, created_at DESC);

-- ── editorial published corpus (version-keyed, additive) ──────────────────────
-- Distinct from the live reader's verse_text / chapter_text. Approving a publish
-- request syncs that chapter's drafts here. The reader is never touched.

CREATE TABLE IF NOT EXISTS public.quran_verse_text_published (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    version_id integer NOT NULL REFERENCES public.quran_versions(id) ON DELETE CASCADE,
    verse_id integer NOT NULL REFERENCES public.verses(id) ON DELETE CASCADE,
    subtitle text,
    content text,
    footer text,
    published_request_id integer REFERENCES public.quran_publish_requests(id) ON DELETE SET NULL,
    published_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT uq_quran_verse_published_version UNIQUE (version_id, verse_id)
);

CREATE INDEX IF NOT EXISTS idx_quran_verse_published_version
    ON public.quran_verse_text_published (version_id);

CREATE TABLE IF NOT EXISTS public.quran_chapter_text_published (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    version_id integer NOT NULL REFERENCES public.quran_versions(id) ON DELETE CASCADE,
    chapter_id integer NOT NULL REFERENCES public.chapters(id) ON DELETE CASCADE,
    title text,
    published_request_id integer REFERENCES public.quran_publish_requests(id) ON DELETE SET NULL,
    published_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT uq_quran_chapter_published_version UNIQUE (version_id, chapter_id)
);

CREATE INDEX IF NOT EXISTS idx_quran_chapter_published_version
    ON public.quran_chapter_text_published (version_id);

-- ── draft upsert keys ─────────────────────────────────────────────────────────
-- One draft per (version, verse) / (version, chapter). Partial so legacy rows
-- predating versions (version_id IS NULL) are unaffected. These are the
-- ON CONFLICT targets for the editorial draft upserts.

CREATE UNIQUE INDEX IF NOT EXISTS uq_verse_text_drafts_version_verse
    ON public.verse_text_drafts (version_id, verse_id)
    WHERE version_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_chapter_text_drafts_version_chapter
    ON public.chapter_text_drafts (version_id, chapter_id)
    WHERE version_id IS NOT NULL;
