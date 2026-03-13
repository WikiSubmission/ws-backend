-- Optional but valuable: trigram similarity for fuzzy/partial matching
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Stored tsvector on verse_text (combines content + subtitle + footer)
-- GENERATED ALWAYS AS STORED auto-populates existing rows on first run
ALTER TABLE verse_text
  ADD COLUMN IF NOT EXISTS content_tsv TSVECTOR
  GENERATED ALWAYS AS (
    to_tsvector('simple',
      coalesce(content,  '') || ' ' ||
      coalesce(subtitle, '') || ' ' ||
      coalesce(footer,   '')
    )
  ) STORED;

CREATE INDEX IF NOT EXISTS idx_verse_fts ON verse_text USING GIN(content_tsv);

-- Stored tsvector on word_text (combines word + meaning)
ALTER TABLE word_text
  ADD COLUMN IF NOT EXISTS word_tsv TSVECTOR
  GENERATED ALWAYS AS (
    to_tsvector('simple',
      coalesce(word,    '') || ' ' ||
      coalesce(meaning, '')
    )
  ) STORED;

CREATE INDEX IF NOT EXISTS idx_word_fts ON word_text USING GIN(word_tsv);

-- Stored tsvector on chapter_text (title only)
ALTER TABLE chapter_text
  ADD COLUMN IF NOT EXISTS title_tsv TSVECTOR
  GENERATED ALWAYS AS (
    to_tsvector('simple', coalesce(title, ''))
  ) STORED;

CREATE INDEX IF NOT EXISTS idx_chapter_text_fts ON chapter_text USING GIN(title_tsv);

-- Trigram indexes for fuzzy/partial search (pg_trgm)
CREATE INDEX IF NOT EXISTS idx_verse_content_trgm  ON verse_text  USING GIN(content gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_word_text_trgm       ON word_text   USING GIN(word    gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_chapter_text_trgm    ON chapter_text USING GIN(title  gin_trgm_ops);
