# ws-backend justfile — run `just` to list recipes.

set dotenv-load := true
set positional-arguments := true
set shell := ["bash", "-euo", "pipefail", "-c"]

bin := "bin/server"
dumps_dir := "db/dumps"

# Default: list recipes
default:
    @just --list

# ---------------------------------------------------------------------------
# Dev
# ---------------------------------------------------------------------------

# Start the API in development mode (pretty slog output)
run:
    APP_ENV=development go run ./api/

# Compile a static binary to bin/server
build:
    @mkdir -p bin
    CGO_ENABLED=0 go build -trimpath -ldflags="-w -s" -o {{bin}} ./api/
    echo "Binary written to {{bin}}"

# Run all tests (DB tests require a live DATABASE_NAME env var)
test:
    go test ./...

# Tidy the Go module
tidy:
    go mod tidy

# Regenerate OpenAPI types from openapi.yaml
generate:
    go tool oapi-codegen -config oapi-codegen.yaml openapi.yaml

# Remove compiled binaries and Go build cache
clean:
    rm -rf bin/
    go clean -cache

# ---------------------------------------------------------------------------
# Connection helpers (private)
# ---------------------------------------------------------------------------

# Resolve a target name (local|railway|hetzner|...) to a Postgres URL.
# Looks up DATABASE_URL_<UPPER>; for `local` falls back to composed DSN.
[private]
url TARGET:
    #!/usr/bin/env bash
    set -euo pipefail
    name="$1"
    var="DATABASE_URL_$(echo "$name" | tr '[:lower:]' '[:upper:]')"
    val="${!var:-}"
    if [ -n "$val" ]; then echo "$val"; exit 0; fi
    if [ "$name" = "local" ] && [ -n "${DATABASE_USER:-}" ]; then
      echo "host=${DATABASE_DOMAIN} port=${DATABASE_PORT} user=${DATABASE_USER} password=${DATABASE_PASSWORD} dbname=${DATABASE_NAME} sslmode=${DATABASE_SSL_MODE:-disable}"
      exit 0
    fi
    echo "ERROR: $var is not set in .env (and no fallback available for '$name')" >&2
    exit 1

# Build a `psql` invocation against a target.
[private]
psql TARGET *ARGS:
    #!/usr/bin/env bash
    set -euo pipefail
    url="$(just url "$1")"; shift
    psql "$url" --set ON_ERROR_STOP=1 "$@"

# ---------------------------------------------------------------------------
# DB lifecycle (local)
# ---------------------------------------------------------------------------

# Create local PostgreSQL role + database + grants
db-setup:
    #!/usr/bin/env bash
    set -euo pipefail
    case "$(uname -s)" in
      Darwin) admin="psql -d postgres --set ON_ERROR_STOP=1" ;;
      *)      admin="sudo -u postgres psql --set ON_ERROR_STOP=1" ;;
    esac
    $admin -c "CREATE USER ${DATABASE_USER} WITH PASSWORD '${DATABASE_PASSWORD}';" || echo "User exists."
    $admin -c "CREATE DATABASE ${DATABASE_NAME} OWNER ${DATABASE_USER};" || echo "Database exists."
    $admin -c "GRANT ALL PRIVILEGES ON DATABASE ${DATABASE_NAME} TO ${DATABASE_USER};"
    # Extensions require superuser; create them as postgres in the target DB.
    case "$(uname -s)" in
      Darwin) admin_db="psql -d ${DATABASE_NAME} --set ON_ERROR_STOP=1" ;;
      *)      admin_db="sudo -u postgres psql -d ${DATABASE_NAME} --set ON_ERROR_STOP=1" ;;
    esac
    $admin_db -c "CREATE EXTENSION IF NOT EXISTS vector; CREATE EXTENSION IF NOT EXISTS pg_trgm;"
    PGPASSWORD="${DATABASE_PASSWORD}" psql -U "${DATABASE_USER}" -h "${DATABASE_DOMAIN}" -p "${DATABASE_PORT}" \
      -d "${DATABASE_NAME}" --set ON_ERROR_STOP=1 \
      -c "GRANT USAGE, CREATE ON SCHEMA public TO ${DATABASE_USER};"
    echo "Local DB ready."

