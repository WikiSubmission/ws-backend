-- Site-wide search index: one row per (route, lang, section) of the site's own
-- pages, so the command menu can search every page in the reader's language.
--
-- Bodies are seeded from db/seeds/site_docs_<lang>.json, extracted from the
-- rendered site by org/apps/web/scripts/extract-site-text.ts. Chapter and
-- appendix landing rows are derived in SQL from chapter_text / appendices (see
-- db/site.go RebuildDerivedSiteDocuments), never round-tripped through JSON.
--
-- One table on purpose. Compare 022_library_documents.sql, which needs a
-- parent/child split because a single appendix has 30+ ordered sections sharing
-- one title. The unit of a site hit is a route plus an anchor, and most rows are
-- page-level, so a split here would double the row count and force a join for
-- the common case. The cost is `title` repeated on section rows: ~50 bytes
-- across a few thousand rows.
--
-- Library bodies are NOT copied here. library_sections is already seeded and
-- indexed; SearchSiteDocuments unions it directly. NOTE: if /appendices ever
-- repoints from library_documents to editorial_appendices, that union leg goes
-- stale — the fix is to change which table the lib_hits CTE reads.
--
-- `lang` is deliberately not a foreign key to `languages`. These are UI locale
-- codes (ckb, kmr) while the languages registry carries content codes (ku, see
-- 026_add_kurdish_language.sql). Same choice library_documents.lang makes.
--
-- No embedding column yet. At roughly 3k rows a GIN index answers in well under
-- a millisecond, and nobody searches a privacy policy semantically; adding
-- vector(512) would put an OpenAI call in the seed path for no gain. When it is
-- wanted: ALTER TABLE ... ADD COLUMN IF NOT EXISTS embedding public.vector(512)
-- plus an HNSW index. The trigram index below is worth having now, because
-- "donat", "privcy" and "wrdlab" are the realistic failure mode.
--
-- Idempotent: safe to re-run.

CREATE TABLE IF NOT EXISTS public.site_documents (
    id            INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    route         TEXT     NOT NULL,             -- '/practices/zakat'
    anchor        TEXT,                          -- '#calculating-zakat'; NULL on the page row
    lang          TEXT     NOT NULL,             -- UI locale: en|ar|ckb|de|fr|kmr|tr
    kind          TEXT     NOT NULL,             -- page|section|chapter|appendix|article|practice|legal
    section_index SMALLINT NOT NULL DEFAULT 0,   -- 0 = page row, >0 = nth section

    title         TEXT     NOT NULL,             -- page title, repeated on each section row
    heading       TEXT,                          -- section heading; NULL on the page row
    description   TEXT,                          -- one-line summary; also feeds llms.txt
    body          TEXT     NOT NULL DEFAULT '',  -- plain text; '' for title-only rows

    -- Editorial weight (home 100 … legal 30), added to ts_rank_cd the same way
    -- the +0.3 chapter-title boost works in db/search.go.
    priority      SMALLINT NOT NULL DEFAULT 50,

    -- 'simple' like every other content tsvector in this schema: exact tokens,
    -- no stemming surprises with seven languages in one column. setweight puts
    -- the title in band A, headings and descriptions in B, body in C, so
    -- ts_rank_cd does the field weighting instead of a hand-rolled score.
    content_tsv   tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(title, '')),       'A') ||
        setweight(to_tsvector('simple', coalesce(heading, '')),     'B') ||
        setweight(to_tsvector('simple', coalesce(description, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(body, '')),        'C')
    ) STORED,

    -- Trigram haystack for the fuzzy fallback leg. Body is excluded on purpose:
    -- similarity() against a 6KB body is meaningless and the index would be
    -- enormous. Typo tolerance only needs to cover names and headings.
    fuzzy_text    TEXT GENERATED ALWAYS AS (
        coalesce(title, '') || ' ' || coalesce(heading, '') || ' ' || coalesce(description, '')
    ) STORED,

    -- Scopes the full-replace: 'seed' rows are wiped per language on re-seed,
    -- 'derived' rows are rebuilt from chapter_text/appendices, 'editorial' rows
    -- are written by the publish path.
    source        TEXT NOT NULL DEFAULT 'seed'
                  CHECK (source IN ('seed', 'derived', 'editorial')),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- anchor is NULL on page rows, so a plain UNIQUE would admit duplicates;
-- coalesce makes NULL collide like a real value (same trick as
-- library_documents_identity_idx).
CREATE UNIQUE INDEX IF NOT EXISTS site_documents_identity_idx
    ON public.site_documents (route, lang, COALESCE(anchor, ''), section_index);

CREATE INDEX IF NOT EXISTS site_documents_content_tsv_idx
    ON public.site_documents USING GIN (content_tsv);

CREATE INDEX IF NOT EXISTS site_documents_fuzzy_trgm_idx
    ON public.site_documents USING GIN (fuzzy_text public.gin_trgm_ops);

CREATE INDEX IF NOT EXISTS site_documents_lang_kind_idx
    ON public.site_documents (lang, kind, source);
