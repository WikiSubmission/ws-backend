-----------------------------------------
-- Roots concordance: materialized view + abjadi rank
-----------------------------------------
--
-- Pre-aggregates word_text.word_root into a single small table for the /roots
-- endpoints. The matview is populated on first creation from whatever corpus
-- data is present at migration time; subsequent re-seeds require a manual
-- REFRESH (run `make refresh-roots`).
--
-- The abjadi rank function maps Arabic letters to their classical numeric
-- weight (1..28); the per-root rank composes the first three letters as
-- l1*1000 + l2*30 + l3 to give a stable, lexicographically-correct ordering.

-- 1. Per-letter abjadi rank.
CREATE OR REPLACE FUNCTION abjadi_letter_rank(ch text) RETURNS int AS $$
  SELECT CASE ch
    WHEN 'ا' THEN 1  WHEN 'ب' THEN 2  WHEN 'ج' THEN 3  WHEN 'د' THEN 4
    WHEN 'ه' THEN 5  WHEN 'و' THEN 6  WHEN 'ز' THEN 7  WHEN 'ح' THEN 8
    WHEN 'ط' THEN 9  WHEN 'ي' THEN 10 WHEN 'ك' THEN 11 WHEN 'ل' THEN 12
    WHEN 'م' THEN 13 WHEN 'ن' THEN 14 WHEN 'س' THEN 15 WHEN 'ع' THEN 16
    WHEN 'ف' THEN 17 WHEN 'ص' THEN 18 WHEN 'ق' THEN 19 WHEN 'ر' THEN 20
    WHEN 'ش' THEN 21 WHEN 'ت' THEN 22 WHEN 'ث' THEN 23 WHEN 'خ' THEN 24
    WHEN 'ذ' THEN 25 WHEN 'ض' THEN 26 WHEN 'ظ' THEN 27 WHEN 'غ' THEN 28
    ELSE 0
  END;
$$ LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE;

-- 2. Composite root rank over the first three letters.
CREATE OR REPLACE FUNCTION abjadi_rank(letters text) RETURNS int AS $$
  WITH parts AS (
    SELECT string_to_array(letters, ' ') AS arr
  )
  SELECT
    COALESCE(abjadi_letter_rank(arr[1]), 0) * 1000
  + COALESCE(abjadi_letter_rank(arr[2]), 0) * 30
  + COALESCE(abjadi_letter_rank(arr[3]), 0)
  FROM parts;
$$ LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE;

-- 3. Materialized view aggregating distinct roots and their occurrence counts.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_matviews WHERE matviewname = 'root_index_mv') THEN
    EXECUTE $mv$
      CREATE MATERIALIZED VIEW root_index_mv AS
      SELECT
        word_root              AS letters,
        COUNT(*)::int          AS occurrences,
        abjadi_rank(word_root) AS abjadi_rank
      FROM   word_text
      WHERE  word_root IS NOT NULL AND word_root <> ''
      GROUP  BY word_root
    $mv$;
  END IF;
END $$;

-- 4. Indexes on the matview.
-- Unique index on letters: required for REFRESH MATERIALIZED VIEW CONCURRENTLY.
CREATE UNIQUE INDEX IF NOT EXISTS idx_root_index_mv_letters
  ON root_index_mv(letters);

-- text_pattern_ops: enables index-friendly LIKE 'prefix%' under any DB collation.
CREATE INDEX IF NOT EXISTS idx_root_index_mv_letters_pattern
  ON root_index_mv(letters text_pattern_ops);

-- Frequency sort.
CREATE INDEX IF NOT EXISTS idx_root_index_mv_occurrences
  ON root_index_mv(occurrences DESC);

-- Abjadi sort (forward and reverse).
CREATE INDEX IF NOT EXISTS idx_root_index_mv_abjadi
  ON root_index_mv(abjadi_rank);
