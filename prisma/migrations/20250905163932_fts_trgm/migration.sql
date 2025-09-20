-- ========================================
-- Full Text Search + Trigram Migration (Fixed)
-- ========================================

-- Enable extensions
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;

-- ========================================
-- Verses
ALTER TABLE "Verses"
ADD COLUMN IF NOT EXISTS subtitle_tsv tsvector,
ADD COLUMN IF NOT EXISTS content_tsv tsvector,
ADD COLUMN IF NOT EXISTS footer_tsv tsvector,
ADD COLUMN IF NOT EXISTS combined_tsv tsvector;

-- Words
ALTER TABLE "Words"
ADD COLUMN IF NOT EXISTS content_tsv tsvector,
ADD COLUMN IF NOT EXISTS root_tsv tsvector;

-- ChapterMetadata
ALTER TABLE "ChapterMetadata"
ADD COLUMN IF NOT EXISTS title_english_tsv tsvector,
ADD COLUMN IF NOT EXISTS title_arabic_tsv tsvector,
ADD COLUMN IF NOT EXISTS title_transliterated_tsv tsvector,
ADD COLUMN IF NOT EXISTS combined_tsv tsvector;

-- ========================================
-- Language-aware FTS function
-- ========================================

CREATE OR REPLACE FUNCTION language_to_tsvector(lang "Languages", txt text)
RETURNS tsvector AS $$
BEGIN
  CASE lang
    WHEN 'ENGLISH' THEN
      RETURN to_tsvector('english', unaccent(coalesce(txt, '')));
    WHEN 'FRENCH' THEN
      RETURN to_tsvector('french', unaccent(coalesce(txt, '')));
    WHEN 'ARABIC_TASHKEEL', 'ARABIC_NON_TASHKEEL', 'ARABIC_ROOT' THEN
      RETURN to_tsvector('arabic', coalesce(txt, ''));
    WHEN 'GERMAN' THEN
      RETURN to_tsvector('german', unaccent(coalesce(txt, '')));
    WHEN 'RUSSIAN' THEN
      RETURN to_tsvector('russian', coalesce(txt, ''));
    ELSE
      -- Fallback: simple with unaccent for other languages
      RETURN to_tsvector('simple', unaccent(coalesce(txt, '')));
  END CASE;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Language-aware tsquery function
CREATE OR REPLACE FUNCTION language_to_tsquery(lang "Languages", txt text)
RETURNS tsquery AS $$
BEGIN
  CASE lang
    WHEN 'ENGLISH' THEN
      RETURN plainto_tsquery('english', unaccent(coalesce(txt, '')));
    WHEN 'FRENCH' THEN
      RETURN plainto_tsquery('french', unaccent(coalesce(txt, '')));
    WHEN 'ARABIC_TASHKEEL', 'ARABIC_NON_TASHKEEL', 'ARABIC_ROOT' THEN
      RETURN plainto_tsquery('arabic', coalesce(txt, ''));
    WHEN 'GERMAN' THEN
      RETURN plainto_tsquery('german', unaccent(coalesce(txt, '')));
    WHEN 'RUSSIAN' THEN
      RETURN plainto_tsquery('russian', coalesce(txt, ''));
    ELSE
      -- Fallback: simple with unaccent for other languages
      RETURN plainto_tsquery('simple', unaccent(coalesce(txt, '')));
  END CASE;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- ========================================
-- Trigger Functions
-- ========================================

-- Verses trigger
CREATE OR REPLACE FUNCTION verses_tsv_trigger()
RETURNS trigger AS $$
BEGIN
  NEW.content_tsv := language_to_tsvector(NEW.language, NEW.content);
  NEW.subtitle_tsv := language_to_tsvector(NEW.language, NEW.subtitle);
  NEW.footer_tsv := language_to_tsvector(NEW.language, NEW.footer);

  -- Give them all equal weight
  NEW.combined_tsv :=
    setweight(language_to_tsvector(NEW.language, NEW.content), 'A') ||
    setweight(language_to_tsvector(NEW.language, coalesce(NEW.subtitle, '')), 'A') ||
    setweight(language_to_tsvector(NEW.language, coalesce(NEW.footer, '')), 'A');

  RETURN NEW;
END
$$ LANGUAGE plpgsql;


-- Words trigger
CREATE OR REPLACE FUNCTION words_tsv_trigger()
RETURNS trigger AS $$
DECLARE
  verse_lang "Languages";