# Drop local database and role
db-drop:
    #!/usr/bin/env bash
    set -euo pipefail
    case "$(uname -s)" in
      Darwin) admin="psql -d postgres --set ON_ERROR_STOP=1" ;;
      *)      admin="sudo -u postgres psql --set ON_ERROR_STOP=1" ;;
    esac
    $admin -c "DROP DATABASE IF EXISTS ${DATABASE_NAME};"
    $admin -c "DROP USER IF EXISTS ${DATABASE_USER};"

# Bootstrap a shared Postgres (Coolify/Hetzner) via init.sql
coolify-init:
    @test -n "${POSTGRES_ADMIN_URL:-}" || (echo "ERROR: POSTGRES_ADMIN_URL is not set in .env" && exit 1)
    psql "$POSTGRES_ADMIN_URL" --set ON_ERROR_STOP=1 \
      -v db_name=$DATABASE_NAME -v db_user=$DATABASE_USER -v db_pass="'$DATABASE_PASSWORD'" \
      -f init.sql
    echo "Bootstrap complete for $DATABASE_NAME."

# ---------------------------------------------------------------------------
# Snapshots
# ---------------------------------------------------------------------------

# Dump data from a target into db/dumps/<ts>__<source>.dump (default: railway)
export SOURCE="railway":
    #!/usr/bin/env bash
    set -euo pipefail
    url="$(just url "$1")"
    mkdir -p {{dumps_dir}}
    ts="$(date -u +%Y-%m-%dT%H-%M-%SZ)"
    out="{{dumps_dir}}/${ts}__$1.dump"
    echo "Exporting $1 -> $out"
    pg_dump --format=custom --data-only --no-owner --no-acl \
            --exclude-table=users \
            --exclude-table-data=verse_text \
            "$url" -f "$out"
    # verse_text is excluded from the custom dump so embeddings don't bloat the file;
    # it ships as a separate CSV without the embedding column.
    csv="{{dumps_dir}}/${ts}__$1.verse_text.csv"
    psql "$url" --set ON_ERROR_STOP=1 \
      -c "\COPY (SELECT id, verse_id, lang_id, subtitle, content, footer, published_at FROM verse_text ORDER BY id) TO '$(pwd)/$csv' WITH (FORMAT CSV)"
    sz=$(du -sh "$out" | cut -f1)
    rows=$(wc -l < "$csv" | tr -d ' ')
    echo "Done. dump=$sz, verse_text=$rows rows."

# Dump only the verse_text embedding column (slow + large; rarely needed)
export-embeddings SOURCE="railway":
    #!/usr/bin/env bash
    set -euo pipefail
    url="$(just url "$1")"
    mkdir -p {{dumps_dir}}/embeddings
    ts="$(date -u +%Y-%m-%dT%H-%M-%SZ)"
    out="{{dumps_dir}}/embeddings/${ts}__$1.csv"
    psql "$url" --set ON_ERROR_STOP=1 \
      -c "\COPY (SELECT id, embedding::text FROM verse_text WHERE embedding IS NOT NULL) TO '$(pwd)/$out' WITH (FORMAT CSV)"
    rows=$(wc -l < "$out" | tr -d ' ')
    echo "Exported $rows embeddings to $out"

