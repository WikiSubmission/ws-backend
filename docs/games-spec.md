# Quran Games — Backend Specification (v1)

> Status: design, pre-implementation. Tracks the contract negotiated with the
> frontend in `wikisubmission-org` branch `feat/quran-games`. This document is
> the single source of truth for the v1 backend implementation.
>
> Frontend scaffold lives under `app/quran/games/` in
> `wikisubmission-org`. The frontend `meApi.games.*` stubs in
> `src/api/me-client.ts` declare the contract types it expects.

---

## 1. Overview

A new "Quran Games" section in the frontend, gated behind authentication, with
**Fill the Missing Word** as the first (and currently only) game.

Players pick a curated passage from the Quran (Rashad Khalifa translation),
choose a difficulty tier and round size, and fill in the words the game has
removed. Scores feed three leaderboards: global all-time, weekly rolling, and
per-variant best.

The defining design constraint is that **every round is procedurally generated
from a deterministic seed**. Two players who select the same `(passage, lang,
difficulty, size, variant_id)` get the same blanks in the same positions and
can be compared on equal footing.

### v1 scope

- Language: **English (Rashad Khalifa translation only)**. Arabic in v2.
- Game: Fill the Missing Word. Other game modes are out of scope.
- Authentication: required for every games endpoint.
- Leaderboards: global all-time + weekly rolling + per-variant best.

---

## 2. Where this fits in ws-backend

| Concern | Location |
| --- | --- |
| HTTP handlers | `api/handlers/games.go` (new) |
| OpenAPI fragments | merged into `openapi.yaml` |
| DB schema | `db/migrations/NNNN_games.sql` (new) |
| Row structs | `db/games.go` (new) |
| GROQ curation command | `bin/groq-curate` or `cmd/groq-curate` (new) |
| Justfile recipes | new recipes prefixed `games-…` |

All endpoints sit behind the same Supabase JWT guard used by `/me/*`. New
public endpoints (`/games/*`) also require auth in v1 — the section is hidden
from anonymous visitors in the UI, and the backend enforces the same.

---

## 3. Data model

All tables are added in a single migration. Identifiers use `bigserial` to
match the existing convention. Timestamps default to `now()` and use
`timestamptz`.

### 3.1 `game_passages` — curated catalog

```sql
CREATE TABLE game_passages (
  id              bigserial PRIMARY KEY,
  chapter_start   int  NOT NULL,
  verse_start     int  NOT NULL,
  chapter_end     int  NOT NULL,
  verse_end       int  NOT NULL,
  label           text NOT NULL,           -- short title, e.g. "The opening of The Key"
  themes          text[] NOT NULL DEFAULT '{}',
  rationale       text NOT NULL,           -- editorial reviewer notes from GROQ
  llm_difficulty  text,                    -- 'easy' | 'moderate' | 'challenging', GROQ hint
  llm_blank_hint  int,                     -- GROQ-suggested blank count (advisory only)
  status          text NOT NULL DEFAULT 'proposed',
                  -- 'proposed' | 'approved' | 'rejected' | 'needs_refinement'
  source          text NOT NULL DEFAULT 'groq',  -- 'groq' | 'manual'
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  approved_at     timestamptz,
  approved_by     text,                    -- user id of the editor who approved
  CHECK (chapter_start >= 1 AND chapter_end <= 114),
  CHECK (chapter_start, verse_start) <= (chapter_end, verse_end),
  CHECK (status IN ('proposed', 'approved', 'rejected', 'needs_refinement'))
);
CREATE INDEX game_passages_status_idx ON game_passages (status);
CREATE INDEX game_passages_themes_gin ON game_passages USING gin (themes);
```

Only `status = 'approved'` passages are returned by `/games/fill-blank/passages`.

### 3.2 `game_variants` — generated round configurations

```sql
CREATE TABLE game_variants (
  id            bigserial PRIMARY KEY,
  variant_id    text UNIQUE NOT NULL,   -- e.g. "p42-en-hard-medium-7"
  passage_id    bigint NOT NULL REFERENCES game_passages(id) ON DELETE CASCADE,
  language      text   NOT NULL,         -- 'en' (Arabic in v2)
  difficulty    text   NOT NULL,         -- 'easy' | 'medium' | 'hard' | 'professional'
  size          text   NOT NULL,         -- 'short' | 'medium' | 'long'
  counter       int    NOT NULL,         -- monotonic per (passage, lang, difficulty, size)
  seed          bigint NOT NULL,         -- PRNG seed used during generation
  blanks        jsonb  NOT NULL,         -- see schema below
  rendered      jsonb  NOT NULL,         -- verses with blank tokens substituted
  created_at    timestamptz NOT NULL DEFAULT now(),
  CHECK (language IN ('en', 'ar')),
  CHECK (difficulty IN ('easy', 'medium', 'hard', 'professional')),
  CHECK (size IN ('short', 'medium', 'long'))
);
CREATE UNIQUE INDEX game_variants_unique_config
  ON game_variants (passage_id, language, difficulty, size, counter);
```