BEGIN
  -- Get language from related verse
  SELECT v.language INTO verse_lang
  FROM "Verses" v WHERE v.uuid = NEW."versesUuid";

  IF verse_lang IS NULL THEN
    verse_lang := 'ENGLISH'::"Languages"; -- fallback with proper casting
  END IF;

  NEW.content_tsv := language_to_tsvector(verse_lang, NEW.content);
  NEW.root_tsv := language_to_tsvector(verse_lang, NEW.root);

  RETURN NEW;
END
$$ LANGUAGE plpgsql;

-- ChapterMetadata trigger
CREATE OR REPLACE FUNCTION chaptermetadata_tsv_trigger()
RETURNS trigger AS $$
BEGIN
  NEW.title_english_tsv := to_tsvector('english', unaccent(coalesce(NEW.title_english, '')));
  NEW.title_arabic_tsv := to_tsvector('arabic', coalesce(NEW.title_arabic, ''));
  NEW.title_transliterated_tsv := to_tsvector('english', unaccent(coalesce(NEW.title_transliterated, '')));

  -- Combined with weights
  NEW.combined_tsv :=
      setweight(NEW.title_english_tsv, 'A') ||
      setweight(NEW.title_arabic_tsv, 'A') ||
      setweight(NEW.title_transliterated_tsv, 'A');


  RETURN NEW;
END
$$ LANGUAGE plpgsql;

-- NEW: Cascade trigger for when verse language changes
CREATE OR REPLACE FUNCTION verses_language_change_trigger()
RETURNS trigger AS $$
BEGIN
  -- Only update words if language actually changed
  IF OLD.language IS DISTINCT FROM NEW.language THEN
    -- Update all words related to this verse to recalculate tsvectors
    UPDATE "Words"
    SET content = content -- This will trigger the words_tsv_trigger
    WHERE "versesUuid" = NEW.uuid;
  END IF;

  RETURN NEW;
END
$$ LANGUAGE plpgsql;

-- ========================================
-- Create/Replace Triggers
-- ========================================

DROP TRIGGER IF EXISTS verses_tsv_update ON "Verses";
CREATE TRIGGER verses_tsv_update
  BEFORE INSERT OR UPDATE ON "Verses"
  FOR EACH ROW EXECUTE FUNCTION verses_tsv_trigger();

-- NEW: Cascade trigger for language changes
DROP TRIGGER IF EXISTS verses_language_cascade ON "Verses";
CREATE TRIGGER verses_language_cascade
  AFTER UPDATE OF language ON "Verses"
  FOR EACH ROW EXECUTE FUNCTION verses_language_change_trigger();

DROP TRIGGER IF EXISTS words_tsv_update ON "Words";
CREATE TRIGGER words_tsv_update
  BEFORE INSERT OR UPDATE ON "Words"
  FOR EACH ROW EXECUTE FUNCTION words_tsv_trigger();

DROP TRIGGER IF EXISTS chaptermetadata_tsv_update ON "ChapterMetadata";
CREATE TRIGGER chaptermetadata_tsv_update
  BEFORE INSERT OR UPDATE ON "ChapterMetadata"
  FOR EACH ROW EXECUTE FUNCTION chaptermetadata_tsv_trigger();

-- ========================================
-- Update existing data
-- ========================================

-- Update all existing verses
UPDATE "Verses" SET content = content;

-- Update all existing words
UPDATE "Words" SET content = content;

-- Update all existing chapter metadata
UPDATE "ChapterMetadata" SET title_english = title_english;

-- ========================================
-- Indexes
-- ========================================

-- FTS Indexes (GIN)
CREATE INDEX IF NOT EXISTS verses_content_tsv_idx ON "Verses" USING GIN (content_tsv);
CREATE INDEX IF NOT EXISTS verses_subtitle_tsv_idx ON "Verses" USING GIN (subtitle_tsv);
CREATE INDEX IF NOT EXISTS verses_footer_tsv_idx ON "Verses" USING GIN (footer_tsv);
CREATE INDEX IF NOT EXISTS verses_combined_tsv_idx ON "Verses" USING GIN (combined_tsv);

CREATE INDEX IF NOT EXISTS words_content_tsv_idx ON "Words" USING GIN (content_tsv);
CREATE INDEX IF NOT EXISTS words_root_tsv_idx ON "Words" USING GIN (root_tsv);

