# Quran Games — Editorial Review Web Interface (hand-off spec)

> Status: BACKEND IMPLEMENTED — frontend remaining. The Go endpoints,
> middleware, and maintenance actions described below are merged and live. The
> remaining work is the frontend in the `org` Next.js app (section 6). Every
> file path and signature is real and current.

## 1. Goal

Give a non-technical editor a browser page to:

1. See passages GROQ proposed for the games catalog.
2. Approve, reject, or mark "needs refinement" with one click.
3. Run the one-time data activation (word frequencies, lemma data) from a
   button, since prod has no shell / `just` / `psql` access.
4. (Optional, nice-to-have) trigger GROQ curation for a chapter from the page.

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

## 5. Backend (IMPLEMENTED — reference only)

All of the following is merged. The frontend just needs to call it.

### Auth guard — `auth/middleware.go`

- `ParseEmailAllowlist(raw string) map[string]struct{}` parses
  `GAMES_EDITOR_EMAILS` (comma-separated, trimmed, lowercased).
- `RequireEditor(allowlist map[string]struct{}) gin.HandlerFunc` runs after
  `RequireAuth`; non-members get `403`, unauthenticated get `401`. An empty
  allowlist denies everyone.

### Endpoints — all under `/api/v1/admin/games`, gated by RequireAuth + RequireEditor

| Method | Path | Body / Query | Behaviour |
| --- | --- | --- | --- |
| GET | `/passages` | `?status=&chapter=` | list for review; `status` defaults to all when omitted, pass `proposed` to filter; full editorial fields returned |
| GET | `/passages/:id` | — | single passage detail |
| POST | `/passages/:id/status` | `{ "status": "approved" \| "rejected" \| "needs_refinement" }` | sets status; `approved` stamps `approved_at` and resolves `approved_by` from the signed-in editor email |
| POST | `/maintenance/seed-frequency` | — | recomputes `game_word_frequency` from `verse_text` in-process; returns `{ data: { language, tokens } }` |
| POST | `/maintenance/load-lemmas` | — | loads the lemma fixture embedded in the running binary into `game_lemma`; returns `{ data: { language, lemma_rows } }` |

- Responses use the `{ "data": ... }` envelope. Errors use `{ "message": ... }`
  with `400` (bad input), `404` (no such passage), `500` (server).
- Handlers live in `api/handlers/games_admin.go` (`GamesAdminHandler`); routes
  are registered in `api/server.go` BEFORE the OpenAPI validator (like the
  Sanity routes) so they are intentionally outside the typed contract. No
  `oapi-codegen` regeneration was needed.

### Maintenance semantics (important for the UI copy)

- `seed-frequency` is pure in-process DB work and fully re-runnable in prod.
  Improves blank-selection quality; not strictly required for play.
- `load-lemmas` loads whatever fixture ships in the current deployment.
  Generating the fixture needs the offline spaCy sidecar
  (`scripts/games_lemma`, see `db/seeds/games_lemma_en.sql`). Until a non-empty
  fixture is generated, committed, and redeployed, this is a safe no-op and
  Hard-tier acceptance falls back to exact match. So the button "loads the
  lemma data that shipped with this deploy", it does not run spaCy.

### Not built (still optional)

`POST /curate` (run GROQ from the page) was deferred. To add it later, extract
`proposePassages` / `validateProposals` / `normalizeDifficulty` from
`api/cmd/groq_curate/main.go` into an importable package and add an endpoint;
surface failures as `502`. Curation also still works from `just
games-curate-chapter N`.

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

Endpoints to wire (all live): list passages (status / chapter filters), set
status, seed-frequency, load-lemmas.

### F3. UI

Keep it minimal. One page:

- A status filter (default `proposed`) and optional chapter filter.
- A table or card list of passages: verse range, label, themes, llm_difficulty,
  llm_blank_hint, rationale, current status.
- Per row: Approve / Reject / Needs refinement buttons that POST the status and
  optimistically update (or refetch).
- A small "Maintenance" panel with two buttons (these replace the `just`
  commands the editor cannot run in prod):
  - "Rebuild word frequencies" → `POST /maintenance/seed-frequency`. Show the
    returned token count. Safe to re-run anytime.
  - "Load lemma data" → `POST /maintenance/load-lemmas`. Show the returned
    `lemma_rows`. Note in the UI that it loads the data shipped with the current
    deploy; `0` rows means no fixture has been generated/deployed yet (Hard tier
    still works via exact match).
  Disable the buttons while their request is in flight; surface a success toast
  with the count and an error toast on `500`.
- (Optional) a "Curate chapter N" input + button if/when `/curate` is added.

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
- Generating the lemma fixture from the UI. The spaCy step stays offline; the UI
  only triggers loading the already-shipped fixture and rebuilding frequencies.
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
5. "Rebuild word frequencies" returns a non-zero token count on a seeded DB.
   "Load lemma data" returns the embedded fixture row count (0 until a real
   fixture ships).
6. Backend stays green (already true at hand-off): `go build ./...`,
   `go vet ./...`, `go test ./...`.

## 10. Remaining work (frontend only)

Backend is merged. The only PR left is in `org`:

1. Protected studio route + review UI + maintenance panel (F1–F3).

(If `POST /curate` is wanted later, that is a separate backend follow-up; see
section 5 "Not built".)

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
