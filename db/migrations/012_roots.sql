-----------------------------------------
-- Roots concordance: materialized view + abjadi rank
-----------------------------------------
--
-- Pre-aggregates word_text.word_root into a single small table for the /roots
-- endpoints. The matview is populated on first creation from whatever corpus
-- data is present at migration time; subsequent re-seeds require a manual
-- REFRESH (run `make refresh-roots`).
--
-- abjadi_rank composes the first three letters of a root using the classical
-- abjadi numeric weights (1..28) into l1*1000 + l2*30 + l3, giving a stable,
-- lexicographically-correct ordering. The letter weights are inlined into the
-- function body via a VALUES table so the function is self-contained and
-- safe to invoke from a materialized view body without depending on a
-- session-resolved helper function.

-- Drop the previous helper if a prior migration created it; the new
-- abjadi_rank no longer needs it.
DROP FUNCTION IF EXISTS abjadi_letter_rank(text);

-- Single self-contained abjadi rank function.
CREATE OR REPLACE FUNCTION abjadi_rank(letters text) RETURNS int AS $$
  WITH ranks(ch, n) AS (
    VALUES
      ('ا', 1),  ('ب', 2),  ('ج', 3),  ('د', 4),
      ('ه', 5),  ('و', 6),  ('ز', 7),  ('ح', 8),
      ('ط', 9),  ('ي', 10), ('ك', 11), ('ل', 12),
      ('م', 13), ('ن', 14), ('س', 15), ('ع', 16),
      ('ف', 17), ('ص', 18), ('ق', 19), ('ر', 20),
      ('ش', 21), ('ت', 22), ('ث', 23), ('خ', 24),
      ('ذ', 25), ('ض', 26), ('ظ', 27), ('غ', 28)
  ),
  parts AS (
    SELECT string_to_array(letters, ' ') AS arr
  )
  SELECT
      COALESCE((SELECT n FROM ranks WHERE ch = arr[1]), 0) * 1000
    + COALESCE((SELECT n FROM ranks WHERE ch = arr[2]), 0) * 30
    + COALESCE((SELECT n FROM ranks WHERE ch = arr[3]), 0)
  FROM parts;
$$ LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE;

-- Materialized view aggregating distinct roots and their occurrence counts.
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

-- Indexes on the matview.
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
