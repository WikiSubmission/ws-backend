# Quran topical index extraction

Builds `db/seeds/quran_topic_index_en.json` from the alphabetical subject index
printed at the back of *Quran, The Final Testament*, published as seven HTML
pages on masjidtucson.org.

```bash
node build.mjs            # parse the cached pages in raw/
node build.mjs --fetch    # re-download raw/ first, then parse
just seed-topic-index     # load the JSON into Postgres (from the repo root)
```

## Why raw/ is committed

`raw/` holds the seven source pages as fetched on 2026-08-19. They are the
provenance of a generated, committed seed file: without them a rebuild depends on
a third-party site still serving byte-identical 1990s HTML, and any drift would
show up as an unexplained diff in a 1.3 MB JSON file. With them, `node build.mjs`
reproduces the seed exactly, and `--fetch` makes re-syncing an explicit,
reviewable step (the `raw/` diff shows what changed upstream before the seed diff
shows what that did).

## What the parser has to tolerate

The pages are Dreamweaver-era HTML and are not well formed. The parser never
assumes structure:

- Entries are bare `<p>` tags and sub-entries are unclosed `<li>` items. `<ul>`
  open and close counts do not balance (26 vs 29 on the A-C page).
- One entry (`Aaron`) has no `<p>` tag at all and sits bare between a `</ul>` and
  the next `<ul>`. A normalisation pass inserts the missing tag, or Aaron's
  sub-entries end up under Aad.
- The pages declare `charset=iso-8859-1`, not UTF-8.
- Range ends are abbreviated to their trailing digits: `9:36-7` means 9:36-37.
- A handful of separators are wrong: `16:11.67` uses a period for a comma, and a
  couple of references omit the separator entirely.
- `Moses` and `Satan's` are each split across two `<p>` blocks and are merged.

## Anomalies

The run prints every quirk it found rather than dropping anything silently.
Around 23 are expected, all genuine defects in the printed source:

- **bad-verse** (~15): references to verses that do not exist (`27:2244`,
  `9:128` in a 127-verse chapter, `18:628`). Quarantined, not guessed at. An
  entry whose every reference fails validation is still kept, so the title
  remains findable.
- **appendix-reference** (2): the printed index occasionally points at an
  appendix rather than a verse. Appendices are a separate corpus
  (`/appendices`) and are not modelled here.
- **missing-separator** (2), **tail-remainder** (1): typographic damage in the
  source, including `29:26,42:30:5,27` under "Almighty,God is".
- **merged-duplicate** (2), **empty-entry** (1): described above.

A materially different count means the parser regressed, not that the source
changed. Expect roughly 1,217 entries, 987 sub-entries and 13,557 references.
