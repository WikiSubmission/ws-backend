-- 009_games.sql
-- Quran Games v1: Fill the Missing Word.
-- See docs/games-spec.md for the full design.
--
-- Five tables:
--   game_passages       — curated catalog (GROQ-proposed + editor-approved)
--   game_variants       — deterministic round configurations, one row per (passage,lang,difficulty,size,counter)
--   game_attempts       — one row per user submission
--   game_word_frequency — backend-only corpus stats, drives blank scoring
--   game_lemma          — backend-only lemma map, drives Hard-tier acceptance
--
-- All user references use INTEGER FK to users(id), consistent with the rest
-- of the schema (notes, reading_progress, bookmark_categories, etc.).

-- ── game_passages ───────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS game_passages (
  id              BIGSERIAL PRIMARY KEY,
  chapter_start   INTEGER NOT NULL,
  verse_start     INTEGER NOT NULL,
  chapter_end     INTEGER NOT NULL,
  verse_end       INTEGER NOT NULL,
  label           TEXT    NOT NULL,
  themes          TEXT[]  NOT NULL DEFAULT '{}',
  rationale       TEXT    NOT NULL DEFAULT '',
  llm_difficulty  TEXT,
  llm_blank_hint  INTEGER,
  status          TEXT    NOT NULL DEFAULT 'proposed',
  source          TEXT    NOT NULL DEFAULT 'groq',
  approved_at     TIMESTAMPTZ,
  approved_by     INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT game_passages_chapter_range CHECK (chapter_start >= 1 AND chapter_end <= 114),
  CONSTRAINT game_passages_ordered_range CHECK (
    (chapter_start, verse_start) <= (chapter_end, verse_end)
  ),
  CONSTRAINT game_passages_status_values CHECK (
    status IN ('proposed', 'approved', 'rejected', 'needs_refinement')
  ),
  CONSTRAINT game_passages_source_values CHECK (
    source IN ('groq', 'manual')
  ),
  CONSTRAINT game_passages_llm_difficulty_values CHECK (
    llm_difficulty IS NULL OR llm_difficulty IN ('easy', 'moderate', 'challenging')
  )
);
CREATE INDEX IF NOT EXISTS game_passages_status_idx       ON game_passages (status);
CREATE INDEX IF NOT EXISTS game_passages_themes_gin       ON game_passages USING gin (themes);
CREATE INDEX IF NOT EXISTS game_passages_chapter_start_idx ON game_passages (chapter_start);

-- ── game_variants ───────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS game_variants (
  id          BIGSERIAL PRIMARY KEY,
  variant_id  TEXT   UNIQUE NOT NULL,
  passage_id  BIGINT NOT NULL REFERENCES game_passages(id) ON DELETE CASCADE,
  language    TEXT   NOT NULL,
  difficulty  TEXT   NOT NULL,
  size        TEXT   NOT NULL,
  counter     INTEGER NOT NULL,
  seed        BIGINT NOT NULL,
  blanks      JSONB  NOT NULL,
  rendered    JSONB  NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT game_variants_language_values CHECK (language IN ('en','ar','ac','fa','ur','fr','de','es','sv','tr','id','tl','ru','bn','ta')),
  CONSTRAINT game_variants_difficulty_values CHECK (
    difficulty IN ('easy', 'medium', 'hard', 'professional')
  ),
  CONSTRAINT game_variants_size_values CHECK (size IN ('short', 'medium', 'long'))
);
CREATE UNIQUE INDEX IF NOT EXISTS game_variants_unique_config
  ON game_variants (passage_id, language, difficulty, size, counter);
CREATE INDEX IF NOT EXISTS game_variants_passage_idx
  ON game_variants (passage_id);

-- ── game_attempts ───────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS game_attempts (
  id            BIGSERIAL PRIMARY KEY,
  attempt_id    TEXT    UNIQUE NOT NULL,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  variant_id    TEXT    NOT NULL REFERENCES game_variants(variant_id) ON DELETE CASCADE,
  size          TEXT    NOT NULL,
  difficulty    TEXT    NOT NULL,
  correct_count INTEGER NOT NULL,
  total_count   INTEGER NOT NULL,
  hints_used    INTEGER NOT NULL DEFAULT 0,
  score         INTEGER NOT NULL,
  elapsed_ms    INTEGER,
  per_blank     JSONB   NOT NULL,
  completed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT game_attempts_size_values CHECK (size IN ('short', 'medium', 'long')),
  CONSTRAINT game_attempts_difficulty_values CHECK (
    difficulty IN ('easy', 'medium', 'hard', 'professional')
  ),
  CONSTRAINT game_attempts_counts_nonneg CHECK (
    correct_count >= 0 AND total_count > 0 AND correct_count <= total_count
  ),
  CONSTRAINT game_attempts_hints_nonneg CHECK (hints_used >= 0),
  CONSTRAINT game_attempts_score_nonneg CHECK (score >= 0)
);
CREATE INDEX IF NOT EXISTS game_attempts_leaderboard_idx
  ON game_attempts (size, difficulty, score DESC, elapsed_ms ASC NULLS LAST);
CREATE INDEX IF NOT EXISTS game_attempts_weekly_leaderboard_idx
  ON game_attempts (size, difficulty, completed_at DESC, score DESC);
CREATE INDEX IF NOT EXISTS game_attempts_user_idx
  ON game_attempts (user_id, completed_at DESC);
CREATE INDEX IF NOT EXISTS game_attempts_variant_idx
  ON game_attempts (variant_id, score DESC);

-- ── game_word_frequency ─────────────────────────────────────────────────────
-- Backend-only. Never returned by any endpoint.
CREATE TABLE IF NOT EXISTS game_word_frequency (
  language        TEXT NOT NULL,
  word_normalized TEXT NOT NULL,
  frequency       INTEGER NOT NULL CHECK (frequency >= 0),
  PRIMARY KEY (language, word_normalized)
);

-- ── game_lemma ──────────────────────────────────────────────────────────────
-- Backend-only. Populated from a Python spaCy sidecar (seeded once, checked
-- into a fixture). Drives Hard-tier acceptance.
CREATE TABLE IF NOT EXISTS game_lemma (
  language        TEXT NOT NULL,
  word_normalized TEXT NOT NULL,
  lemma           TEXT NOT NULL,
  PRIMARY KEY (language, word_normalized)
);
CREATE INDEX IF NOT EXISTS game_lemma_lemma_idx ON game_lemma (language, lemma);
