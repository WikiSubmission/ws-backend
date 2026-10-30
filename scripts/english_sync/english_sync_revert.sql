-- Reverts english_sync.sql: restores each verse_text row archived with reason
-- 'english-sync-2026-10-29' to the text it had before, archiving the synced text first.
--
-- Dry run (default):
--   psql "$DATABASE_URL" -X -f scripts/english_sync/english_sync_revert.sql
-- Apply:
--   psql "$DATABASE_URL" -X -v apply=true -f scripts/english_sync/english_sync_revert.sql

\set ON_ERROR_STOP on
\if :{?apply}
\else
\set apply false
\endif

BEGIN;

CREATE TEMP TABLE english_sync_before ON COMMIT DROP AS
SELECT DISTINCT ON (verse_text_id) verse_text_id, content, subtitle, footer
FROM verse_text_history
WHERE reason = 'english-sync-2026-10-29'
ORDER BY verse_text_id, archived_at, id;

SELECT count(*) AS rows_to_restore FROM english_sync_before;

INSERT INTO verse_text_history (verse_text_id, verse_id, lang_id, content, subtitle, footer, reason)
SELECT vt.id, vt.verse_id, vt.lang_id, vt.content, vt.subtitle, vt.footer, 'english-sync-2026-10-29-revert'
FROM verse_text vt JOIN english_sync_before b ON b.verse_text_id = vt.id;

UPDATE verse_text vt
SET content = b.content, subtitle = b.subtitle, footer = b.footer, published_at = now()
FROM english_sync_before b WHERE b.verse_text_id = vt.id;

\if :apply
COMMIT;
\echo 'Reverted.'
\else
ROLLBACK;
\echo 'Dry run: rolled back. Run again with -v apply=true to revert.'
\endif
