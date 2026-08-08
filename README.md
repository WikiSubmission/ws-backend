# ws-backend

WikiSubmission Quran Backend — a Go REST API for multilingual Quran reading and a translation editorial workflow. Spec-first design with OpenAPI 3.0 and strict code generation.

## Architecture

Single Go module (`github.com/wikiSubmission/ws-backend`) with three packages:

| Package | Purpose |
| --- | --- |
| `api/` | Gin HTTP server, OpenAPI handlers, route registration |
| `db/` | PostgreSQL layer (pgx/v5), migrations, row structs |
| `auth/` | Auth.js (HS256) JWT verification and route guards |

**Request flow:**

```text
api/openapi.yaml → oapi-codegen → api/handlers/api.gen.go
                                         ↓
api/server.go (Gin + OpenAPI validator) → handlers/Server.method() → db.DB.Query()
```

## Prerequisites

- Go 1.26+
- PostgreSQL 18 (with `vector` and `pg_trgm` extensions)
- `oapi-codegen` (installed as a Go tool — `go tool oapi-codegen`)
- [`just`](https://github.com/casey/just) (replaces the previous Makefile)

## Environment Variables

Copy `.env.example` to `.env` and fill in the values:

| Variable | Purpose | Default |
| --- | --- | --- |
| `APP_ENV` | `production` or `development` | `development` |
| `PORT` | HTTP listen port | `8080` |
| `TRUSTED_PROXIES` | Comma-separated proxy IPs | *(none)* |
| `DATABASE_USER` | PostgreSQL user | — |
| `DATABASE_PASSWORD` | PostgreSQL password | — |
| `DATABASE_DOMAIN` | PostgreSQL host | — |
| `DATABASE_PORT` | PostgreSQL port | `5432` |
| `DATABASE_NAME` | PostgreSQL database name | — |
| `DATABASE_SSL_MODE` | `disable`, `require`, `verify-full` | — |
| `WS_BACKEND_JWT_SECRET` | Shared secret used to verify HS256 JWTs minted by the Auth.js frontend layer. When unset, the verifier is disabled and authenticated routes reject. | — |
| `VAPID_PUBLIC_KEY` | Web Push VAPID public key used by `/me/push/test`. Should correspond to `NEXT_PUBLIC_VAPID_PUBLIC_KEY` in the frontend. | *(unset = push send disabled)* |
| `VAPID_PRIVATE_KEY` | Web Push VAPID private key used to sign push notifications. | *(unset = push send disabled)* |
| `VAPID_SUBJECT` | Web Push VAPID subject URI. | `mailto:admin@wikisubmission.org` |
| `WSLIB_BASE_URL` | Internal ws-lib base URL used to store user data exports. | *(unset = export upload disabled)* |
| `STORE_AUTH_SECRET` | HMAC secret shared with ws-lib `/private/store` for user data exports. | *(unset = export upload disabled)* |
| `CORS_ALLOWED_ORIGINS` | Comma-separated allowed browser origins for authenticated API calls | `*` |

## First-Time Setup

```bash
# 1. Create role + database + extensions on local PG 18
just db-setup

# 2. Pull the latest snapshot from Railway and load it locally
just export railway   # writes db/dumps/<ts>__railway.dump
just import           # restores the most recent dump into local

# 3. Boot the API
just run
```

The migration runner (`db/migrations/001_baseline.sql` + `002_data_fixes.sql`) brings the schema up automatically on startup. `just import` only loads data.

## Refreshing local from Railway

```bash
just export       # snapshot Railway → db/dumps/
just import       # latest dump → local (no prompt)
just verify railway local
```

## Promoting a snapshot to a remote (Railway → Hetzner cutover)

```bash
just export railway       # capture truth
just push hetzner         # restore into Hetzner (prompts for confirmation)
just verify railway hetzner
```

The `push` recipe is target-agnostic: any `DATABASE_URL_<NAME>` set in `.env` is a valid target.

## Development

```bash
just run     # start the API (development mode, pretty slog)
just test    # all tests (DB tests need DATABASE_NAME)
just tidy    # tidy Go module
```

## Building

```bash
just build   # static binary at bin/server
```

## Code Generation

```bash
just generate
```

> **Never hand-edit `api/handlers/api.gen.go`** — it is always overwritten on regeneration.

## Docker

```bash
just docker-build
just docker-run    # reads .env for DB config
```

The production image uses `gcr.io/distroless/static-debian12:nonroot` — no shell, minimal attack surface. Database migrations are embedded in the binary at build time.

## API Endpoints

## Authentication

Authenticated frontend calls carry an HS256 JWT minted by the Auth.js (NextAuth v5) frontend layer as a bearer token. The backend verifies the signature with the shared `WS_BACKEND_JWT_SECRET`, then uses the namespaced subject claim (e.g. `google:12345`, `email:user@x.com`) as the canonical `auth_id`.

Protected endpoints currently include:

- `POST /api/v1/users`
- `GET /api/v1/users/:id`
- `GET /api/v1/users`
- `PATCH /api/v1/users/:id`

`POST /api/v1/users` accepts first-login upserts from the Next.js app. For SSO flows such as Google and Apple, the backend will fall back to the JWT email claim when the frontend omits `email` from the sync payload.

### `GET /api/v1/languages`

Returns all supported languages with their ID, code, name, and text direction. Cached for 24 hours (`Cache-Control: public, max-age=86400`).

No parameters. Response:

```json
[
  { "id": 1, "code": "en", "name": "English", "direction": "ltr" },
  { "id": 2, "code": "ar", "name": "Arabic",  "direction": "rtl" }
]
```

### `GET /api/v1/chapters`

Returns chapter metadata (number, verse count, revelation order, title) for all 114 chapters in the requested language.

| Parameter | Type | Required | Description |
| --- | --- | --- | --- |
| `lang` | `string` | No | Language code (default `en`) |

### `GET /api/v1/appendices`

Returns appendix entries (code, title, snippet) in the requested language.

| Parameter | Type | Required | Description |
| --- | --- | --- | --- |
| `lang` | `string` | No | Language code (default `en`) |

Both endpoints validate the `lang` code against the in-memory language cache and return `400` for unsupported languages.

### `GET /api/v1/quran`

Fetch verses with translations and (optionally) word-by-word data.

| Parameter | Type | Required | Description |
| --- | --- | --- | --- |
| `langs` | `[]string` | Yes | Language codes (e.g. `en`, `ar`) |
| `chapter_number_start` | `integer` | Yes | Start chapter (1–114) |
| `chapter_number_end` | `integer` | No | End chapter (1–114) |
| `verse_start` | `integer` | No | Start verse index |
| `verse_end` | `integer` | No | End verse index |
| `include_words` | `boolean` | No | Include word-by-word data |
| `word_langs` | `[]string` | No | Language codes for word translations |
| `include_root` | `boolean` | No | Include Arabic root in word data |
| `include_meaning` | `boolean` | No | Include word meaning |

Max range: 20 chapters per request.

### `GET /api/v1/search`

Full-text search across all verse, word, and chapter title content. Also supports exact Arabic root lookup.

| Parameter | Type | Required | Description |
| --- | --- | --- | --- |
| `q` | `string` | Conditional | Search query (2–200 chars). Required unless `root` is provided. |
| `root` | `string` | No | Arabic root (e.g. `رحم`). When set with `scope=words`, bypasses FTS and does exact `word_root` matching. Takes priority over `q`. |
| `langs` | `[]string` | No | Restrict to specific languages |
| `scope` | `string` | No | `verses` (default) or `words` |
| `limit` | `integer` | No | Max results (default 20, max 100) |
| `offset` | `integer` | No | Pagination offset |
| `include_words` | `boolean` | No | Include word-by-word data |
| `word_langs` | `[]string` | No | Language codes for word data |
| `include_root` | `boolean` | No | Include Arabic root in word results |
| `include_meaning` | `boolean` | No | Include word meaning |

**FTS mode** (`q` provided): results include relevance scores (`sc`), highlighted snippets (`hl` with `<b>` tags), and a title-match flag (`tm`) when the chapter name matches.

**Root mode** (`root` + `scope=words`): returns every occurrence of the root across all verses. `sc` is `1.0` for all results (no ranking). `hl` contains the plain word text.

### `GET /api/v1/communities`

Sanity-backed directory of online and physical communities. Active entries
only; results are cached server-side and invalidated by webhook on document
changes.

| Parameter | Type | Required | Description |
| --- | --- | --- | --- |
| `kind` | `string` | No | `online` or `physical` |
| `language` | `string` | No | ISO 639-1 language code |
| `country` | `string` | No | ISO 3166-1 alpha-2 country code (physical only) |

Served from the first-party editorial store (`editorial_communities`);
only published entries are returned.

### Utility Endpoints

| Endpoint | Description |
| --- | --- |
| `GET /health` | Deep health check (verifies DB connectivity) |
| `GET /ping` | Liveness check |
| `GET /robots.txt` | Crawler directives |
| `GET /sitemap.xml` | Site map for search engines |
| `GET /metrics` | Prometheus metrics |

## Response Format

JSON keys are minified for bandwidth efficiency:

| Field | Key | Field | Key |
| --- | --- | --- | --- |
| `chapter_number` | `cn` | `translations` | `tr` |
| `verse_index` | `vi` | `words` | `w` |
| `verse_key` | `vk` | `word_index` | `wi` |
| `text/content` | `tx` | `global_index` | `gi` |
| `lang_id` | `li` | `subtitle` | `s` |
| `lang_code` | `lc` | `footer` | `f` |
| `direction` | `d` | `root` | `r` |
| `highlight` | `hl` | `meaning` | `m` |
| `score` | `sc` | `title_match` | `tm` |

## Testing

```bash
just test
# DB integration tests need a live database:
DATABASE_NAME=your_db go test ./db/...
```

## Justfile reference

Run `just` (no args) for the full list. Key recipes:

| Recipe | Description |
| --- | --- |
| `just run` | Start API in development mode |
| `just build` | Compile static binary to `bin/server` |
| `just test` | Run all tests |
| `just tidy` | Tidy Go module dependencies |
| `just generate` | Regenerate OpenAPI types from `openapi.yaml` |
| `just db-setup` | Create local role + database + extensions |
| `just db-drop` | Drop local role + database |
| `just rebuild` | Drop → setup → legacy import → seed |
| `just export [SOURCE]` | Snapshot a remote (default: `railway`) into `db/dumps/` |
| `just import [VERSION]` | Restore latest (or named) dump into local |
| `just push TARGET [VERSION]` | Restore a dump into a remote target (`railway`, `hetzner`, …) |
| `just dumps` | List dumps in `db/dumps/`, newest first |
| `just verify A B` | Diff row counts between two targets |
| `just embed-quran` | Generate `verse_text` embeddings via OpenAI |
| `just export-embeddings [SOURCE]` | Snapshot just the embedding column |
| `just push-embeddings TARGET` | Upload latest embeddings CSV to a target |
| `just source-import [TARGET]` | Load legacy Supabase dumps |
| `just seed [TARGET]` | Legacy → normalized seed transformation |
| `just seed-bible` | Seed Bible CSVs |
| `just seed-music [TARGET]` | Seed music tables |
| `just seed-admin EMAIL` | Promote a user to admin |
| `just refresh-roots [TARGET]` | Refresh `root_index_mv` |
| `just coolify-init` | Bootstrap a shared Postgres (Coolify/Hetzner) via `init.sql` |
| `just docker-build` / `just docker-run` | Production image lifecycle |
| `just clean` | Remove `bin/` + Go build cache |

## Database migrations

Schema lives in two SQL files, both embedded into the binary and applied at startup (alphabetical, idempotent):

- `db/migrations/001_baseline.sql` — squashed from prior 13 migrations; baseline schema.
- `db/migrations/002_data_fixes.sql` — production data corrections (Arabic glyphs, 14:40 EN, 105:3 transliteration). Add new fixes here.

Snapshots (created by `just export`) are **data-only** — schema always comes from the migration runner.