`blanks` JSON shape (one entry per blank, ordered by appearance in the passage):

```jsonc
[
  {
    "index": 0,                       // 0-based position within the variant
    "verse_key": "1:2",               // verse this blank lives in
    "word_position": 3,               // 0-based word index inside that verse
    "answer": "praise",               // plaintext, server-only, never returned to the client
    "answer_lemma": "praise",         // for hard-tier acceptance
    "options": ["praise", "thanks", "glory", "mercy"]  // easy-tier only; omitted otherwise
  }
]
```

`rendered` JSON shape:

```jsonc
[
  { "verse_key": "1:2", "text": "__BLANK_0__ be to GOD, Lord of the universe." }
]
```

> **Note:** the API response to the client returns only `index`, `verse_key`,
> `word_position`, and `options` (when difficulty is easy). `answer` and
> `answer_lemma` never leave the server.

### 3.3 `game_attempts` — one row per submission

```sql
CREATE TABLE game_attempts (
  id              bigserial PRIMARY KEY,
  attempt_id      text UNIQUE NOT NULL,    -- ulid or shortid
  user_id         text NOT NULL,
  variant_id      text NOT NULL REFERENCES game_variants(variant_id) ON DELETE CASCADE,
  size            text NOT NULL,           -- denormalized for leaderboard indexing
  difficulty      text NOT NULL,           -- denormalized for leaderboard indexing
  correct_count   int  NOT NULL,
  total_count     int  NOT NULL,
  hints_used      int  NOT NULL DEFAULT 0,
  score           int  NOT NULL,
  elapsed_ms      int,                     -- tiebreaker, nullable
  per_blank       jsonb NOT NULL,          -- [{index, correct, accepted_answer?}]
  completed_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX game_attempts_leaderboard_idx
  ON game_attempts (size, difficulty, score DESC, elapsed_ms ASC);
CREATE INDEX game_attempts_user_idx
  ON game_attempts (user_id, completed_at DESC);
CREATE INDEX game_attempts_variant_idx
  ON game_attempts (variant_id, score DESC);
```

### 3.4 `game_word_frequency` — backend-only reference table

Populated by a one-off command (see §6.1). Used by the variant generator to
rank candidate blanks: rarer words make better blanks.

```sql
CREATE TABLE game_word_frequency (
  language       text NOT NULL,            -- 'en' for v1
  word_normalized text NOT NULL,           -- lower-cased, punctuation stripped
  frequency       int  NOT NULL,           -- raw count in the corpus
  PRIMARY KEY (language, word_normalized)
);
```

> Never shipped to the frontend. Drives blank selection only.

### 3.5 `game_lemma` — backend-only reference table

Populated by a one-off command (see §6.2). Used for Hard-tier answer
acceptance: a guess of `worshipped` is accepted when the answer is `worship`
because both map to lemma `worship`.

```sql
CREATE TABLE game_lemma (
  language       text NOT NULL,            -- 'en' for v1
  word_normalized text NOT NULL,
  lemma          text NOT NULL,
  PRIMARY KEY (language, word_normalized)
);
CREATE INDEX game_lemma_lemma_idx ON game_lemma (language, lemma);
```

> Never shipped to the frontend.

---

## 4. Variant ID scheme

`variant_id` is a human-readable slug:

```
p{passage_id}-{lang}-{difficulty}-{size}-{counter}
```

Examples:

- `p42-en-easy-short-1`
- `p42-en-hard-medium-7`
- `p17-en-professional-long-3`

`counter` is monotonic per `(passage_id, language, difficulty, size)` and
starts at `1`. The generator increments it when minting a new variant.

The frontend uses `variant_id` directly in URLs:
`/quran/games/fill-blank/play/<variant_id>` (shareable, deterministic).

---

## 5. Variant generation algorithm

A pure function over `(passage_id, language, difficulty, size, counter)`. The
same inputs always produce the same output — that is what makes leaderboards
fair and variants shareable.

