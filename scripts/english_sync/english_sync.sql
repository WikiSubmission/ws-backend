-- English translation sync: brings verse_text (lang 'en') in line with the
-- WikiSubmission monorepo's English text. Generated; see README.md and CHANGES.md.
--
-- Dry run (default), reports and rolls back:
--   psql "$DATABASE_URL" -X -f scripts/english_sync/english_sync.sql
-- Apply:
--   psql "$DATABASE_URL" -X -v apply=true -f scripts/english_sync/english_sync.sql
--
-- Every change carries the text it expects to replace. A row whose live text is
-- neither the expected old text nor the new one is a conflict, and any conflict
-- aborts the whole run, so nothing is overwritten that was not reviewed. Each
-- changed row is archived to verse_text_history first, with reason 'english-sync-2026-10-29',
-- which english_sync_revert.sql restores from. Running it again is a no-op.
-- 14 fields across 14 verses; held back: C, D, E, F, G, H, I, J, K, L, M, N, O.

\set ON_ERROR_STOP on
\if :{?apply}
\else
\set apply false
\endif

BEGIN;
SET LOCAL standard_conforming_strings = on;

CREATE TEMP TABLE english_sync (
    chapter integer NOT NULL,
    verse integer NOT NULL,
    field text NOT NULL CHECK (field IN ('content', 'subtitle', 'footer')),
    ids text NOT NULL,
    old text NOT NULL,
    new text NOT NULL,
    PRIMARY KEY (chapter, verse, field)
) ON COMMIT DROP;

INSERT INTO english_sync (chapter, verse, field, ids, old, new) VALUES
(2, 72, 'content', 'A1', 'You had killed a person, then disputed among yourselves. GOD was to expose what you tried to conceal.', 'You had killed a soul, then disputed among yourselves. GOD was to expose what you tried to conceal.'),
(2, 127, 'subtitle', 'B1', 'Abraham Delivered All the Practices of Submission', 'Abraham Delivered All the Practices of Submission (Islam)'),
(2, 135, 'subtitle', 'B2', 'Submission: Abraham''s Religion*', 'Submission (Islam): Abraham''s Religion*'),
(2, 286, 'content', 'A2', 'GOD never burdens a soul beyond its means: to its credit is what it earns, and against it is what it commits. "Our Lord, do not condemn us if we forget or make mistakes. Our Lord, and protect us from blaspheming against You, like those before us have done. Our Lord, protect us from sinning until it becomes too late for us to repent. Pardon us, forgive us, and have mercy on us. You are our Lord and Master. Grant us victory over the disbelieving people."', 'GOD never burdens a soul beyond its means: to its credit is what it earns, and against it is what it commits. "Our Lord, do not condemn us if we forget or make mistakes. Our Lord, and protect us from blaspheming against You, like those before us have done. Our Lord, protect us from sinning until it becomes too late for us to repent. Pardon us and forgive us. You are our Lord and Master. Grant us victory over the disbelieving people."'),
(4, 125, 'subtitle', 'B3', 'Abraham: Original Messenger of Submission*', 'Abraham: Original Messenger of Islam*'),
(14, 40, 'content', 'A3', '"Our Lord, make me one who consistently observes the Contact Prayers (Salat), and also my children. Our Lord, please answer my prayers.', '"My Lord, make me one who consistently observes the Contact Prayers (Salat), and also my children. Our Lord, please answer my prayers.'),
(21, 73, 'subtitle', 'B4', 'Abraham: Delivered All Religious Duties of Submission', 'Abraham: Delivered All Religious Duties of Islam'),
(21, 96, 'footer', 'B5', '±21:96 By the year 2270 AD, thanks to God''s mathematical miracle in the Quran (Appendix 1), America will be the heart of Submission, and billions around the globe will have believed in the Quran (9:33, 41:53, 48:28, 61:9). Gog and Magog (allegorical names of villainous communities), will be the only bastions of heathenism, and they will attack the submitters. That is when the world will end (15:87, 18:94, Appendix 25). Gog and Magog are mentioned in 18:94 & 21:96, 17 verses before the end of each sura; this may indicate the time of their appearance.', '±21:96 By the year 2270 AD, thanks to God''s mathematical miracle in the Quran (Appendix 1), America will be the heart of Islam, and billions around the globe will have believed in the Quran (9:33, 41:53, 48:28, 61:9). Gog and Magog (allegorical names of villainous communities), will be the only bastions of heathenism, and they will attack the submitters. That is when the world will end (15:87, 18:94, Appendix 25). Gog and Magog are mentioned in 18:94 & 21:96, 17 verses before the end of each sura; this may indicate the time of their appearance.'),
(22, 26, 'subtitle', 'B6', 'Pilgrimage, Like All Duties in Submission, Decreed Through Abraham*', 'Pilgrimage, Like All Duties in Islam, Decreed Through Abraham*'),
(22, 78, 'subtitle', 'B7', 'Abraham: Original Messenger of Submission*', 'Abraham: Original Messenger of Islam*'),
(39, 45, 'footer', 'B8', '±39:45 Despite the clear commandment in 3:18 that the First Pillar of Submission is proclaiming: "Ash-hadu An Lã Elãha Ellã Allãh (there is no other god beside God)," the majority of "Muslims" insist upon adding the name of Muhammad. This Greatest Criterion alerts us that rejoicing in adding the name of Muhammad, or any other name, exposes disbelief in the Hereafter. See also Footnote 17:46.', '±39:45 Despite the clear commandment in 3:18 that the First Pillar of Islam is proclaiming: "Ash-hadu An Lã Elãha Ellã Allãh (there is no other god beside God)," the majority of "Muslims" insist upon adding the name of Muhammad. This Greatest Criterion alerts us that rejoicing in adding the name of Muhammad, or any other name, exposes disbelief in the Hereafter. See also Footnote 17:46.'),
(63, 1, 'footer', 'B9', '±63:1 The "first pillar of Submission," as stated in 3:18 is to bear witness that God is the only god. But the corrupted "Muslim" scholars add "Muhammad is God''s messenger," and this violates a number of commandments (see 2:285). Verse 63:1 is the only place in the Quran where such a statement is made. Only the hypocrites make such a statement.', '±63:1 The "first pillar of Islam," as stated in 3:18 is to bear witness that God is the only god. But the corrupted "Muslim" scholars add "Muhammad is God''s messenger," and this violates a number of commandments (see 2:285). Verse 63:1 is the only place in the Quran where such a statement is made. Only the hypocrites make such a statement.'),
(70, 4, 'content', 'A4', 'The angels and the Spirit, with their reports, climb to Him in a day that equals fifty thousand years.', 'The angels, with their reports, climb to Him in a day that equals fifty thousand years.'),
(70, 41, 'content', 'A5', 'to substitute better people in their place; we can never be defeated.', 'to substitute better people in your place; we can never be defeated.');

