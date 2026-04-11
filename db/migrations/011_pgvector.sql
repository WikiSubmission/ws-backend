-- Enables pgvector for hybrid semantic search.
-- Wrapped in a single DO block so that if the extension is not available on
-- this PostgreSQL instance, the migration succeeds with a NOTICE and the app
-- starts normally in FTS-only mode. No data is lost — the embedding column
-- is simply absent and SearchVersesHybrid falls back to SearchVerses.
DO $$
BEGIN
  -- 1. Install extension — skip everything if unavailable.
  BEGIN
    CREATE EXTENSION IF NOT EXISTS vector;
  EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pgvector extension not available (%), skipping semantic search setup', SQLERRM;
    RETURN;
  END;

  -- 2. Add nullable embedding column (idempotent).
  ALTER TABLE verse_text ADD COLUMN IF NOT EXISTS embedding vector(512);

  -- 3. Create HNSW index (requires pgvector >= 0.5.0).
  --    Falls back to IVFFlat if HNSW is not supported by the installed version.
  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes WHERE indexname = 'idx_verse_text_embedding_hnsw'
  ) THEN
    BEGIN
      EXECUTE 'CREATE INDEX idx_verse_text_embedding_hnsw
               ON verse_text USING hnsw (embedding vector_cosine_ops)
               WITH (m = 16, ef_construction = 128)';
    EXCEPTION WHEN OTHERS THEN
      RAISE NOTICE 'HNSW not supported (%), falling back to IVFFlat', SQLERRM;
      IF NOT EXISTS (
        SELECT 1 FROM pg_indexes WHERE indexname = 'idx_verse_text_embedding_ivfflat'
      ) THEN
        EXECUTE 'CREATE INDEX idx_verse_text_embedding_ivfflat
                 ON verse_text USING ivfflat (embedding vector_cosine_ops)
                 WITH (lists = 100)';
      END IF;
    END;
  END IF;

END $$;
