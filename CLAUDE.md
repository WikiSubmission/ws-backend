# CLAUDE.md — ws-backend

## Project Overview

WikiSubmission Quran Backend — a Go monorepo API for multilingual Quran reading and a translation editorial workflow. Spec-first design with OpenAPI 3.0 and strict code generation.

## Architecture

### Go Workspace (`go.work`)

Three modules, all on Go 1.26:

- `api/` — Gin HTTP server, OpenAPI handlers, route registration
- `db/` — PostgreSQL layer (pgx/v5), migrations, row structs
- `auth/` — Authentication module (stub, not yet implemented)

### Request Flow

```graph
openapi.yaml → oapi-codegen → api/handlers/api.gen.go
                                      ↓
api/server.go (Gin + OAP validator) → handlers/Server.method() → db.DB.Query()
```

## Key Technologies

| Concern | Library |
|---|---|
| HTTP | `gin-gonic/gin` |
| OpenAPI validation | `oapi-codegen/gin-middleware` |
| Code generation | `oapi-codegen/oapi-codegen/v2` |
| Database | `jackc/pgx/v5` (pgxpool) |
| Logging | `log/slog` + `lmittmann/tint` (dev) / JSON (prod) |
| Compression | `gin-contrib/gzip` |
| Metrics | `zsais/go-gin-prometheus` |
| Env | `joho/godotenv` + direnv (`.envrc`) |

## Code Generation

### Regenerate API types and server stubs

```bash
cd api
go generate ./...
# or directly:
go tool oapi-codegen -config oapi-codegen.yaml openapi.yaml
```

Config (`api/oapi-codegen.yaml`) generates: strict server, gin server, models, embedded spec → `api/handlers/api.gen.go`.

**Never hand-edit `api/handlers/api.gen.go`** — it is always overwritten on regeneration.

## Database Conventions

### Connection

All config comes from env vars: `DATABASE_USER`, `DATABASE_PASSWORD`, `DATABASE_DOMAIN`, `DATABASE_PORT`, `DATABASE_NAME`, `DATABASE_SSL_MODE`.

### Migrations

- Files live in `db/migrations/*.sql`, embedded via `//go:embed`
- Sorted alphabetically; use numeric prefixes (`01_`, `02_`, …)
- Must be idempotent — use `CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`, etc.
- Run automatically at startup via `db.DbSetup()`

### Queries

- Use `pgx/v5` directly — no ORM
- Row structs in `db/` use `db:"column_name"` struct tags
- Scan with `pgx.CollectRows(rows, pgx.RowToStructByName[RowType])`
- Nullable columns use pointer types (`*string`, `*int`)
- Batch inserts use `pgx.Batch` + `Pool.SendBatch()`

## Handler Conventions

### Server struct (`api/handlers/main.go`)

```go
type Server struct {
    DB        *db.DB
    Languages map[string]LanguageCache  // keyed by lang code, e.g. "en"
    Chapters  map[int]ChapterCache      // keyed by chapter number
}
```

Caches are populated at startup by `InitializeCaches()` — no live DB calls for metadata.

### Adding a new endpoint

1. Add the path/operation to `api/openapi.yaml`
2. Regenerate: `go tool oapi-codegen -config oapi-codegen.yaml openapi.yaml`
3. Implement the generated interface method in `api/handlers/`
4. The strict handler signature is always:

   ```go
   func (s *Server) OperationName(ctx context.Context, request OperationNameRequestObject) (OperationNameResponseObject, error)
   ```

5. Return typed response objects (e.g. `GetQuran200JSONResponse{...}`, `GetQuran400JSONResponse{...}`)

### Unimplemented endpoints

Return `501` with `NotImplementedErrorJSONResponse{Message: "...", Code: "NOT_IMPLEMENTED"}`.

## Response Schema Conventions

JSON keys are intentionally minified (bandwidth optimisation):

| Field | Key |
| --- | --- |
| chapter_number | `cn` |
| verse_index | `vi` |
| verse_key | `vk` |
| translations | `tr` |
| words | `w` |
| word_index | `wi` |
| global_index | `gi` |
| text/content | `tx` |
| lang_id | `li` |
| lang_code | `lc` |
| direction | `d` |
| subtitle | `s` |
| footer | `f` |
| root | `r` |
| meaning | `m` |

## Logging

- Always use `slog` (not `fmt.Println` except in seed scripts)
- Structured fields: `slog.Info("msg", "key", value)` or `slog.Any("error", err)`
- Dev: colored output via `tint`; Prod (`APP_ENV=production`): JSON to stdout

## Environment Variables

| Variable | Purpose |
| --- | --- |
| `APP_ENV` | `production` or `development` |
| `PORT` | HTTP listen port (default `8080`) |
| `TRUSTED_PROXIES` | Comma-separated proxy IPs |
| `DATABASE_*` | See DB section above |
| `GOPRIVATE` | Set to `github.com/wikiSubmission/*` (via `.envrc`) |

## Running Locally

```bash
# From repo root (uses go.work)
cd api && go run .

# Seed the database (one-time migration from legacy schema)
cd api && go run ./cmd/seed.go
```

## Testing

```bash
cd db && go test ./...
```

DB tests (`db/db_test.go`) require a live database connection.

## Module Boundaries

- `db` must never import `api` or `auth`
- `api` imports `db`; do not put HTTP logic in `db`
- Keep DB row structs in `db/`; keep API response types in `api/handlers/` (generated)
