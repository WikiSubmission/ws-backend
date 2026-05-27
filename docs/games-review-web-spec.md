# Quran Games — Editorial Review Web Interface (hand-off spec)

> Status: ready to implement. This document is a self-contained brief for an
> engineer (or agent) to build a simple web UI that replaces the
> `api/cmd/games_review` CLI. Read it top to bottom; every file path and
> signature below is real and current.

## 1. Goal

Give a non-technical editor a browser page to:

1. See passages GROQ proposed for the games catalog.
2. Approve, reject, or mark "needs refinement" with one click.
3. (Optional, nice-to-have) trigger GROQ curation for a chapter from the page.

Access is restricted to a small allowlist of accounts identified by email,
configured through an environment variable. Sign-in uses the Google / Apple
login that the public site already supports.

This replaces the CLI in `api/cmd/games_review` for day-to-day use. The CLI can
stay as a fallback.

## 2. What already exists (do not rebuild)

### Backend (`ws-backend`, Go + Gin)

- Editorial DB methods are already implemented in `db/games.go`:
  - `FetchPassagesForReview(ctx, status *string, chapter *int) ([]GamePassageRow, error)`
  - `FetchPassageByID(ctx, id int64) (*GamePassageRow, error)`
  - `SetPassageStatus(ctx, id int64, status string, editorEmail *string) (bool, error)`
    - When `status == "approved"`: stamps `approved_at = now()` and resolves
      `approved_by` from the editor email. Other statuses clear both fields.
- Auth middleware in `auth/middleware.go`:
  - `RequireAuth(verifier)` validates the Auth.js bearer JWT and sets
    `ContextKeyUID` and `ContextKeyEmail` on the Gin context.
  - `RequireAdmin(database)` is the existing role-based gate (DB `users.role`).
- Admin routes are registered manually in `api/server.go` (search for
  `// ── Admin routes`), OUTSIDE the OpenAPI surface. Follow this same pattern
  for the new editor routes.
- GROQ curation logic lives in `api/cmd/groq_curate/main.go`:
  - `proposePassages(ctx, baseURL, apiKey, model string, chapter int, chapterText string) ([]proposal, error)`
  - `validateProposals(props []proposal) (kept []proposal, dropped []droppedProposal)`
  - These are in `package main` today. To call them from an HTTP handler they
    must be extracted into an importable package (see Task B3).

### Frontend (`org`, Next.js + NextAuth at `/home/hichem/Projects/ws/org`)

- NextAuth is configured in `auth.ts` with Google, Apple, Discord, and an OTP
  Credentials provider. Session strategy is JWT; `session.accessToken` holds
  the backend-verifiable token.
- `src/api/client.ts` exposes `wsApi`, a typed `openapi-fetch` client that
  attaches `Authorization: Bearer <session.accessToken>` automatically.
- Server-side calls use `src/api/server-client.ts` / `me-server-client.ts`,
  which read the token and call the backend with `INTERNAL_API_URL`.
- Existing authenticated `/me/*` pages under `app/` are the pattern to copy.

## 3. Architecture decision

Build the UI as a protected route inside the existing `org` Next.js app, not as
a separate service.

Rationale: Google / Apple sign-in, session handling, the typed API client, and
deployment all already exist there. A standalone app would duplicate all of it.
The editorial route is simply not linked from public navigation.

The security boundary is the backend allowlist middleware (Task B1). The
frontend guard (Task F1) is for UX only (redirect non-editors); never rely on
it for security.

## 4. Auth and gating design

Add one new env var, read by the backend:

```
GAMES_EDITOR_EMAILS=editor@example.com,hesham@wikisubmission.org
```

Comma-separated, case-insensitive, trimmed. Empty or unset means no one is an
editor (endpoints return 403). This is the single source of truth for access.

Flow:

1. Editor signs in on the site with Google or Apple (existing flow).
2. The page calls the new editor endpoints; `wsApi` attaches the bearer token.
3. Backend `RequireAuth` validates the JWT and puts the email on the context.
4. New `RequireEditor` middleware checks that email against
   `GAMES_EDITOR_EMAILS`. Not on the list → `403`.

No new OAuth setup, no DB role changes. The allowlist is intentionally separate
from the `users.role = admin` mechanism so granting editorial access does not
grant broader admin rights.

## 5. Backend tasks

### B1. `RequireEditor` middleware

Add to `auth/middleware.go`:

```go
// RequireEditor allows only emails present in GAMES_EDITOR_EMAILS (comma-
// separated, case-insensitive). Must run after RequireAuth.
func RequireEditor() gin.HandlerFunc
```

- Read and parse `GAMES_EDITOR_EMAILS` once at construction (pass the parsed
  set in, or build a small constructor `NewEditorGuard(emails string)`), not on
  every request.
- Read the email from `c.GetString(ContextKeyEmail)`.
- Empty allowlist or non-member → `c.AbortWithStatusJSON(403, gin.H{"error": "editor access required"})`.
- Reuse `observability.AuthAttemptsTotal` labels if it is cheap to do so;
  otherwise skip metrics.

### B2. Editor HTTP handlers

Create `api/handlers/games_review_http.go` (plain Gin handlers, NOT strict /
OpenAPI handlers, mirroring how `api/users.go` admin handlers are written). The
`Server` already holds `*db.DB`.

Endpoints (all under `/api/v1/admin/games`):

