-- Adds skill_band to game_variants so adaptive difficulty can produce
-- separate variant catalogs per player skill level, keeping determinism.

ALTER TABLE game_variants
  ADD COLUMN IF NOT EXISTS skill_band TEXT NOT NULL DEFAULT 'medium';

ALTER TABLE game_variants
  ADD CONSTRAINT game_variants_skill_band_values
  CHECK (skill_band IN ('easy', 'medium', 'hard'));

-- Existing variants default to 'medium' — safe to expand the unique key.
DROP INDEX IF EXISTS game_variants_unique_config;
CREATE UNIQUE INDEX IF NOT EXISTS game_variants_unique_config
  ON game_variants (passage_id, language, difficulty, size, skill_band, counter);
