# ws-backend

WikiSubmission Quran Backend — a Go REST API for multilingual Quran reading and a translation editorial workflow. Spec-first design with OpenAPI 3.0 and strict code generation.

## Architecture

Single Go module (`github.com/wikiSubmission/ws-backend`) with three packages:

| Package | Purpose |
| --- | --- |
| `api/` | Gin HTTP server, OpenAPI handlers, route registration |
| `db/` | PostgreSQL layer (pgx/v5), migrations, row structs |
| `auth/` | Supabase JWT verification and route guards |

**Request flow:**

```text
api/openapi.yaml → oapi-codegen → api/handlers/api.gen.go
                                         ↓
api/server.go (Gin + OpenAPI validator) → handlers/Server.method() → db.DB.Query()
```

## Prerequisites

- Go 1.24+
- PostgreSQL 14+
- `oapi-codegen` (installed as a Go tool — `go tool oapi-codegen`)

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
| `SUPABASE_JWKS_URL` | Supabase project JWKS endpoint used to verify bearer tokens | — |
| `CORS_ALLOWED_ORIGINS` | Comma-separated allowed browser origins for authenticated API calls | `*` |

## First-Time Setup

```bash
# 1. Create the PostgreSQL user and database
make db-setup

# 2. Load raw source data (Quran text dumps)
make source-import

# 3. Run the data migration seed (transforms legacy schema → app schema)
make seed

# 4. Remove temporary source tables and asset files
make source-cleanup
```

Or run everything at once (drop → setup → import → seed → cleanup):

```bash
make db-rebuild
```

## Development

```bash
# Start the API server with pretty slog output
make run

# Run all tests (DB tests require DATABASE_NAME to be set)
make test

# Tidy dependencies
make tidy
```

## Building

```bash
# Compile a static binary to bin/server
make build
```

## Code Generation

The API types and server stubs are generated from `api/openapi.yaml`. Run from the **repo root**:

```bash
make generate
```

> **Never hand-edit `api/handlers/api.gen.go`** — it is always overwritten on regeneration.

## Docker

```bash
# Build the production image
make docker-build

# Run locally (reads .env for DB config)
make docker-run
```

The production image uses `gcr.io/distroless/static-debian12:nonroot` — no shell, minimal attack surface. Database migrations are embedded in the binary at build time.

## API Endpoints

## Authentication

Authenticated frontend calls use Supabase access tokens as bearer tokens. The backend verifies those JWTs against `SUPABASE_JWKS_URL`, then uses the token subject as the canonical `supabase_uid`.

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
# All tests (unit + integration)
make test

# DB integration tests require a live database:
DATABASE_NAME=your_db go test ./db/...
```

## Makefile Reference

| Target | Description |
| --- | --- |
| `make run` | Start API in development mode |
| `make build` | Compile static binary to `bin/server` |
| `make test` | Run all tests |
| `make tidy` | Tidy Go module dependencies |
| `make generate` | Regenerate OpenAPI types from `api/openapi.yaml` |
| `make seed` | Run one-time data migration from legacy schema |
| `make db-setup` | Create PostgreSQL user, database, and permissions |
| `make db-drop` | Remove the database and user entirely |
| `make db-rebuild` | Full rebuild: drop → setup → import → seed → cleanup |
| `make source-import` | Load raw Supabase dumps into the database |
| `make source-cleanup` | Drop temporary source tables and remove asset files |
| `make seed-railway` | First-time seed of the Railway database (requires `DATABASE_URL` in `.env`) |
| `make reseed-railway` | Re-seed Railway without dropping tables (truncates data, preserves schema) |
| `make docker-build` | Build the production Docker image |
| `make docker-run` | Build and run the container locally |
| `make clean` | Remove compiled binaries and Go build cache |
