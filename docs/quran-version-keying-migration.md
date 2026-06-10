# Runbook (DEFERRED): Version-key the live Quran corpus

> **Status:** Deferred. Do this **last**, after the editorial write path
> (Phase 1) is otherwise complete and exercised. It is the one piece that
> touches the live, public-reader corpus, so it gets its own careful, staged
> rollout (local → staging → prod) with verification and rollback.
>
> Phase 1 is built so this can be switched on later **without contract or
> handler changes** — see "Why this is safe to defer" below.

## Problem

The public reader serves `verse_text` / `chapter_text` / `word_text`, all keyed
by `(verse_id, lang_id)` / `(chapter_id, lang_id)` / `(word_id, lang_id)` with a
table-wide `UNIQUE` on those pairs. None of them carry a `version_id`.

Migration `018` introduced the `quran_versions` registry and added `version_id`
to the **draft** tables only. So a translation "version" exists for editing, but
the published corpus still collapses every translation in a language into one
row. Two versions in the same language collide: approving a **new English
translation** would overwrite **canonical English** — the text the public reads.

The fix is to make the published corpus version-keyed instead of lang-keyed, and
to backfill the existing rows so no data is lost.

## Why this is safe to defer

Phase 1 never writes the live reader tables. The publish/approve path writes to
**separate, additive, version-keyed editorial tables**
(`quran_verse_text_published`, `quran_chapter_text_published`, keyed by
`(version_id, verse_id)` / `(version_id, chapter_id)`). The public reader query
is untouched.

All "where does published text live" logic is isolated in the `db` layer
(`db/quran_editorial_publish.go`). Switching to a version-keyed live corpus later
means changing those functions' target table and the reader query — the OpenAPI
contract, the generated types, and the HTTP handlers do not change.

The reference rail reads the reference version's published text with a fallback:
editorial published table first, then legacy `verse_text` by the reference
version's `lang_id` (so canonical English, which currently only exists as legacy
rows, still renders).

## Target schema

For each of `verse_text`, `chapter_text`, `word_text`:

1. `ADD COLUMN version_id integer REFERENCES quran_versions(id)` (nullable during
   backfill).
2. Backfill `version_id` from `lang_id` via a deterministic lang→version map
   (see below).
3. After verification (zero NULLs), `SET NOT NULL`.
4. Replace the lang-unique with a version-unique:
   - `verse_text`: drop `uq_verse_text_lang (verse_id, lang_id)` →
     `UNIQUE (version_id, verse_id)`.
   - `chapter_text`: drop `uq_chapter_text_lang (chapter_id, lang_id)` →
     `UNIQUE (version_id, chapter_id)`.
   - `word_text`: drop `uq_word_text_lang (word_id, lang_id)` →
     `UNIQUE (version_id, word_id)`.
5. Keep `lang_id` (still useful for direction/display; now derivable from the
   version). FTS / trigram / embedding / FK-to-`languages` indexes are unaffected.

### lang → version backfill map

For every distinct `lang_id` present in the published tables, ensure a
`quran_versions` row exists, then map each existing row to that version:

- The `is_canonical_english` version owns existing English (`lang_id` = en).
- Every other existing language becomes its own version (slug derived from the
  language code/name, `direction` from `languages`).
- Backfill is pure SQL keyed on `lang_id`; it never reads or rewrites the text
  itself, so it cannot corrupt content.

After the corpus is version-keyed, fold the Phase 1 editorial published tables
into it (or repoint the reader at them) and retire the separate tables.

## Reader change

The public reader (`db/quran.go`, `db/search.go`) currently joins published text
by `lang_id`. It must resolve **lang → a chosen version** (the canonical/default
version for that language) and filter by `version_id`. Add a per-language default
(e.g. `quran_versions.is_default_for_lang`, or reuse `is_canonical_english` for
English plus a default flag for others). This is the only change that affects
public output and must be validated against golden reader responses.

## Staged rollout

**Roles:** confirm which host is prod vs staging before starting.
- Hetzner / Coolify: `78.47.218.163:5432/ws_backend`
- Railway: `metro.proxy.rlwy.net:38165/railway`

> **Caution:** migrations auto-run at startup via `db.DbSetup()`. Whatever
> `DATABASE_*` / `DATABASE_URL` the deployed binary reads is what gets migrated.
> Gate this migration carefully so it lands in the intended environment only.

1. **Local**
   - Restore a fresh dump of the real corpus into the local `ws_backend` DB.
   - Run the migration. Verify: zero NULL `version_id`; row counts unchanged
     pre/post; reader golden responses identical for the canonical/default
     versions; no orphaned rows.
   - Test rollback (see below) on the local copy.
2. **Staging**
   - Take a backup/dump first.
   - Apply, run the same verification suite, smoke-test the public reader and the
     editor publish flow end to end.
3. **Production**
   - Take a backup/dump first. Schedule a low-traffic window.
   - Apply, run verification, monitor reader error rate and latency.

## Verification queries (run pre/post in every environment)

- Row counts unchanged: `SELECT count(*) FROM verse_text;` (and chapter/word).
- No unmapped rows: `SELECT count(*) FROM verse_text WHERE version_id IS NULL;`
  must be `0` before `SET NOT NULL`.
- Every version referenced exists: anti-join `verse_text.version_id` against
  `quran_versions.id`.
- Reader parity: diff `/quran` and `/search` golden responses for the
  canonical/default version before and after.

## Rollback

The migration is additive until step 4. Until the unique constraint is swapped:
- Rollback = drop the new `version_id` columns and the new unique indexes;
  restore the original `uq_*_lang` constraints. No data is lost.

After step 4 (unique swapped), rollback requires restoring from the
pre-migration dump. Always take that dump immediately before applying in staging
and prod.

## Done when

- All three corpus tables are version-keyed with `NOT NULL version_id` and
  version-unique constraints.
- The reader resolves lang → default version and parity holds.
- Phase 1 editorial published tables are folded in or repointed, and the
  separate editorial tables are retired.
