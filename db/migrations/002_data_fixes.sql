-- 002_data_fixes.sql
-- Production data corrections against the normalized schema.
-- All statements are idempotent: UPDATEs are exact-match SETs, and the 14:40
-- replace() becomes a no-op once the substring is gone. The schema_migrations
-- ledger runs this file once per database, so new fixes go in a new numbered
-- migration, not here; do not touch 001_baseline.sql for data changes.

-- ---------------------------------------------------------------------------
-- Word fixes (lookup by words.global_index, which equals the legacy index)
-- ---------------------------------------------------------------------------

-- 2:60:28 — Arabic glyph fix
UPDATE word_text SET word = 'مُفْسِدِينَ'
WHERE word_id = (SELECT id FROM words WHERE global_index = 908)
  AND lang_id = (SELECT id FROM languages WHERE code = 'ar');

-- 16:62:16
UPDATE word_text SET word = 'وَأَنَّهُم'
WHERE word_id = (SELECT id FROM words WHERE global_index = 34971)
  AND lang_id = (SELECT id FROM languages WHERE code = 'ar');

-- 16:114:11 — root fix (applies to all language rows for this word)
UPDATE word_text SET word_root = 'ك و ن'
WHERE word_id = (SELECT id FROM words WHERE global_index = 35818);

-- 20:97:18
UPDATE word_text SET word = 'إِلَـٰهِكَ'
WHERE word_id = (SELECT id FROM words WHERE global_index = 41028)
  AND lang_id = (SELECT id FROM languages WHERE code = 'ar');

-- 21:44:7
UPDATE word_text SET word = 'عَلَيْهِمُ'
WHERE word_id = (SELECT id FROM words WHERE global_index = 41939)
  AND lang_id = (SELECT id FROM languages WHERE code = 'ar');

-- 26:173:4 — root fix
UPDATE word_text SET word_root = 'س و ء'
WHERE word_id = (SELECT id FROM words WHERE global_index = 48198);

-- 29:20:9
UPDATE word_text SET word = 'ثُمَّ'
WHERE word_id = (SELECT id FROM words WHERE global_index = 51356)
  AND lang_id = (SELECT id FROM languages WHERE code = 'ar');

-- 55:69:1
UPDATE word_text SET word = 'فَبِأَيِّ'
WHERE word_id = (SELECT id FROM words WHERE global_index = 69029)
  AND lang_id = (SELECT id FROM languages WHERE code = 'ar');

-- ---------------------------------------------------------------------------
-- Verse fixes
-- ---------------------------------------------------------------------------

-- 105:3 — transliteration
UPDATE verse_text SET content = 'wa-arsala ʿalayhim ṭayran abābīla'
WHERE verse_id = (SELECT v.id FROM verses v
                  JOIN chapters c ON c.id = v.chapter_id
                  WHERE c.chapter_number = 105 AND v.verse_index = 3)
  AND lang_id = (SELECT id FROM languages WHERE code = 'tl');

-- 14:40, English: "Our Lord" → "My Lord" in the opening clause, as the
-- English sync sets it (scripts/english_sync). Before the migration ledger this
-- ran on every boot, so it had to agree with the synced text.
UPDATE verse_text
SET content = replace(content,
    'Our Lord, make me one who consistently observes',
    'My Lord, make me one who consistently observes')
WHERE verse_id = (SELECT v.id FROM verses v
                  JOIN chapters c ON c.id = v.chapter_id
                  WHERE c.chapter_number = 14 AND v.verse_index = 40)
  AND lang_id = (SELECT id FROM languages WHERE code = 'en');
