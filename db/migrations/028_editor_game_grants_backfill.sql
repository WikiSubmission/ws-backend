-- 028_editor_game_grants_backfill.sql
--
-- Backfills the 'game' module grant for everyone who currently holds the legacy
-- users.permissions.games_editor flag. Without this, every existing games editor
-- loses access the moment auth.RequireGameEditor starts reading the grant tables
-- introduced in 027.
--
-- Separate from 027 by necessity, not style: Postgres cannot use an enum value
-- in the same transaction that ALTER TYPE added it, and the migration runner
-- executes each file as one implicit transaction. See the note in 027.
--
-- Existing holders get the GLOBAL tier with write, which is what the single
-- boolean meant. Admins are intentionally skipped — they bypass all grants and
-- are never rows in the grant tables.
--
-- The legacy permissions.games_editor key is deliberately left in place so a
-- rollback to the previous binary is non-destructive. Dropping it is a later,
-- separate cleanup.
--
-- Idempotent: safe to run repeatedly (ON CONFLICT DO NOTHING).

INSERT INTO public.editor_module_grants (user_id, module, can_write)
SELECT id, 'game', true
FROM public.users
WHERE permissions ->> 'games_editor' = 'true'
  AND role <> 'admin'
ON CONFLICT (user_id, module) DO NOTHING;