```text
generate(passage, language, difficulty, size, counter):
    seed   = hash(passage.id, language, difficulty, size, counter)
    rng    = seeded_prng(seed)
    verses = fetch_verses(passage, language)        // from existing /quran data
    tokens = tokenize(verses)
    cands  = score_blankability(tokens, language, difficulty)
    blanks = pick_top_n(cands, rng, target_blank_count(size))
    if difficulty == 'easy':
        distractors = pick_distractors(blanks, rng, same_chapter_pool, frequency_band)
    return build_variant(verses, blanks, distractors_if_easy)
```

### 5.1 Round-size targets (verses × blanks)

| Size   | Verses (target band) | Blanks (target band) |
| ------ | -------------------- | -------------------- |
| short  | 3–5                  | 5–8                  |
| medium | 8–15                 | 12–18                |
| long   | 20+                  | 25–35                |

The picker matches a passage to the requested size by picking the closest
sub-range from the curated passage (passages may span more verses than a
short round wants; the generator selects a contiguous window inside).

### 5.2 Blank-selection policy (v1)

The blankability scorer assigns a score to every word in the passage. Top-N
candidates by score, with the rules below applied as filters or weights:

| Rule | Easy | Medium | Hard | Professional |
| --- | --- | --- | --- | --- |
| Skip stopwords (`the`, `of`, `and`, `to`, `is`, `a`, `in`, `for`, ...) | yes | yes | no | no |
| Minimum word length | 3 chars | 4 chars | none | none |
| Avoid consecutive blanks | yes | yes | yes | yes |
| Mid-verse position bonus | strong | strong | weak | weak |
| Frequency bias (rarer = higher score) | strong | strong | moderate | moderate |

The stopword list is fixed in code as a small set (~50 words). Punctuation is
stripped before normalization; capitalization is ignored for matching but the
original surface form is recorded for the answer.

### 5.3 Easy-tier distractors

Three wrong options per blank, picked deterministically with the same PRNG:

1. Pull the **same-chapter pool** — every unique word from the passage's
   chapter (excluding stopwords and the answer itself).
2. Filter to words whose `game_word_frequency` rank is within ±20% of the
   answer's rank (the "same frequency band").
3. Sample 3 with the PRNG, distinct from each other and from the answer.
4. Shuffle the four options together using the PRNG so position is also
   variant-deterministic.

If the same-chapter pool isn't large enough at the matched frequency band,
widen the band by 10% increments until 3 candidates exist. Fall back to "any
non-stopword from the chapter" as a last resort.

### 5.4 Medium-tier hints

Four hint types available, each costs **25 points** (flat).

| Hint | Reveals |
| --- | --- |
| First letter | First character of the answer (preserving case of the source word) |
| Word length | `____ (n)` where n is the answer's character count |
| Phonetic / first two letters | First two characters |
| Lemma / root | English lemma (looked up from `game_lemma`); Arabic root (v2) |

Each hint is independently toggleable per blank. The frontend tracks
`hints_used` and submits the total count; the server applies the score
penalty.

### 5.5 Hard tier — lemma acceptance

A guess is accepted if either:

- `normalize(guess) == normalize(answer)`, or
- `lookup_lemma(guess) == lookup_lemma(answer)`, where `lookup_lemma` queries
  `game_lemma`. If a word isn't in the lemma table, fall back to exact match.

Normalization: lowercase, strip punctuation and surrounding whitespace.

### 5.6 Professional tier — exact match

`normalize(guess) == normalize(answer)`. Nothing else. Same normalization as
hard tier (lowercase + strip punctuation).

### 5.7 Rotation policy

When a user has played every variant of a `(passage, language, difficulty,
size)`, the next `startVariant` call cycles back to the oldest variant and
returns it with a `replay: true` flag. Replays are scored and stored
identically; the per-variant leaderboard still discriminates because the
variant is deterministic.

To enforce "no replay until exhausted": when `startVariant` is called without
an explicit `variant_id`, the server looks for the lowest-counter variant the
user hasn't attempted yet. Only if all are attempted does it pick the
oldest-played and set `replay: true`.

---

## 6. Curation pipeline

### 6.1 Frequency table seeding (`just games-seed-frequency`)

One-off command. Tokenizes the full Rashad Khalifa English translation,
normalizes each word (lowercase, strip punctuation), counts occurrences, and
writes to `game_word_frequency` for `language = 'en'`. Idempotent — `TRUNCATE`
then re-insert.

