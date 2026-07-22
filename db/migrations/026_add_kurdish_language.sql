-- 026_add_kurdish_language.sql
--
-- Adds Kurdish (ku) to the languages registry. The dataset already carries a
-- Kurdish Quran version (seeded via studio's seed:kurdish-quran, and preserved
-- in APP_LANGUAGES), but the baseline seed in 001_baseline.sql never listed the
-- code. Without a row here, studio's sync:content can't resolve `ku` to a
-- language id and silently skips the Kurdish version and any appendices under it.
--
-- id is GENERATED ALWAYS AS IDENTITY, so it is intentionally omitted — Postgres
-- assigns it. Consumers match languages by code, not id.
--
-- Idempotent: safe to run repeatedly.

INSERT INTO public.languages (code, name, direction)
VALUES ('ku', 'Kurdish', 'rtl')
ON CONFLICT (code) DO NOTHING;
