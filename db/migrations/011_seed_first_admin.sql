-- Seed the first admin user so DB-backed access control can take over from
-- the GAMES_EDITOR_EMAILS env allowlist without locking anyone out. After
-- this runs, the admin can grant games_editor (and other future permissions)
-- to other accounts from the admin access page.
--
-- Idempotent: only flips the role; running again is a no-op.

UPDATE users
SET role = 'admin', updated_at = now()
WHERE email = 'hesham@wikisubmission.org' AND role <> 'admin';
