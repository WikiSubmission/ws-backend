# TODO

## Misc

- ~~how to cache the response of /chapters and /appendices?~~ Done via `Cache-Control` headers.

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
