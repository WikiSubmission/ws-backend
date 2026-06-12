# TODO

## Deferred — Bible editorial module (Phase 3, unlikely but planned)

Largest backend gap of the editorial port; deprioritized in favor of completing
Quran parity. Mirror the Quran write path once/if picked up.

- [ ] **Bible draft/history tables** — none exist today. Add version-keyed
      `bible_*_drafts` + `*_history` for book/chapter/verse translations,
      following the shape of the Quran editorial tables (additive, version-keyed,
      partial-unique upsert keys).
- [ ] **Write handlers** — book/chapter nav reads, chapter + verse translation
      editors, reference-version display, per-version read/write permission
      wiring (reuse the editorial permission model + `RequirePermission`).
- [ ] **Publish workflow** — per-chapter publish requests + changelog + approver
      grant, identical state machine to Quran (`019_quran_publish_workflow.sql`).
- [ ] **Bible sync** — the existing Bible sync path is stubbed; implement it.
- [ ] **Frontend** — version management, book/chapter nav, chapter + verse
      translation editors, reference rail. Mirror `org/app/(editor)/editor/quran/`.

Risk notes: no Bible word model; canonical-text gaps likely surface during build
(flag them rather than backfilling blind). Full phase write-up:
`.claude/plans/wikisubmission-editorial-app-port.md` §3 Phase 3.

## Deferred (do last)

- [ ] **Version-key the live Quran corpus** — migrate `verse_text` /
      `chapter_text` / `word_text` from lang-keyed to version-keyed, backfill,
      and repoint the public reader. Staged local → staging → prod, zero data
      loss. Full runbook: [docs/quran-version-keying-migration.md](docs/quran-version-keying-migration.md).
      Phase 1 publish writes additive editorial tables so this can be switched on
      later without contract/handler changes.

## Misc

- ~~how to cache the response of /chapters and /appendices?~~ Done via `Cache-Control` headers.

## Sanity proxy

- [ ] **Auth on `/communities`**: currently public read. Decide whether to gate
      reads behind a JWT once the consumer site needs user-specific filtering
      (e.g. communities scoped to a logged-in user's region) or write access.
- [ ] **Cloudflare cache rule for `/sanity/image/*`**: passthrough binary,
      eligible for long edge TTL (response already sets `Cache-Control:
      public, max-age=86400, stale-while-revalidate=604800`).
- [ ] **Multi-replica cache**: in-process TTL cache only works for a single
      instance. Move to Redis (or a Sanity-fronting CDN allow-list of GROQ
      hashes) before scaling out.
- [ ] **Sanity Studio webhook config**: project owner must register a webhook
      pointing at `POST /webhooks/sanity` with projection
      `{ "_id": _id, "_type": _type, "operation": delta::operation() }` and
      the shared secret matching `SANITY_WEBHOOK_SECRET`.

## Cloudflare Cache Setup (CDN edge caching for public API)

Backend already sends correct `Cache-Control` headers:

- `/languages`, `/chapters`, `/appendices` → `public, max-age=86400` (24h)
- `/quran`, `/search` → `public, max-age=3600` (1h)

Steps to activate edge caching on Cloudflare:

1. DNS record for the domain must use **orange cloud** (Proxied), not grey (DNS only)
2. Cloudflare dashboard → Caching → Cache Rules → Create rule:
   - Match: `URI Path starts with /api/v1/` AND `Method = GET`
   - Action: Eligible for cache, Edge TTL = Override origin (use header value)
3. Add a second higher-priority rule to **bypass** cache for:
   - `URI Path starts with /api/v1/editor/` OR `/api/v1/auth/`
4. Verify with: `curl -I https://yourdomain.com/api/v1/chapters` → look for `CF-Cache-Status: HIT`