# List dumps in db/dumps/, newest first
dumps:
    @ls -lh --time-style=long-iso {{dumps_dir}}/*.dump 2>/dev/null | awk '{print $6, $7, $5, $8}' || echo "no dumps yet"

# Restore a snapshot into local (default: latest dump)
import VERSION="latest":
    #!/usr/bin/env bash
    set -euo pipefail
    if [ "$1" = "latest" ]; then
      file=$(ls -1t {{dumps_dir}}/*.dump 2>/dev/null | head -1)
    else
      file=$(ls -1 {{dumps_dir}}/*$1*.dump 2>/dev/null | head -1)
    fi
    [ -n "$file" ] && [ -f "$file" ] || { echo "ERROR: no dump matching '$1'" >&2; exit 1; }
    csv="${file%.dump}.verse_text.csv"
    [ -f "$csv" ] || { echo "ERROR: missing companion csv $csv" >&2; exit 1; }
    url="$(just url local)"
    echo "Restoring $file into local..."
    psql "$url" --set ON_ERROR_STOP=1 -c "TRUNCATE TABLE \
      music_tracks, music_albums, music_categories, music_artists, \
      bible_verse_text, bible_book_text, bible_verses, bible_chapters, bible_books, \
      word_text_history, verse_text_history, chapter_text_history, \
      word_text_drafts, verse_text_drafts, chapter_text_drafts, \
      word_text, verse_text, chapter_text, \
      appendices, words, verses, chapters, languages \
      RESTART IDENTITY CASCADE;"
    pg_restore --data-only --single-transaction --no-owner -d "$url" "$file"
    psql "$url" --set ON_ERROR_STOP=1 \
      -c "\COPY verse_text(id, verse_id, lang_id, subtitle, content, footer, published_at) FROM '$(pwd)/$csv' WITH (FORMAT CSV)"
    psql "$url" --set ON_ERROR_STOP=1 -c "SELECT setval(pg_get_serial_sequence('verse_text','id'), (SELECT MAX(id) FROM verse_text));"
    # First refresh of an unpopulated mat-view cannot use CONCURRENTLY; fall back if needed.
    psql "$url" --set ON_ERROR_STOP=1 -c "REFRESH MATERIALIZED VIEW CONCURRENTLY root_index_mv;" 2>/dev/null \
      || psql "$url" --set ON_ERROR_STOP=1 -c "REFRESH MATERIALIZED VIEW root_index_mv;"
    echo "Import complete."

# Push a snapshot to a non-local target (railway|hetzner|...). Prompts.
push TARGET VERSION="latest":
    #!/usr/bin/env bash
    set -euo pipefail
    target="$1"; ver="${2:-latest}"
    [ "$target" = "local" ] && { echo "Use 'just import' for local." >&2; exit 1; }
    if [ "$ver" = "latest" ]; then
      file=$(ls -1t {{dumps_dir}}/*.dump 2>/dev/null | head -1)
    else
      file=$(ls -1 {{dumps_dir}}/*$ver*.dump 2>/dev/null | head -1)
    fi
    [ -n "$file" ] && [ -f "$file" ] || { echo "ERROR: no dump matching '$ver'" >&2; exit 1; }
    url="$(just url "$target")"
    host=$(echo "$url" | sed -E 's#.*@([^/]+)/.*#\1#')
    echo "About to overwrite $target ($host) with $file."
    read -r -p "Continue? [y/N] " ans
    [ "$ans" = "y" ] || [ "$ans" = "Y" ] || { echo "Aborted."; exit 1; }
    csv="${file%.dump}.verse_text.csv"
    psql "$url" --set ON_ERROR_STOP=1 -c "TRUNCATE TABLE \
      music_tracks, music_albums, music_categories, music_artists, \
      bible_verse_text, bible_book_text, bible_verses, bible_chapters, bible_books, \
      word_text_history, verse_text_history, chapter_text_history, \
      word_text_drafts, verse_text_drafts, chapter_text_drafts, \
      word_text, verse_text, chapter_text, \
      appendices, words, verses, chapters, languages \
      RESTART IDENTITY CASCADE;"
    pg_restore --data-only --single-transaction --no-owner -d "$url" "$file"
    psql "$url" --set ON_ERROR_STOP=1 \
      -c "\COPY verse_text(id, verse_id, lang_id, subtitle, content, footer, published_at) FROM '$(pwd)/$csv' WITH (FORMAT CSV)"
    psql "$url" --set ON_ERROR_STOP=1 -c "SELECT setval(pg_get_serial_sequence('verse_text','id'), (SELECT MAX(id) FROM verse_text));"
    # First refresh of an unpopulated mat-view cannot use CONCURRENTLY; fall back if needed.
    psql "$url" --set ON_ERROR_STOP=1 -c "REFRESH MATERIALIZED VIEW CONCURRENTLY root_index_mv;" 2>/dev/null \
      || psql "$url" --set ON_ERROR_STOP=1 -c "REFRESH MATERIALIZED VIEW root_index_mv;"
    echo "Pushed to $target."

# Upload latest embeddings CSV to a target (UPDATE only, no re-embedding)
push-embeddings TARGET:
    #!/usr/bin/env bash
    set -euo pipefail
    target="$1"
    url="$(just url "$target")"
    file=$(ls -1t {{dumps_dir}}/embeddings/*.csv 2>/dev/null | head -1)
    [ -n "$file" ] && [ -f "$file" ] || { echo "ERROR: no embeddings CSV in {{dumps_dir}}/embeddings/" >&2; exit 1; }
    echo "Uploading $file to $target..."
    psql "$url" --set ON_ERROR_STOP=1 -c "DROP TABLE IF EXISTS _emb_stage; CREATE TABLE _emb_stage (id INT PRIMARY KEY, embedding vector(512));"
    psql "$url" --set ON_ERROR_STOP=1 -c "\COPY _emb_stage (id, embedding) FROM '$(pwd)/$file' WITH (FORMAT CSV)"
    psql "$url" --set ON_ERROR_STOP=1 -c "UPDATE verse_text v SET embedding = e.embedding FROM _emb_stage e WHERE v.id = e.id; DROP TABLE _emb_stage;"
    echo "Embeddings restored on $target."

# Compare row counts of key tables between two targets (sanity check post-migration)
verify A B:
    #!/usr/bin/env bash
    set -euo pipefail
    ua="$(just url "$1")"; ub="$(just url "$2")"
    tables="languages chapters verses words chapter_text verse_text word_text appendices bible_books bible_chapters bible_verses bible_book_text bible_verse_text music_artists music_categories music_albums music_tracks"
    # Build one query that returns all counts in a single round trip, so
    # flaky networks (firewall connection-rate limits) only have to hold up
    # once per side instead of once per table.
    sql=""
    for t in $tables; do
      sql+="SELECT '$t' AS t, count(*)::text AS c FROM $t UNION ALL "
    done
    sql="${sql%UNION ALL }ORDER BY 1;"
    fetch() {
      psql "$1" -tAF $'\t' -c "$sql" 2>/dev/null \
        | awk -F'\t' 'NF==2 {print $1"="$2}'
    }
    declare -A CA CB
    while IFS='=' read -r k v; do CA["$k"]="$v"; done < <(fetch "$ua")
    while IFS='=' read -r k v; do CB["$k"]="$v"; done < <(fetch "$ub")
    printf "%-22s %12s %12s %s\n" "table" "$1" "$2" "delta"
    printf "%-22s %12s %12s %s\n" "----------------------" "------------" "------------" "-----"
    diffs=0
    for t in $tables; do
      ca="${CA[$t]:-?}"; cb="${CB[$t]:-?}"
      d=""
      if [ "$ca" != "$cb" ]; then d="MISMATCH"; diffs=$((diffs+1)); fi
      printf "%-22s %12s %12s %s\n" "$t" "$ca" "$cb" "$d"
    done
    [ "$diffs" -eq 0 ] && echo "OK — all tables match." || { echo "WARNING: $diffs mismatches."; exit 1; }

# ---------------------------------------------------------------------------
# Embeddings
# ---------------------------------------------------------------------------

# Generate verse_text embeddings via OpenAI (idempotent; skips already-embedded rows)
embed-quran:
    @test -n "${OPENAI_API_KEY:-}" || (echo "ERROR: OPENAI_API_KEY is not set in .env" && exit 1)
    go run ./api/cmd/seed_embeddings/

# ---------------------------------------------------------------------------
# Offline content bundles (SQLite + FTS5) for the PWA / Capacitor app
# ---------------------------------------------------------------------------

# Build offline Quran bundles + manifest from a target DB (local|railway|...).
# Outputs to ./offline-bundles. S3/CloudFront upload is deferred (local fixtures).
offline-bundles TARGET="local" LANGS="ar,en" DATA_VERSION="1":
    #!/usr/bin/env bash
    set -euo pipefail
    url="$(just url "$1")"
    DATABASE_URL="$url" go run ./cmd/build-offline-bundles \
      -out offline-bundles -langs "$2" -data-version "$3"

# ---------------------------------------------------------------------------
# Seeding (legacy source data → normalized schema)
# ---------------------------------------------------------------------------

# Import legacy ws_quran_* source dumps into a target (local|railway)
source-import TARGET="local":
    #!/usr/bin/env bash
    set -euo pipefail
    url="$(just url "$1")"
    p() { psql "$url" --set ON_ERROR_STOP=1 "$@"; }
    p -f assets/source_data/source_schema.sql
    p -f assets/source_data/ws_quran_chapters_rows.sql
    p -f assets/source_data/ws_quran_footnotes_rows.sql
    p -f assets/source_data/ws_quran_index_rows.sql
    p -f assets/source_data/ws_quran_subtitles_rows.sql
    p -f assets/source_data/ws_quran_text_rows.sql
    p -f assets/source_data/ws_quran_word_by_word_rows.sql
    p -c "GRANT SELECT ON ALL TABLES IN SCHEMA public TO ${DATABASE_USER};" || true

# Run the legacy → normalized seed transformation (Go script)
seed TARGET="local":
    #!/usr/bin/env bash
    set -euo pipefail
    url="$(just url "$1")"
    DATABASE_URL="$url" go run api/cmd/seed.go

# Seed Bible data from CSV
seed-bible:
    go run ./api/cmd/seed_bible/

# Seed music tables on a target (local|railway)
seed-music TARGET="local":
    #!/usr/bin/env bash
    set -euo pipefail
    url="$(just url "$1")"
    p() { psql "$url" --set ON_ERROR_STOP=1 "$@"; }
    p -c "DROP TABLE IF EXISTS public.ws_music_tracks, public.ws_music_albums, public.ws_music_categories, public.ws_music_artists CASCADE;"
    p -f assets/source_data/ws_music_artists.sql
    p -f assets/source_data/ws_music_categories.sql
    p -f assets/source_data/ws_music_album.sql
    p -f assets/source_data/ws_music_tracks.sql
    p -f assets/ws_music_artists_rows.sql
    p -f assets/ws_music_categories_rows.sql
    p -f assets/ws_music_tracks_rows.sql
    p -f assets/source_data/music_transform.sql
    p -c "DROP TABLE IF EXISTS public.ws_music_tracks, public.ws_music_albums, public.ws_music_categories, public.ws_music_artists CASCADE;"
    echo "Music seed complete on $1."

# Promote a user to admin role (local). Usage: just seed-admin foo@bar.com
seed-admin EMAIL:
    go run ./api/cmd/seed_admin/ --email "{{EMAIL}}"

# ---------------------------------------------------------------------------
# Quran Games
# ---------------------------------------------------------------------------

# Populate game_word_frequency for the English (RK) translation. Idempotent.
games-seed-frequency:
    go run ./api/cmd/games_seed_frequency/ --language en

# Regenerate the lemma fixture from the seeded frequency vocabulary using the
# spaCy sidecar. Requires the venv from scripts/games_lemma to be active (see
# scripts/games_lemma/README.md). Writes assets/source_data/games_lemma_en.sql,
# which must then be committed. Run games-seed-frequency first.
games-build-lemma-fixture TARGET="local":
    #!/usr/bin/env bash
    set -euo pipefail
    url="$(just url "$1")"
    out="db/seeds/games_lemma_en.sql"
    psql "$url" --set ON_ERROR_STOP=1 -tAc \
      "SELECT DISTINCT word_normalized FROM game_word_frequency WHERE language='en' ORDER BY word_normalized" \
      | python3 scripts/games_lemma/generate_lemma_fixture.py --language en > "$out"
    echo "Wrote $out — commit it so it ships embedded, then redeploy and click"
    echo "\"Load lemma data\" in the editor (or run: just games-seed-lemma $1)."

# One-shot lemma fixture refresh: bootstraps the Python venv if missing, then
# generates db/seeds/games_lemma_<LANGUAGE>.sql from the chosen target DB's
# vocabulary. Supported languages: en fr de es ru sv (those with spaCy models).
# Run once per language, NOT per curation pass. Re-run only when translation
# text changes or when you bump spaCy.
games-refresh-lemmas TARGET="local" LANGUAGE="en":
    #!/usr/bin/env bash
    set -euo pipefail
    declare -A MODELS=(
      [en]="en_core_web_sm"
      [fr]="fr_core_news_sm"
      [de]="de_core_news_sm"
      [es]="es_core_news_sm"
      [ru]="ru_core_news_sm"
      [sv]="sv_core_news_sm"
    )
    model="${MODELS[$2]:-}"
    if [ -z "$model" ]; then
      echo "No spaCy model for language '$2'. Supported: ${!MODELS[*]}" >&2
      exit 1
    fi
    venv="scripts/games_lemma/.venv"
    if [ ! -d "$venv" ]; then
      echo "Creating Python venv at $venv (first run only)…"
      python3 -m venv "$venv"
      "$venv/bin/pip" install --quiet -r scripts/games_lemma/requirements.txt
    fi
    if ! "$venv/bin/python" -c "import spacy; spacy.load('$model')" 2>/dev/null; then
      echo "Downloading spaCy model $model…"
      "$venv/bin/python" -m spacy download "$model"
    fi
    url="$(just url "$1")"
    out="db/seeds/games_lemma_$2.sql"
    psql "$url" --set ON_ERROR_STOP=1 -tAc \
      "SELECT DISTINCT word_normalized FROM game_word_frequency WHERE language='$2' ORDER BY word_normalized" \
      | "$venv/bin/python" scripts/games_lemma/generate_lemma_fixture.py --language "$2" > "$out"
    echo ""
    echo "Wrote $out."

# Refresh lemma fixtures for all spaCy-supported languages in one pass.
# Seeds word frequencies first (required), then generates all fixtures.
# Commit the resulting db/seeds/games_lemma_*.sql files and redeploy.
games-refresh-all-lemmas TARGET="local":
    #!/usr/bin/env bash
    set -euo pipefail
    for lang in en fr de es ru sv; do
      echo "=== $lang ==="
      just games-refresh-lemmas {{TARGET}} "$lang"
    done
    echo ""
    echo "All done. Commit db/seeds/games_lemma_*.sql and redeploy, then click"
    echo "Load in /admin/games/fill-blank/maintenance for each language."

# Load the committed lemma fixture into game_lemma on a target DB. Idempotent.
# In prod prefer the editor's "Load lemma data" button, which loads the fixture
# embedded in the running binary. Hard-tier acceptance falls back to exact match
# until a non-empty fixture is loaded.
games-seed-lemma TARGET="local":
    #!/usr/bin/env bash
    set -euo pipefail
    fixture="db/seeds/games_lemma_en.sql"
    if [ ! -f "$fixture" ]; then
      echo "Fixture $fixture not found. Build it first: just games-build-lemma-fixture $1" >&2
      exit 1
    fi
    url="$(just url "$1")"
    psql "$url" --set ON_ERROR_STOP=1 -f "$fixture"
    echo "Loaded $fixture into game_lemma on $1."

# Propose passages for one chapter via GROQ. Requires GROQ_API_KEY in env.
# Writes rows to game_passages with status='proposed' for editor review.
games-curate-chapter CHAPTER:
    go run ./api/cmd/groq_curate/ --chapter "{{CHAPTER}}"

# Re-propose passages currently in rejected/needs_refinement for one chapter.
# Approved passages are untouched.
games-refine-rejected CHAPTER:
    go run ./api/cmd/groq_curate/ --chapter "{{CHAPTER}}" --refine-rejected

# Editorial review. With no args lists proposed passages; forwards any flags
# to the tool, e.g.:
#   just games-review --status all
#   just games-review --id 42
#   just games-review --id 42 --approve --by editor@example.com
#   just games-review --id 42 --reject
games-review *ARGS:
    go run ./api/cmd/games_review/ {{ARGS}}

# Full local rebuild: drop, set up, import legacy, seed
rebuild: db-drop db-setup
    just source-import local
    just seed local
    echo "Rebuild complete."

# Refresh root_index_mv on a target
refresh-roots TARGET="local":
    #!/usr/bin/env bash
    set -euo pipefail
    url="$(just url "$1")"
    psql "$url" --set ON_ERROR_STOP=1 -c "REFRESH MATERIALIZED VIEW CONCURRENTLY root_index_mv;"

# ---------------------------------------------------------------------------
# Docker
# ---------------------------------------------------------------------------

# Build the production Docker image
docker-build:
    docker build -t ws-backend:latest .

# Run the container locally (reads .env for DB vars)
docker-run: docker-build
    docker run --rm -p 8080:8080 --env-file .env -e APP_ENV=production ws-backend:latest
