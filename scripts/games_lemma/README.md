# Quran Games — lemma fixture generator

One-time tooling that builds the `game_lemma` seed fixture used for Hard-tier
answer acceptance (a guess of `worshipped` is accepted when the answer is
`worship` because both map to lemma `worship`).

spaCy runs **once**, here, to produce a committed SQL fixture. Production loads
the fixture only; it never runs Python or spaCy at deploy or request time. Until
the fixture exists, Hard tier safely falls back to exact match.

## Prerequisites

1. The frequency table must already be seeded (it is the vocabulary source):

   ```bash
   just games-seed-frequency
   ```

2. Python deps (use a virtualenv):

   ```bash
   cd scripts/games_lemma
   python3 -m venv .venv && source .venv/bin/activate
   pip install -r requirements.txt
   python -m spacy download en_core_web_sm
   ```

## Regenerate the fixture

From the repo root, with the venv active:

```bash
just games-build-lemma-fixture local
```

This dumps the distinct vocabulary from `game_word_frequency`, pipes it through
`generate_lemma_fixture.py`, and writes
`assets/source_data/games_lemma_en.sql`. Commit that file.

## Load the fixture

```bash
just games-seed-lemma local      # or: railway, hetzner, ...
```

Idempotent: the fixture wraps a `DELETE` for the language plus
`INSERT ... ON CONFLICT DO UPDATE` in a transaction.

## Notes

- Only words whose lemma differs from the surface form are stored. Identity
  mappings are dropped because the matcher falls back to exact match anyway.
- Keys (`word_normalized`) match `game_word_frequency` exactly: lowercase,
  letters and digits only.
