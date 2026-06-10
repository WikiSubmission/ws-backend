#!/usr/bin/env bash
#
# restore-local-db.sh — (re)create an isolated local Postgres for editorial work
# by restoring a pg_dumpall cluster dump into a throwaway Docker container.
#
# Repeatable: re-running drops and recreates the container from the dump, so each
# attempt starts from a clean copy of the snapshot.
#
#   scripts/restore-local-db.sh [dump.gz]   # default dump: db/dumps/pg-dump-all-*.gz
#
# The container uses trust auth (it binds to 127.0.0.1 only) so the dump's
# original SCRAM passwords don't get in the way of local connections. Connect with:
#
#   postgres://ws_backend_user@127.0.0.1:5433/ws_backend?sslmode=disable
#
set -euo pipefail

DUMP="${1:-$(dirname "$0")/../db/dumps/pg-dump-all-1781049615.gz}"
CONTAINER="${WS_LOCAL_PG_CONTAINER:-ws-local-pg}"
IMAGE="${WS_LOCAL_PG_IMAGE:-pgvector/pgvector:pg18}"
HOST_PORT="${WS_LOCAL_PG_PORT:-5433}"

if [[ ! -f "$DUMP" ]]; then
  echo "ERROR: dump not found: $DUMP" >&2
  exit 1
fi

echo "==> Removing any existing container '$CONTAINER'"
docker rm -f "$CONTAINER" >/dev/null 2>&1 || true

echo "==> Starting $IMAGE as '$CONTAINER' on 127.0.0.1:$HOST_PORT"
docker run -d --name "$CONTAINER" \
  -e POSTGRES_PASSWORD=localdev \
  -e POSTGRES_HOST_AUTH_METHOD=trust \
  -p "127.0.0.1:${HOST_PORT}:5432" \
  "$IMAGE" >/dev/null

echo -n "==> Waiting for Postgres to accept connections"
for _ in $(seq 1 60); do
  if docker exec "$CONTAINER" pg_isready -U postgres -q 2>/dev/null; then
    echo " — ready"
    break
  fi
  echo -n "."
  sleep 1
done

echo "==> Restoring cluster dump (this includes roles + the ws_backend database)"
# ON_ERROR_STOP=0: the dump re-CREATEs the bootstrap 'postgres' role, which
# already exists — that single benign error must not abort the restore.
gunzip -c "$DUMP" | docker exec -i "$CONTAINER" \
  psql -U postgres -d postgres -q -v ON_ERROR_STOP=0 \
  > /tmp/ws-local-restore.log 2>&1 || true

echo "==> Restore summary"
docker exec "$CONTAINER" psql -U postgres -d ws_backend -tAc \
  "SELECT 'tables=' || count(*) FROM information_schema.tables WHERE table_schema='public';"
docker exec "$CONTAINER" psql -U postgres -d ws_backend -tAc \
  "SELECT 'verse_text=' || count(*) FROM verse_text;" 2>/dev/null \
  || echo "verse_text= (not present)"
docker exec "$CONTAINER" psql -U postgres -d ws_backend -tAc \
  "SELECT 'chapters=' || count(*) FROM chapters;" 2>/dev/null || true

echo "==> Done. DSN: postgres://ws_backend_user@127.0.0.1:${HOST_PORT}/ws_backend?sslmode=disable"
echo "    Restore log: /tmp/ws-local-restore.log"