| Method | Path | Body / Query | Calls |
| --- | --- | --- | --- |
| GET | `/passages` | `?status=&chapter=` | `FetchPassagesForReview` |
| GET | `/passages/:id` | — | `FetchPassageByID` |
| POST | `/passages/:id/status` | `{ "status": "approved"\|"rejected"\|"needs_refinement" }` | `SetPassageStatus` (editor email from `ContextKeyEmail`) |
| POST | `/curate` | `{ "chapter": 1, "refine": false }` | curation package (Task B3) — optional |

Response shape: reuse the existing envelope convention (`{ "data": ... }`). For
the list, return the full editorial fields (label, themes, rationale,
llm_difficulty, llm_blank_hint, status, verse range, approved_at/by). These are
editor-only, so it is fine that they are absent from the public
`GET /games/fill-blank/passages`.

Validation:

- `status` must be one of the three allowed values → else `400`.
- `:id` must parse as int64 → else `400`.
- `SetPassageStatus` returning `false` → `404`.

### B3. Extract curation core (only if building the `/curate` endpoint)

Move `proposal`, `proposalEnvelope`, `proposePassages`, `validateProposals`,
and `normalizeDifficulty` out of `api/cmd/groq_curate/main.go` into a new
importable package, e.g. `api/games/curate/curate.go`. Have both the CLI
(`cmd/groq_curate`) and the new endpoint import it. Keep the embedded
`prompt.md` with the package.

Curation calls an external API and writes proposals; it usually finishes within
a few seconds but can approach the 60s client timeout. Run it synchronously for
v1 and surface failures as `502`. If it proves slow, make it a background job
later (out of scope here).

### B4. Register routes

In `api/server.go`, next to the existing admin group, add:

```go
editor := r.Group("/api/v1/admin/games")
editor.Use(auth.RequireAuth(verifier))
editor.Use(auth.RequireEditor()) // or NewEditorGuard(os.Getenv("GAMES_EDITOR_EMAILS"))
// register the B2 handlers on `editor`
```

These routes sit outside the OpenAPI validator (like the current `/users`
admin routes), so no `oapi-codegen` regeneration is needed.

## 6. Frontend tasks

### F1. Protected route

Add `app/studio/games/page.tsx` (server component). Pattern after an existing
authenticated page under `app/`.

- Call `auth()` to get the session. If no session, redirect to sign-in.
- Soft editor check: compare `session.user.email` against an allowlist exposed
  to the Next server (reuse the same `GAMES_EDITOR_EMAILS` value, set in the
  org app env too). If not an editor, render a "not authorized" message. This
  is UX only; the backend enforces the real gate.
- Do not link this route from public navigation.

### F2. API calls

Because the editor endpoints are deliberately outside the OpenAPI contract,
`wsApi` (typed) will not know them. Use one of:

- Server actions / route handlers that call the backend with the session token
  via the `server-client.ts` token helper (recommended; keeps it server-side),
  or
- A thin client that uses `fetch` with the bearer token, similar to
  `me-client.ts`.

Endpoints to wire: list passages (with status / chapter filters), set status,
and optionally curate.

### F3. UI

Keep it minimal. One page:

- A status filter (default `proposed`) and optional chapter filter.
- A table or card list of passages: verse range, label, themes, llm_difficulty,
  llm_blank_hint, rationale, current status.
- Per row: Approve / Reject / Needs refinement buttons that POST the status and
  optimistically update (or refetch).
- (Optional) a "Curate chapter N" input + button calling `/curate`.

No design system work required beyond what the org app already provides.

## 7. Environment variables (new)

| Var | Where | Purpose |
| --- | --- | --- |
| `GAMES_EDITOR_EMAILS` | backend (+ org app for the soft check) | comma-separated editor allowlist |
| `GROQ_API_KEY` | backend | already used by the curate CLI; needed only if `/curate` endpoint is built |
| `GROQ_MODEL`, `GROQ_BASE_URL` | backend | optional overrides, defaults in `cmd/groq_curate` |

## 8. Non-goals (do not build now)

- Editing passage text / verse ranges in the UI. Approve/reject only.
- Bulk approve. One passage at a time is fine for v1.
- Frequency / lemma seeding from the UI. Those stay as CLI jobs.
- Replacing the role-based `RequireAdmin` mechanism. The editor allowlist is
  additive and separate.

## 9. Acceptance criteria

1. A signed-in Google or Apple account on `GAMES_EDITOR_EMAILS` can open the
   page, see proposed passages, and approve one.
2. After approval, `GET /games/fill-blank/passages` returns that passage, and
   `games_review --id <id>` shows `status=approved` with `approved_by` set.
3. A signed-in account NOT on the allowlist gets `403` from the endpoints and a
   "not authorized" page.
4. An unauthenticated request to any `/api/v1/admin/games/*` endpoint gets
   `401`.
5. Backend `go build ./...`, `go vet ./...`, and `go test ./...` stay green.
   Add a unit test for the email-allowlist parser/matcher in `auth`.

## 10. Suggested commit breakdown

1. `feat(auth): editor email allowlist guard` — B1 + allowlist parser + test.
2. `feat(games): admin review endpoints` — B2, B4, (B3 if doing curate).
3. Frontend PR in `org`: protected studio route + review UI (F1–F3).

## 11. Reference: current CLI behaviour to mirror

```
just games-review                          # list proposed
just games-review --status all
just games-review --id 42                  # detail
just games-review --id 42 --approve --by editor@example.com
just games-review --id 42 --reject
just games-review --id 42 --needs-refinement
```

The web UI should cover list + detail + the three status transitions. The `--by`
editor email is taken from the signed-in session, not typed by hand.
