# English translation sync

Corrects 14 fields of the English Quran text (`verse_text`, language `en`) where ws-backend had
drifted from Masjid Tucson's edition: five verses of wording (A1 to A5: 2:72, 2:286, 14:40, 70:4,
70:41) and nine headings and footnotes that say "Submission" where the edition says "Islam" (B1 to
B9). Each new text matches masjidtucson.org/quran/noframes word for word, checked October 29, 2026.

The fixes came from comparing ws-backend with the WikiSubmission monorepo's English, which found
338 differences in all. The rest (punctuation, quotes, markers, the Basmalah) are held back and
catalogued in [CHANGES.md](CHANGES.md) for a community edition; `changes.json` holds the same list
as data.

| File | What it does |
| --- | --- |
| `english_sync.sql` | The sync. Dry run by default; `-v apply=true` commits. |
| `english_sync_revert.sql` | Puts back the text each synced row had before. Dry run by default. |
| `CHANGES.md` | Every change by group, before and after, with notes for a community edition. |
| `changes.json` | The same changes as data: verse, field, group, old, new. |

## How it stays safe

- One transaction. Any error rolls back everything.
- Each change carries the text it expects to replace. If the live text is anything else (an edit
  made after the review), the run stops before writing and lists the verse.
- Before a row changes, its current text goes to `verse_text_history` with the reason
  `english-sync-2026-10-29`. The revert script restores from those rows.
- Running it again changes nothing: applied rows report `already applied`.
- Only English `verse_text` rows change. Other languages, words, the editorial tables and the
  schema are untouched. No restart is needed; the API reads `verse_text` on every request.

## Running it

From a machine that reaches the production database (with `DATABASE_URL_HETZNER` set):

```bash
psql "$(just url hetzner)" -X -f scripts/english_sync/english_sync.sql
psql "$(just url hetzner)" -X -v apply=true -f scripts/english_sync/english_sync.sql
```

Or on the server, through the database container (stdin works the same as `-f`):

```bash
docker exec -i <postgres container> psql -U <user> -d <database> -X < english_sync.sql
docker exec -i <postgres container> psql -U <user> -d <database> -X -v apply=true < english_sync.sql
```

The dry run must show 14 fields `pending` (or `already applied`) and an empty conflicts table.
To undo, run `english_sync_revert.sql` the same way.

## Deploy order

`db/migrations/002_data_fixes.sql` runs on every boot and used to force 14:40 to "Our Lord". It now
sets "My Lord", matching the sync. Deploy that change before or after running the sync; either way
the two agree once both are in. Until it deploys, a restart would put "Our Lord" back on 14:40
only, and rerunning the sync fixes it.

## Side effects to know

- Search: `content_tsv` is generated, so full-text search follows the new text at once. The
  semantic search embeddings of the five reworded verses are left as they were; the edits are small.
  `just embed-quran` refreshes only rows whose embedding is null.
- Browser caches hold Quran responses for up to an hour.
- Offline bundles built before the sync carry the old text until rebuilt (`just offline-bundles`).
- `assets/source_data` and `assets/verse_text.csv` still hold the old text. Reseeding from them
  (`just seed`) would bring it back.
