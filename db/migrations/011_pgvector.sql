-- Enable pgvector extension.
-- Requires the pgvector extension to be installed on the PostgreSQL server
-- (pre-installed on Railway, Supabase, Neon, and most managed providers).
CREATE EXTENSION IF NOT EXISTS vector;

-- Add a nullable embedding column to verse_text.
-- NULL until seed_embeddings populates it — existing queries are unaffected.
ALTER TABLE verse_text
  ADD COLUMN IF NOT EXISTS embedding vector(512);

-- HNSW index for approximate nearest-neighbor cosine search.
-- Built on the (initially empty) column at startup, then updated as rows are seeded.
-- m=16, ef_construction=128: good defaults for ~100K rows at 512 dims.
CREATE INDEX IF NOT EXISTS idx_verse_text_embedding_hnsw
  ON verse_text
  USING hnsw (embedding vector_cosine_ops)
  WITH (m = 16, ef_construction = 128);
