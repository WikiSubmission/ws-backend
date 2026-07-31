-- 027_editor_game_grants.sql
--
-- Per-game editorial access. The games review surface was previously gated by a
-- single JSONB flag (users.permissions.games_editor), so one boolean unlocked
-- every game. This introduces the same two-tier shape the scripture editor
-- already uses (module grant + per-resource grant):
--
--   1. A new 'game' value on public.editor_module. A row in
--      editor_module_grants with module = 'game' is the GLOBAL tier: presence
--      grants read on every game, can_write grants write on every game.
--
--   2. editor_game_grants is the PER-GAME tier, keyed against the new
--      editor_games registry so the admin console can enumerate games from the
--      database the way it already enumerates quran_versions.
--
-- IMPORTANT semantic difference from version grants: per-version access
-- INTERSECTS with its module grant (both are required), whereas game access is
-- a UNION — the global 'game' module grant OR a per-game grant is sufficient on
-- its own. See EditorPermissions.CanReadGame in db/editorial_permissions.go.
--
-- The backfill of existing games_editor holders lives in 028, NOT here: the
-- migration runner sends each file as a single Exec, which pgx executes in one
-- implicit transaction, and Postgres refuses to *use* an enum value added by
-- ALTER TYPE in the same transaction that added it ("unsafe use of new value of
-- enum type"). Splitting the files commits the enum change first.
--
-- Idempotent: safe to run repeatedly.

-- ── enum: add the games module ─────────────────────────────────────────────────

ALTER TYPE public.editor_module ADD VALUE IF NOT EXISTS 'game';

-- ── game registry ─────────────────────────────────────────────────────────────
-- One row per game with an editorial surface. game_key matches the route
-- segment under /api/v1/admin/games/{gameKey} and /admin/games/{gameKey}.

CREATE TABLE IF NOT EXISTS public.editor_games (
    key        text PRIMARY KEY,
    name       text NOT NULL,
    sort_order integer NOT NULL DEFAULT 0,
    is_active  boolean NOT NULL DEFAULT true,
    created_at timestamp with time zone NOT NULL DEFAULT now()
);

INSERT INTO public.editor_games (key, name, sort_order)
VALUES ('fill-blank', 'Fill in the Blank', 1)
ON CONFLICT (key) DO NOTHING;

-- ── editor permissions: per-game ──────────────────────────────────────────────
-- Presence of a row grants READ on that game; can_write additionally grants
-- WRITE. Does NOT require the 'game' module grant (union semantics, above).

CREATE TABLE IF NOT EXISTS public.editor_game_grants (
    user_id    integer NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    game_key   text NOT NULL REFERENCES public.editor_games(key) ON DELETE CASCADE,
    can_write  boolean NOT NULL DEFAULT false,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, game_key)
);

CREATE INDEX IF NOT EXISTS idx_editor_game_grants_user
    ON public.editor_game_grants (user_id);
