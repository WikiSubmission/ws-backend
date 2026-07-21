-- 025_editorial_author_user_link.sql
--
-- Author profiles become a 1:1 projection of a WS user account: each user has
-- at most one editorial_authors row. A partial unique index enforces this for
-- linked rows, while legacy authors imported from the previous CMS (user_id
-- NULL) stay unconstrained.
--
-- Rows are auto-seeded from users.display_name when a user is granted the
-- article editorial module (see db.ReplaceEditorGrants); both the user and
-- admins can edit the profile afterwards.
--
-- Idempotent: safe to run repeatedly.

CREATE UNIQUE INDEX IF NOT EXISTS editorial_authors_user_id_key
    ON public.editorial_authors (user_id)
    WHERE user_id IS NOT NULL;
