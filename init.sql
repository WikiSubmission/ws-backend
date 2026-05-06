-- ws-backend bootstrap for a shared Postgres instance (e.g. Coolify pgvector/pgvector:pg18).
-- Run ONCE as a superuser (e.g. `postgres`). Idempotent — safe to re-run.
--
-- Usage (from host):
--   psql "postgres://postgres:SUPERPASS@HOST:5432/postgres" \
--        -v db_name=ws_backend \
--        -v db_user=ws_backend_user \
--        -v db_pass='ns7592dhndu49Dief' \
--        -f init.sql
--
-- Schema itself is created by the app at startup via db.DbSetup()
-- (see db/migrations/*.sql). This file only handles role + database +
-- extensions + permissions.

\set ON_ERROR_STOP on

-- 1. Role (login user for the app).
SELECT format('CREATE ROLE %I WITH LOGIN PASSWORD %L', :'db_user', :'db_pass')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'db_user')
\gexec

-- 2. Database owned by that role.
SELECT format('CREATE DATABASE %I OWNER %I', :'db_name', :'db_user')
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'db_name')
\gexec

-- 3. Switch into the new database as superuser to install extensions.
--    `vector` powers semantic search on verse_text (see migrations/011_pgvector.sql).
--    `pg_trgm` is optional but useful for fuzzy fallback on text columns.
\c :"db_name"

CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- 4. Permissions.
GRANT ALL ON SCHEMA public TO :"db_user";
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES    TO :"db_user";
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO :"db_user";
