# Multi-role access rollout

Migration `030_multi_user_roles.sql` is additive. Do not drop `users.role` or
the compatibility trigger during the local, staging, or initial production
rollout.

## Deployment order

Repeat this sequence in local, then staging, then production:

1. Deploy the backend. Startup applies migration 030 before serving traffic.
2. Verify the invariants below.
3. Exercise `/editorial/session`, `/users`, and `/editorial/admin/editors` with
   an admin, CMS editor, game editor, and ordinary member.
4. Deploy the frontend containing the multi-role access dialog.
5. Verify role combinations, CMS/game grants, the Quran reference preference,
   and category deletion restrictions.

The old frontend can continue using the deprecated single `role` field while
the backend rolls. The old backend can continue reading `users.role` if a
rollback is needed. `game_editor` projects to legacy `member`; old game access
continues to come from the existing game grant tables.

## Post-migration invariants

```sql
-- Must return 0: every account is a member.
SELECT count(*)
FROM users u
WHERE NOT EXISTS (
  SELECT 1 FROM user_roles ur
  WHERE ur.user_id = u.id AND ur.role = 'member'
);

-- Must return 0: legacy admins/editors were preserved.
SELECT count(*)
FROM users u
WHERE u.role::text IN ('admin', 'editor')
  AND NOT EXISTS (
    SELECT 1 FROM user_roles ur
    WHERE ur.user_id = u.id AND ur.role::text = u.role::text
  );

-- Must return 0: existing CMS grants received the editor role.
SELECT count(*)
FROM editor_module_grants g
WHERE g.module::text <> 'game'
  AND NOT EXISTS (
    SELECT 1 FROM user_roles ur
    WHERE ur.user_id = g.user_id AND ur.role = 'editor'
  );

-- Must return 0: existing game grants received the game_editor role.
SELECT count(*)
FROM (
  SELECT user_id FROM editor_module_grants WHERE module::text = 'game'
  UNION
  SELECT user_id FROM editor_game_grants
) g
WHERE NOT EXISTS (
  SELECT 1 FROM user_roles ur
  WHERE ur.user_id = g.user_id AND ur.role = 'game_editor'
);

-- Must be at least 1 before proceeding.
SELECT count(DISTINCT u.id) AS active_admins
FROM users u
JOIN user_roles ur ON ur.user_id = u.id
WHERE ur.role = 'admin' AND u.is_active;
```

## Rollback

Roll back application versions without reversing migration 030. The migration
keeps `users.role` synchronized as the highest legacy role, and all previous
grant tables remain intact. Reversing or removing the additive schema should be
a later, separately reviewed migration only after the production rollback
window has closed.
