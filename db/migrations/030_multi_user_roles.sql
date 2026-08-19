-- 030_multi_user_roles.sql
--
-- Additive multi-role model. The legacy users.role column is intentionally
-- retained as a compatibility projection while local, staging, and production
-- roll through the new backend/frontend independently.

DO $WS_030$ BEGIN
    CREATE TYPE public.access_role AS ENUM (
        'member',
        'editor',
        'game_editor',
        'admin'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $WS_030$;

CREATE TABLE IF NOT EXISTS public.user_roles (
    user_id integer NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    role public.access_role NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role)
);

CREATE INDEX IF NOT EXISTS idx_user_roles_role_user
    ON public.user_roles (role, user_id);

-- Every account is a member. Preserve the old elevated role during backfill.
INSERT INTO public.user_roles (user_id, role)
SELECT id, 'member'::public.access_role FROM public.users
ON CONFLICT DO NOTHING;

INSERT INTO public.user_roles (user_id, role)
SELECT id, role::text::public.access_role
FROM public.users
WHERE role::text IN ('editor', 'admin')
ON CONFLICT DO NOTHING;

-- Grant tables were authoritative before multi-role support, so preserve every
-- existing editor even when users.role remained member.
INSERT INTO public.user_roles (user_id, role)
SELECT DISTINCT user_id, 'editor'::public.access_role
FROM public.editor_module_grants
WHERE module::text <> 'game'
ON CONFLICT DO NOTHING;

INSERT INTO public.user_roles (user_id, role)
SELECT DISTINCT user_id, 'game_editor'::public.access_role
FROM (
    SELECT user_id FROM public.editor_module_grants WHERE module::text = 'game'
    UNION
    SELECT user_id FROM public.editor_game_grants
) existing_game_editors
ON CONFLICT DO NOTHING;

-- Old application versions may still create users or update users.role. Keep
-- those writes represented in user_roles during a rolling deployment. The new
-- backend updates users.role first, then writes the complete role set.
CREATE OR REPLACE FUNCTION public.sync_legacy_user_role()
RETURNS trigger
LANGUAGE plpgsql
AS $WS_030$
BEGIN
    INSERT INTO public.user_roles (user_id, role)
    VALUES (NEW.id, 'member')
    ON CONFLICT DO NOTHING;

    IF TG_OP = 'UPDATE' AND NEW.role IS DISTINCT FROM OLD.role THEN
        DELETE FROM public.user_roles
        WHERE user_id = NEW.id AND role IN ('editor', 'admin');
    END IF;

    IF NEW.role::text IN ('editor', 'admin') THEN
        INSERT INTO public.user_roles (user_id, role)
        VALUES (NEW.id, NEW.role::text::public.access_role)
        ON CONFLICT DO NOTHING;
    END IF;
    RETURN NEW;
END
$WS_030$;

DROP TRIGGER IF EXISTS users_sync_legacy_role ON public.users;
CREATE TRIGGER users_sync_legacy_role
AFTER INSERT OR UPDATE OF role ON public.users
FOR EACH ROW EXECUTE FUNCTION public.sync_legacy_user_role();

-- Legacy author/category grant rows are deliberately retained for rollback.
-- New authorization ignores them because authors are admin-only and categories
-- inherit the Articles grant.