CREATE TEMP TABLE english_sync_rows ON COMMIT DROP AS
SELECT r.*, vt.id AS verse_text_id,
       COALESCE(CASE r.field WHEN 'content' THEN vt.content WHEN 'subtitle' THEN vt.subtitle ELSE vt.footer END, '') AS live
FROM english_sync r
LEFT JOIN chapters c ON c.chapter_number = r.chapter
LEFT JOIN verses v ON v.chapter_id = c.id AND v.verse_index = r.verse
LEFT JOIN verse_text vt ON vt.verse_id = v.id
                       AND vt.lang_id = (SELECT id FROM languages WHERE code = 'en');

ALTER TABLE english_sync_rows ADD COLUMN status text;
UPDATE english_sync_rows SET status = CASE
    WHEN verse_text_id IS NULL THEN 'missing'
    WHEN live = new THEN 'already applied'
    WHEN live = old THEN 'pending'
    ELSE 'conflict'
END;

\echo
\echo 'Fields by status:'
SELECT status, count(*) AS fields FROM english_sync_rows GROUP BY status ORDER BY status;

\echo 'Conflicts and missing rows (must be empty):'
SELECT chapter || ':' || verse AS verse, field, ids, status, left(live, 120) AS live, left(old, 120) AS expected
FROM english_sync_rows WHERE status IN ('conflict', 'missing') ORDER BY chapter, verse, field;

DO $english_sync$
BEGIN
    IF EXISTS (SELECT 1 FROM english_sync_rows WHERE status IN ('conflict', 'missing')) THEN
        RAISE EXCEPTION 'english sync: live text differs from what was reviewed; nothing changed';
    END IF;
END
$english_sync$;

INSERT INTO verse_text_history (verse_text_id, verse_id, lang_id, content, subtitle, footer, reason)
SELECT vt.id, vt.verse_id, vt.lang_id, vt.content, vt.subtitle, vt.footer, 'english-sync-2026-10-29'
FROM verse_text vt
WHERE vt.id IN (SELECT verse_text_id FROM english_sync_rows WHERE status = 'pending');

UPDATE verse_text vt SET content = r.new, published_at = now()
FROM english_sync_rows r WHERE r.verse_text_id = vt.id AND r.field = 'content' AND r.status = 'pending';
UPDATE verse_text vt SET subtitle = r.new, published_at = now()
FROM english_sync_rows r WHERE r.verse_text_id = vt.id AND r.field = 'subtitle' AND r.status = 'pending';
UPDATE verse_text vt SET footer = r.new, published_at = now()
FROM english_sync_rows r WHERE r.verse_text_id = vt.id AND r.field = 'footer' AND r.status = 'pending';

DO $english_sync$
BEGIN
    IF EXISTS (
        SELECT 1 FROM english_sync_rows r JOIN verse_text vt ON vt.id = r.verse_text_id
        WHERE COALESCE(CASE r.field WHEN 'content' THEN vt.content WHEN 'subtitle' THEN vt.subtitle ELSE vt.footer END, '') <> r.new
    ) THEN
        RAISE EXCEPTION 'english sync: a row did not take its new text; nothing changed';
    END IF;
END
$english_sync$;

SELECT count(*) AS verse_rows_archived FROM verse_text_history WHERE reason = 'english-sync-2026-10-29';

\if :apply
COMMIT;
\echo 'Applied.'
\else
ROLLBACK;
\echo 'Dry run: rolled back. Run again with -v apply=true to apply.'
\endif