CREATE INDEX IF NOT EXISTS chaptermetadata_title_english_tsv_idx ON "ChapterMetadata" USING GIN (title_english_tsv);
CREATE INDEX IF NOT EXISTS chaptermetadata_title_arabic_tsv_idx ON "ChapterMetadata" USING GIN (title_arabic_tsv);
CREATE INDEX IF NOT EXISTS chaptermetadata_title_transliterated_tsv_idx ON "ChapterMetadata" USING GIN (title_transliterated_tsv);
CREATE INDEX IF NOT EXISTS chaptermetadata_combined_tsv_idx ON "ChapterMetadata" USING GIN (combined_tsv);

-- Trigram Indexes (GIN)
CREATE INDEX IF NOT EXISTS verses_content_trgm_idx ON "Verses" USING GIN (content gin_trgm_ops);
CREATE INDEX IF NOT EXISTS verses_subtitle_trgm_idx ON "Verses" USING GIN (subtitle gin_trgm_ops);
CREATE INDEX IF NOT EXISTS verses_footer_trgm_idx ON "Verses" USING GIN (footer gin_trgm_ops);

CREATE INDEX IF NOT EXISTS words_content_trgm_idx ON "Words" USING GIN (content gin_trgm_ops);
CREATE INDEX IF NOT EXISTS words_root_trgm_idx ON "Words" USING GIN (root gin_trgm_ops);

CREATE INDEX IF NOT EXISTS chaptermetadata_title_english_trgm_idx ON "ChapterMetadata" USING GIN (title_english gin_trgm_ops);
CREATE INDEX IF NOT EXISTS chaptermetadata_title_arabic_trgm_idx ON "ChapterMetadata" USING GIN (title_arabic gin_trgm_ops);
CREATE INDEX IF NOT EXISTS chaptermetadata_title_transliterated_trgm_idx ON "ChapterMetadata" USING GIN (title_transliterated gin_trgm_ops);

-- Additional useful indexes for filtering and cursor pagination
CREATE INDEX IF NOT EXISTS verses_language_chapter_idx ON "Verses" (language, chapter_index);
CREATE INDEX IF NOT EXISTS words_language_idx ON "Words" (language);

-- IMPORTANT: Index for cursor-based pagination stability
CREATE INDEX IF NOT EXISTS verses_global_index_idx ON "Verses" (global_index);
CREATE INDEX IF NOT EXISTS words_global_index_idx ON "Words" (global_index);

-- ========================================
-- Fixed search functions
-- ========================================

-- Language-aware hybrid search rank function
CREATE OR REPLACE FUNCTION hybrid_search_rank(
    search_term text,
    content_field text,
    tsv_field tsvector,
    lang "Languages",
    fts_weight float DEFAULT 1.0,
    similarity_weight float DEFAULT 0.5
) RETURNS float AS $$
BEGIN
    RETURN (
        -- FTS ranking with language-aware query
        COALESCE(ts_rank_cd(tsv_field, language_to_tsquery(lang, search_term)), 0) * fts_weight +
        -- Trigram similarity
        COALESCE(similarity(content_field, search_term), 0) * similarity_weight
    );
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Cursor encoding/decoding functions for stable pagination
CREATE OR REPLACE FUNCTION encode_cursor(score float, global_idx bigint)
RETURNS text AS $$
BEGIN
    RETURN encode(
        convert_to(
            json_build_object('score', score::text, 'global_index', global_idx)::text,
            'UTF8'
        ),
        'base64'
    );
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE OR REPLACE FUNCTION decode_cursor(cursor_str text)
RETURNS json AS $$
DECLARE
    decoded_json json;
BEGIN
    SELECT convert_from(decode(cursor_str, 'base64'), 'UTF8')::json INTO decoded_json;
    RETURN decoded_json;
EXCEPTION
    WHEN OTHERS THEN
        RETURN json_build_object('score', '0', 'global_index', 0);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Set more reasonable trigram similarity threshold
SET pg_trgm.similarity_threshold = 0.25;

-- ========================================
-- Example search query with proper cursor pagination
-- ========================================

-- Example function for verse search with stable pagination
CREATE OR REPLACE FUNCTION search_verses(
    search_term text,
    target_language "Languages" DEFAULT 'ENGLISH',
    page_size int DEFAULT 20,
    cursor_token text DEFAULT NULL
)
RETURNS TABLE (
    uuid uuid,
    content text,
    subtitle text,
    language "Languages",
    chapter_index int,
    verse_index int,
    global_index bigint,
    combined_score float,
    next_cursor text
) AS $$
DECLARE
    cursor_data json;
    cursor_score float;
    cursor_global_idx bigint;