### 6.2 Lemma table seeding (`just games-seed-lemma`)

One-off command. Walks the unique vocabulary collected in step 6.1, runs each
word through a lemmatizer (recommend `wink-lemmatizer` invoked via a small
Node sidecar, or pure-Go `prose` if a Go-native option is preferred), and
writes `(word_normalized, lemma)` rows to `game_lemma`. Idempotent.

> **Recommendation:** if a Go-native lemmatizer is too coarse, run a Python
> sidecar with spaCy once and check the resulting table into a seed fixture.
> The lemmatizer never runs at request time.

### 6.3 GROQ passage curation

#### `just games-curate-chapter CHAPTER`

Sends the full text of one chapter (RK translation) to GROQ and writes the
returned passage proposals into `game_passages` with `status = 'proposed'`,
`source = 'groq'`.

System prompt (verbatim, save in `cmd/groq-curate/prompt.md` or similar):

```
You are curating a Quran reading game using the Rashad Khalifa English
translation. You will be given the full text of one chapter. Propose
"natural reading units" — coherent passages of 3 to 40 verses that form
self-contained reading experiences without cutting mid-clause or
mid-thought. Favor passages where the meaning is intelligible on its own.

ALWAYS use Rashad Khalifa's terminology. Chapter 1 is "The Key", not
"Al-Fatiha". Refer to God as "GOD" (Khalifa's convention) when referring
to the divine in your rationale or labels. Use names and concepts as they
appear in this specific translation.

Return a JSON array. Each item:
{
  "verse_start": <int>,
  "verse_end":   <int>,
  "label":       "<≤ 60 chars, RK terminology>",
  "themes":      ["mercy", "judgement", "prayer", ...],
  "rationale":   "<editor-facing justification, 1-3 sentences>",
  "llm_difficulty":  "easy" | "moderate" | "challenging",
  "llm_blank_hint":  <int, suggested blank count (advisory)>
}

Aim for 6-12 proposals per chapter. Vary lengths so the runtime can serve
short, medium, and long rounds from the same catalog. Do not overlap
ranges; each verse belongs to at most one passage. Avoid passages that
end on a connector ("and", "but", "so that") or begin in the middle of
a clause.
```

The command writes proposals directly to `game_passages`, leaving an editor to
review and flip status to `approved`, `rejected`, or `needs_refinement` via
a sanity tool (probably an admin endpoint or CLI command — out of scope for
v1, can be done with a SQL update).

#### `just games-refine-rejected CHAPTER`

Re-runs GROQ only against passages where `status IN ('rejected',
'needs_refinement')` for the given chapter. Approved passages are untouched.
Generates new proposals to replace the rejected ones, again with `status =
'proposed'`.

#### Future: `--chapters` and `--all` scopes

Both deferred until per-chapter quality is proven.

---

## 7. API endpoints

All endpoints sit at the existing API base. New `/games/*` and `/me/games/*`
prefixes. Every endpoint requires Supabase JWT auth (existing middleware).

OpenAPI fragments below are illustrative; canonical schema lands in
`openapi.yaml` and is generated into Go types via `just generate`.

### 7.1 `GET /games/fill-blank/passages`

List approved passages.

**Query**

| Param | Type | Notes |
| --- | --- | --- |
| `language` | `'en'` (v1) | default `'en'` |
| `theme` | string | optional theme filter |
| `chapter` | int | optional chapter filter (1-114) |

**200**

```json
{
  "data": [
    {
      "id": 42,
      "chapter_start": 1,
      "verse_start": 1,
      "chapter_end": 1,
      "verse_end": 7,
      "label": "The Key — opening prayer",
      "themes": ["worship", "guidance"]
    }
  ]
}
```

`rationale`, `llm_difficulty`, `llm_blank_hint` are NOT returned in this
endpoint (editor-only fields).

### 7.2 `POST /games/fill-blank/variants`

Start a round. Returns either the next unseen variant for the user, or a
specific variant when `variant_id` is supplied (for shared links).

**Body**

```json
{
  "passage_id": 42,
  "language": "en",
  "difficulty": "hard",
  "size": "medium",
  "variant_id": "p42-en-hard-medium-7"  // optional
}
```

Without `variant_id`: server picks the lowest-counter variant the user hasn't
attempted yet, creating a new one if all existing ones are consumed.

With `variant_id`: server returns that exact variant (404 if it doesn't
exist; auto-creation only happens for the no-id case).

