-- 024_editorial_content.sql
--
-- First-party storage for the editorial content modules (Sanity deprecation):
-- articles, authors, categories, communities, and version-scoped appendices.
--
-- Model — document pair, one row per document:
--   draft      jsonb  : the working copy edited in /editor (always present)
--   published  jsonb  : the snapshot served to readers (null until published)
-- Status is derived, never stored:
--   published IS NULL                     -> 'draft'
--   draft IS DISTINCT FROM published      -> 'changed'
--   otherwise                             -> 'published'
--
-- Hot lookup fields (slug, language, appendix code, article author) are
-- extracted into real columns on write so uniqueness and filters stay
-- relational; everything else lives in the payload.
--
-- Idempotent: safe to run repeatedly.

-- ── authors ───────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS public.editorial_authors (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug text NOT NULL,
    -- optional link to a WS account (replaces Sanity's sanityUserId)
    user_id integer REFERENCES public.users(id) ON DELETE SET NULL,
    draft jsonb NOT NULL DEFAULT '{}'::jsonb,
    published jsonb,
    created_by integer REFERENCES public.users(id) ON DELETE SET NULL,
    updated_by integer REFERENCES public.users(id) ON DELETE SET NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    published_at timestamp with time zone
);

CREATE UNIQUE INDEX IF NOT EXISTS editorial_authors_slug_key
    ON public.editorial_authors (slug);

-- ── categories ────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS public.editorial_categories (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug text NOT NULL,
    draft jsonb NOT NULL DEFAULT '{}'::jsonb,
    published jsonb,
    created_by integer REFERENCES public.users(id) ON DELETE SET NULL,
    updated_by integer REFERENCES public.users(id) ON DELETE SET NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    published_at timestamp with time zone
);

CREATE UNIQUE INDEX IF NOT EXISTS editorial_categories_slug_key
    ON public.editorial_categories (slug);

-- ── articles ──────────────────────────────────────────────────────────────────
-- translation_group ties language variants of the same article together
-- (replaces @sanity/document-internationalization).

CREATE TABLE IF NOT EXISTS public.editorial_articles (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    translation_group uuid NOT NULL DEFAULT gen_random_uuid(),
    lang_id smallint NOT NULL REFERENCES public.languages(id),
    slug text NOT NULL,
    author_id integer REFERENCES public.editorial_authors(id) ON DELETE SET NULL,
    draft jsonb NOT NULL DEFAULT '{}'::jsonb,
    published jsonb,
    created_by integer REFERENCES public.users(id) ON DELETE SET NULL,
    updated_by integer REFERENCES public.users(id) ON DELETE SET NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    published_at timestamp with time zone
);

CREATE UNIQUE INDEX IF NOT EXISTS editorial_articles_lang_slug_key
    ON public.editorial_articles (lang_id, slug);

-- one variant per language within a translation group
CREATE UNIQUE INDEX IF NOT EXISTS editorial_articles_group_lang_key
    ON public.editorial_articles (translation_group, lang_id);

CREATE INDEX IF NOT EXISTS idx_editorial_articles_author
    ON public.editorial_articles (author_id);

-- ── communities ───────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS public.editorial_communities (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug text NOT NULL,
    draft jsonb NOT NULL DEFAULT '{}'::jsonb,
    published jsonb,
    created_by integer REFERENCES public.users(id) ON DELETE SET NULL,
    updated_by integer REFERENCES public.users(id) ON DELETE SET NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    published_at timestamp with time zone
);

CREATE UNIQUE INDEX IF NOT EXISTS editorial_communities_slug_key
    ON public.editorial_communities (slug);

-- ── appendices (Quran-version-scoped) ─────────────────────────────────────────
-- Editorial replacement for the read-only public.appendices seed; the public
-- reader is repointed at cutover.

CREATE TABLE IF NOT EXISTS public.editorial_appendices (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    version_id integer NOT NULL REFERENCES public.quran_versions(id) ON DELETE CASCADE,
    code text NOT NULL,
    draft jsonb NOT NULL DEFAULT '{}'::jsonb,
    published jsonb,
    created_by integer REFERENCES public.users(id) ON DELETE SET NULL,
    updated_by integer REFERENCES public.users(id) ON DELETE SET NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    published_at timestamp with time zone
);

CREATE UNIQUE INDEX IF NOT EXISTS editorial_appendices_version_code_key
    ON public.editorial_appendices (version_id, code);