BEGIN
    -- Decode cursor if provided
    IF cursor_token IS NOT NULL THEN
        cursor_data := decode_cursor(cursor_token);
        cursor_score := (cursor_data->>'score')::float;
        cursor_global_idx := (cursor_data->>'global_index')::bigint;
    ELSE
        cursor_score := 999999; -- Start from top
        cursor_global_idx := 0;
    END IF;

    RETURN QUERY
    WITH scored_verses AS (
        SELECT
            v.uuid,
            v.content,
            v.subtitle,
            v.language,
            v.chapter_index,
            v.verse_index,
            v.global_index,
            hybrid_search_rank(
                search_term,
                v.content,
                v.combined_tsv,
                v.language
            ) as score
        FROM "Verses" v
        WHERE
            v.language = target_language
            AND (
                v.combined_tsv @@ language_to_tsquery(v.language, search_term)
                OR similarity(v.content, search_term) > 0.25
            )
    ),
    paginated_results AS (
        SELECT
            sv.*,
            encode_cursor(sv.score, sv.global_index) as cursor
        FROM scored_verses sv
        WHERE
            (sv.score < cursor_score)
            OR (sv.score = cursor_score AND sv.global_index > cursor_global_idx)
        ORDER BY sv.score DESC, sv.global_index ASC
        LIMIT page_size + 1 -- Get one extra to determine if there are more pages
    )
    SELECT
        pr.uuid,
        pr.content,
        pr.subtitle,
        pr.language,
        pr.chapter_index,
        pr.verse_index,
        pr.global_index,
        pr.score,
        CASE
            WHEN row_number() OVER () <= page_size THEN pr.cursor
            ELSE NULL
        END as next_cursor
    FROM paginated_results pr
    LIMIT page_size;
END;
$$ LANGUAGE plpgsql;

-- ========================================
-- Extra Indexes for Hybrid Search
-- ========================================

-- Foreign key join: Words → Verses
-- (needed so EXISTS/joins don’t seq-scan Words)
CREATE INDEX IF NOT EXISTS words_verses_uuid_idx
  ON "Words"("versesUuid");

-- Cursor pagination stability (ORDER BY score, global_index)
-- score can’t be indexed (computed at runtime),
-- but global_index can be — this helps the tiebreak ordering
CREATE INDEX IF NOT EXISTS verses_global_index_idx
  ON "Verses"(global_index);

-- Support ordering/filtering by (language, global_index) combo
-- useful since most queries filter by language first
CREATE INDEX IF NOT EXISTS verses_lang_global_idx
  ON "Verses"(language, global_index);

-- Common join/filter for words with verse language
-- (avoids repeated lookups when recalculating word tsvectors)
CREATE INDEX IF NOT EXISTS words_language_idx
  ON "Words"(language);

-- Speeds up EXISTS/aggregates on words by verse
-- especially when combining similarity + FTS checks
CREATE INDEX IF NOT EXISTS words_versesuuid_contenttsv_idx
  ON "Words"("versesUuid", content_tsv);

  -- Keep versesUuid as a simple B-Tree index
  CREATE INDEX IF NOT EXISTS words_versesuuid_idx
    ON "Words"("versesUuid");

  -- Separate trigram indexes (GIN)
  CREATE INDEX IF NOT EXISTS words_content_trgm_idx
    ON "Words" USING GIN (content gin_trgm_ops);

  CREATE INDEX IF NOT EXISTS words_root_trgm_idx
    ON "Words" USING GIN (root gin_trgm_ops);

-- ========================================
-- Per language indices for better performance
-- ========================================

-- English
CREATE INDEX IF NOT EXISTS verses_content_trgm_en_idx
    ON "Verses" USING GIN (content gin_trgm_ops)
    WHERE language = 'ENGLISH';

CREATE INDEX IF NOT EXISTS words_content_trgm_en_idx
    ON "Words" USING GIN (content gin_trgm_ops)
    WHERE language = 'ENGLISH';

CREATE INDEX IF NOT EXISTS words_root_trgm_en_idx
    ON "Words" USING GIN (root gin_trgm_ops)
    WHERE language = 'ENGLISH';

-- French
CREATE INDEX IF NOT EXISTS verses_content_trgm_fr_idx
    ON "Verses" USING GIN (content gin_trgm_ops)
    WHERE language = 'FRENCH';

CREATE INDEX IF NOT EXISTS words_content_trgm_fr_idx
    ON "Words" USING GIN (content gin_trgm_ops)
    WHERE language = 'FRENCH';

CREATE INDEX IF NOT EXISTS words_root_trgm_fr_idx
    ON "Words" USING GIN (root gin_trgm_ops)
    WHERE language = 'FRENCH';