**200**

```json
{
  "data": {
    "variant_id": "p42-en-hard-medium-7",
    "passage": {
      "id": 42,
      "chapter_start": 1,
      "verse_start": 1,
      "chapter_end": 1,
      "verse_end": 7,
      "label": "The Key — opening prayer"
    },
    "language": "en",
    "difficulty": "hard",
    "size": "medium",
    "rendered_verses": [
      { "verse_key": "1:2", "text": "__BLANK_0__ be to GOD, Lord of the universe." }
    ],
    "blanks": [
      { "index": 0, "verse_key": "1:2", "word_position": 0 }
    ],
    "replay": false
  }
}
```

For `difficulty = "easy"`, each blank also includes `options: string[]`.
For all other tiers, `options` is omitted.

`answer` and `answer_lemma` are **never** in this response.

### 7.3 `POST /games/fill-blank/submit`

Score a completed variant.

**Body**

```json
{
  "variant_id": "p42-en-hard-medium-7",
  "guesses": [
    { "index": 0, "value": "Praise" }
  ],
  "hints_used": 1,
  "elapsed_ms": 73214
}
```

`elapsed_ms` is optional. Tiebreaker only; not shown in the primary score.

**200**

```json
{
  "data": {
    "attempt_id": "01HBJV...",
    "score": 1485,
    "correct_count": 7,
    "total_count": 8,
    "per_blank": [
      { "index": 0, "correct": true, "accepted_answer": "Praise" }
    ],
    "difficulty_multiplier": 2.0,
    "hint_penalty": 25
  }
}
```

`accepted_answer` is returned ONLY for blanks the user answered correctly.
For incorrect blanks, the field is omitted (the player must replay to see
the right answer; this avoids handing out the variant's solution from a
single bad attempt).

**Rate limiting:** max 10 submissions per user per minute, max 5 submissions
per `(user, variant_id)` per hour. Returns `429` on overflow.

### 7.4 `GET /games/fill-blank/leaderboard`

**Query**

| Param | Type | Notes |
| --- | --- | --- |
| `scope` | `'global' \| 'weekly' \| 'variant'` | required |
| `size` | `'short' \| 'medium' \| 'long'` | required for global/weekly |
| `difficulty` | difficulty | required for global/weekly |
| `variant_id` | string | required when `scope = 'variant'` |
| `limit` | int | default 25, max 100 |

**200**

```json
{
  "data": [
    {
      "rank": 1,
      "user_id": "01H...",
      "display_name": "anon-1234",
      "score": 2940,
      "size": "medium",
      "difficulty": "professional",
      "attempted_at": "2026-05-21T14:32:00Z"
    }
  ]
}
```

Display name resolution falls back to `user_id`-derived anonymous handle
when the user has no profile name set.

### 7.5 `GET /me/games/history`

Returns the current user's completed attempts, newest first.

**Query**

| Param | Type | Notes |
| --- | --- | --- |
| `limit` | int | default 25, max 100 |
| `cursor` | string | opaque pagination cursor (encoded `completed_at`) |

**200**

```json
{
  "data": [
    {
      "attempt_id": "01HBJV...",
      "variant_id": "p42-en-hard-medium-7",
      "score": 1485,
      "size": "medium",
      "difficulty": "hard",
      "completed_at": "2026-05-26T13:00:00Z"
    }
  ],
  "next_cursor": "01HBJV..."
}
```

### 7.6 `GET /me/games/stats`

```json
{
  "data": {
    "total_attempts": 42,
    "best_score": 2840,
    "variants_completed": 30,
    "by_difficulty": {
      "easy":         { "attempts": 5,  "best_score": 980 },
      "medium":       { "attempts": 12, "best_score": 1450 },
      "hard":         { "attempts": 18, "best_score": 1900 },
      "professional": { "attempts": 7,  "best_score": 2840 }
    }
  }
}
```

---

## 8. Score formula

```
base                  = (correct_blanks / total_blanks) * 1000
difficulty_multiplier = { easy: 1.0, medium: 1.5, hard: 2.0, professional: 3.0 }[difficulty]
hint_penalty          = hints_used * 25
score                 = max(0, round(base * difficulty_multiplier - hint_penalty))
```

`elapsed_ms` is stored but NOT included in `score`. It is used only as a
tiebreaker in leaderboard ordering (lower elapsed wins ties).

Worked examples:

