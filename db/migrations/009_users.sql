DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'user_role') THEN
        CREATE TYPE user_role AS ENUM ('admin', 'editor', 'member');
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS users (
    id            INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    supabase_uid  UUID NOT NULL UNIQUE,
    email         TEXT NOT NULL,
    display_name  TEXT,
    role          user_role NOT NULL DEFAULT 'member',
    permissions   JSONB NOT NULL DEFAULT '{}',
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_supabase_uid ON users(supabase_uid);
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