| difficulty | correct/total | hints | elapsed | base | mult | penalty | score |
| --- | --- | --- | --- | --- | --- | --- | --- |
| easy        | 8/8 | 0 | 60s   | 1000.0 | 1.0 | 0  | 1000 |
| medium      | 14/15 | 2 | 180s | 933.3  | 1.5 | 50 | 1350 |
| hard        | 7/8 | 0 | 240s   | 875.0  | 2.0 | 0  | 1750 |
| professional| 25/25 | 0 | 600s | 1000.0 | 3.0 | 0  | 3000 |

---

## 9. Anti-cheat

### 9.1 Answer storage

`game_variants.blanks[*].answer` is stored as **plaintext** on the server. It
is never sent to the client. DB access is assumed to be controlled; if that
assumption breaks we switch to per-variant salted hashes (`sha256(answer +
variant_salt)`).

### 9.2 Validation flow

1. Client sends `{ variant_id, guesses, hints_used }`.
2. Server loads the variant, iterates `blanks[]`, evaluates each guess using
   the difficulty's acceptance rule (exact / lemma / etc.).
3. Server computes the score from the truth, never trusting any client-side
   score calculation.
4. Server writes a new `game_attempts` row and returns the result.

### 9.3 Rate limiting

- 10 submissions / minute / user (global)
- 5 submissions / hour / (user, variant_id)

Both enforced at the handler layer. Returns `429 Too Many Requests` with a
plain-text `Retry-After` header.

### 9.4 Leaderboard integrity

`score` is server-computed. `elapsed_ms` is client-supplied and used only as
a tiebreaker — a malicious client can claim 1ms but cannot improve the
primary score. We accept this trade-off in v1.

---

## 10. Open questions (deferred from v1)

- **Arabic mode (v2).** Same architecture; new blankability scorer using root
  data already in the corpus; diacritic normalization on submit; separate
  leaderboard namespace (`language = 'ar'`).
- **Editorial UI for passage approval.** v1 uses raw SQL or a simple admin
  endpoint to flip `status`. A proper review UI is a follow-up.
- **Anonymous play.** Not in v1 (auth is required to even see the games
  section). May relax later for player acquisition.
- **Sharing.** Variant URLs are deterministic and shareable today; adding
  Open Graph cards and an explicit "challenge a friend" CTA is a v2 polish.
- **Adaptive difficulty.** Use user history to surface variants matched to
  skill. Out of scope for v1.

---

## 11. Implementation checklist (suggested order)

1. **Migration** — `db/migrations/NNNN_games.sql` with all five tables.
2. **OpenAPI fragments** — add to `openapi.yaml`, run `just generate`.
3. **Seed commands** — `just games-seed-frequency`, `just games-seed-lemma`.
   Tests confirm tables non-empty and key Quran words present.
4. **Handlers (skeletons)** — register routes, return 501 for now.
5. **Variant generator** — implement `generate(...)` with table-driven tests
   covering seed determinism, blank-selection rules per difficulty, distractor
   selection, round-size targets.
6. **GROQ command** — `just games-curate-chapter CHAPTER`. Smoke-test on
   chapter 1 ("The Key"). Iterate prompt until 6-12 proposals come back with
   useful labels and themes.
7. **Approve chapter 1** manually (set status to 'approved'). Wire the
   passage list endpoint and verify the frontend lobby renders it.
8. **Start-variant + submit + score** — close the round loop end-to-end.
9. **Leaderboards** — global / weekly / variant queries with the indexes
   already declared in §3.3.
10. **History + stats** — `/me/games/history`, `/me/games/stats`.
11. **Rate limiting** — middleware over the games handlers.
12. **Bulk curation** — once chapter 1 quality looks right, run further
    chapters one at a time. Promote `--chapters` and `--all` scopes only
    after a half-dozen chapters look clean.

---

## 12. Frontend contract reference

The frontend already declares the types it expects in
`wikisubmission-org/src/api/me-client.ts` under the `Game*` types. Notable
shapes:

- `GameVariant.rendered_verses[*].text` contains `__BLANK_<index>__` tokens
- `GameBlank.options` present only on `difficulty = 'easy'`
- `GameSubmitResult.per_blank[*].accepted_answer` present only on correct
  blanks
- `GameLeaderboardScope` is `'global' | 'weekly' | 'variant'`

When this spec ships, regenerate the frontend OpenAPI types via `npm run
sync-api && npm run generate` and replace the throwing stubs in `meApi.games`
with real typed `wsApi.GET/POST` calls.
