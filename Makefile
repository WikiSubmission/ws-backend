include .env
export


ifneq ($(DATABASE_URL),)
    # Railway / remote: DATABASE_URL already contains superuser credentials.
    PSQL_CONN=$(DATABASE_URL)
    PSQL_MAINT=$(shell echo $(DATABASE_URL) | sed 's/\/[^/]*$$/\/postgres/')
    PSQL_ADMIN=psql $(PSQL_MAINT) --set ON_ERROR_STOP=1
else
    # Local dev: app user for normal queries; OS postgres superuser for admin.
    PSQL_ADMIN=sudo -u postgres psql --set ON_ERROR_STOP=1
endif
# For general queries and imports: use PGPASSWORD so psql never prompts.
PSQL=PGPASSWORD=$(DATABASE_PASSWORD) psql \
    -U $(DATABASE_USER) \
    -h $(DATABASE_DOMAIN) \
    -p $(DATABASE_PORT) \
    -d $(DATABASE_NAME) \
    --set ON_ERROR_STOP=1

BIN=bin/server

.PHONY: run build test tidy generate seed seed-bible seed-admin \
        seed-music seed-music-railway embed-quran export-embeddings import-embeddings-railway \
        dump-db import-db-railway \
        db-setup db-drop db-rebuild \
        source-import source-cleanup \
        seed-railway reseed-railway \
        docker-build docker-run clean

# ---------------------------------------------------------------------------
# Development
# ---------------------------------------------------------------------------

## run: Start the API in development mode (pretty slog output)
run:
	@APP_ENV=development go run ./api/

## build: Compile a static binary to bin/server
build:
	@mkdir -p bin
	@CGO_ENABLED=0 go build -trimpath -ldflags="-w -s" -o $(BIN) ./api/
	@echo "Binary written to $(BIN)"

## test: Run all tests (DB tests require a live DATABASE_NAME env var)
test:
	@go test ./...

## tidy: Tidy the single root module
tidy:
	@go mod tidy

## generate: Regenerate OpenAPI types from api/openapi.yaml
##           Must be run from the workspace root.
generate:
	@go tool oapi-codegen -config oapi-codegen.yaml openapi.yaml

## seed: One-time data migration from legacy Supabase schema
seed:
	@go run api/cmd/seed.go

## seed-bible: Seed Bible data from assets/bible/ CSV files
seed-bible:
	@go run ./api/cmd/seed_bible/

## seed-music: Seed music artists, categories, and tracks from source SQL files (local)
seed-music:
	@echo "Dropping stale music source tables (if any)..."
	@$(PSQL_APP) -c "DROP TABLE IF EXISTS public.ws_music_tracks, public.ws_music_albums, public.ws_music_categories, public.ws_music_artists CASCADE;"
	@echo "Importing music source schemas..."
	@$(PSQL_APP) -f assets/source_data/ws_music_artists.sql
	@$(PSQL_APP) -f assets/source_data/ws_music_categories.sql
	@$(PSQL_APP) -f assets/source_data/ws_music_album.sql
	@$(PSQL_APP) -f assets/source_data/ws_music_tracks.sql
	@echo "Importing music source data..."
	@$(PSQL_APP) -f assets/ws_music_artists_rows.sql
	@$(PSQL_APP) -f assets/ws_music_categories_rows.sql
	@$(PSQL_APP) -f assets/ws_music_tracks_rows.sql
	@echo "Transforming into app tables..."
	@$(PSQL_APP) -f assets/source_data/music_transform.sql
	@echo "Cleaning up music source tables..."
	@$(PSQL_APP) -c "DROP TABLE IF EXISTS public.ws_music_tracks, public.ws_music_albums, public.ws_music_categories, public.ws_music_artists CASCADE;"
	@echo "Music seed complete."

## seed-music-railway: Seed music on Railway (requires DATABASE_URL in .env)
seed-music-railway:
	@test -n "$(DATABASE_URL)" || (echo "ERROR: DATABASE_URL is not set in .env" && exit 1)
	@echo "Dropping stale music source tables (if any)..."
	@$(PSQL_REMOTE) -c "DROP TABLE IF EXISTS public.ws_music_tracks, public.ws_music_albums, public.ws_music_categories, public.ws_music_artists CASCADE;"
	@echo "Importing music source schemas..."
	@$(PSQL_REMOTE) -f assets/source_data/ws_music_artists.sql
	@$(PSQL_REMOTE) -f assets/source_data/ws_music_categories.sql
	@$(PSQL_REMOTE) -f assets/source_data/ws_music_album.sql
	@$(PSQL_REMOTE) -f assets/source_data/ws_music_tracks.sql
	@echo "Importing music source data..."
	@$(PSQL_REMOTE) -f assets/ws_music_artists_rows.sql
	@$(PSQL_REMOTE) -f assets/ws_music_categories_rows.sql
	@$(PSQL_REMOTE) -f assets/ws_music_tracks_rows.sql
	@echo "Transforming into app tables..."
	@$(PSQL_REMOTE) -f assets/source_data/music_transform.sql
	@echo "Cleaning up music source tables..."
	@$(PSQL_REMOTE) -c "DROP TABLE IF EXISTS public.ws_music_tracks, public.ws_music_albums, public.ws_music_categories, public.ws_music_artists CASCADE;"
	@echo "Music seed complete (Railway)."

## embed-quran: Generate and store embeddings for all English verse_text rows.
##              Requires OPENAI_API_KEY in .env. Idempotent — skips rows already embedded.
##              Cost: ~$0.004 total (text-embedding-3-small, 512 dims).
embed-quran:
	@test -n "$(OPENAI_API_KEY)" || (echo "ERROR: OPENAI_API_KEY is not set in .env" && exit 1)
	@go run ./api/cmd/seed_embeddings/

## export-embeddings: Dump (id, embedding) pairs from the local DB to assets/embeddings.csv.
##                    Run this after embed-quran to snapshot embeddings for prod upload.
export-embeddings:
	@echo "Exporting embeddings from local database..."
	@$(PSQL) -c "\COPY (SELECT id, embedding::text FROM verse_text WHERE embedding IS NOT NULL) TO '$(CURDIR)/assets/embeddings.csv' WITH (FORMAT CSV)"
	@echo "Exported $$(wc -l < $(CURDIR)/assets/embeddings.csv | tr -d ' ') rows to assets/embeddings.csv"

## dump-db: Snapshot local DB to assets/ (migration tables only, no users, no embeddings).
##          Runs as postgres superuser (sudo) to bypass app-user permission limits.
##          Output: assets/db_dump.sql + assets/verse_text.csv (embedding column excluded)
dump-db:
	@echo "Dumping migration tables (excluding users and embeddings)..."
	@sudo -u postgres pg_dump \
		--data-only \
		--format=plain \
		--table=languages --table=chapters --table=verses --table=words \
		--table=chapter_text --table=word_text \
		--table=chapter_text_drafts --table=verse_text_drafts --table=word_text_drafts \
		--table=chapter_text_history --table=verse_text_history --table=word_text_history \
		--table=appendices \
		--table=bible_books --table=bible_chapters --table=bible_verses \
		--table=bible_verse_text --table=bible_book_text \
		--table=music_artists --table=music_categories \
		--table=music_albums --table=music_tracks \
		$(DATABASE_NAME) > assets/db_dump.sql
	@echo "Exporting verse_text (without embedding column)..."
	@$(PSQL) -c "\COPY (SELECT id, verse_id, lang_id, subtitle, content, footer, published_at FROM verse_text ORDER BY id) TO '$(CURDIR)/assets/verse_text.csv' WITH (FORMAT CSV)"
	@echo "Done: db_dump=$$(du -sh assets/db_dump.sql | cut -f1), verse_text=$$(wc -l < assets/verse_text.csv | tr -d ' ') rows"

## import-db-railway: Full data refresh on Railway (users preserved, embeddings via separate target).
##                   Requires DATABASE_URL in .env. Run dump-db first.
import-db-railway:
	@test -n "$(DATABASE_URL)" || (echo "ERROR: DATABASE_URL is not set in .env" && exit 1)
	@test -f assets/db_dump.sql || (echo "ERROR: assets/db_dump.sql not found — run make dump-db first" && exit 1)
	@test -f assets/verse_text.csv || (echo "ERROR: assets/verse_text.csv not found — run make dump-db first" && exit 1)
	@echo "Truncating content tables on Railway (users preserved)..."
	@psql "$(DATABASE_URL)" -c "TRUNCATE TABLE \
		music_tracks, music_albums, music_categories, music_artists, \
		bible_verse_text, bible_book_text, bible_verses, bible_chapters, bible_books, \
		word_text_history, verse_text_history, chapter_text_history, \
		word_text_drafts, verse_text_drafts, chapter_text_drafts, \
		word_text, verse_text, chapter_text, \
		appendices, words, verses, chapters, languages \
		RESTART IDENTITY CASCADE;"
	@echo "Restoring main tables..."
	@psql "$(DATABASE_URL)" --single-transaction -f assets/db_dump.sql
	@echo "Restoring verse_text (without embeddings)..."
	@psql "$(DATABASE_URL)" -c "ALTER TABLE verse_text ALTER COLUMN id SET GENERATED BY DEFAULT;"
	@psql "$(DATABASE_URL)" -c "\COPY verse_text(id, verse_id, lang_id, subtitle, content, footer, published_at) FROM '$(CURDIR)/assets/verse_text.csv' WITH (FORMAT CSV)"
	@psql "$(DATABASE_URL)" -c "ALTER TABLE verse_text ALTER COLUMN id SET GENERATED ALWAYS; SELECT setval(pg_get_serial_sequence('verse_text','id'), (SELECT MAX(id) FROM verse_text));"
	@echo "Import complete. Run: make import-embeddings-railway to restore search embeddings."

## import-embeddings-railway: Upload assets/embeddings.csv to Railway (UPDATE only, no re-embedding).
##                             Requires DATABASE_URL in .env. Idempotent.
import-embeddings-railway:
	@test -n "$(DATABASE_URL)" || (echo "ERROR: DATABASE_URL is not set in .env" && exit 1)
	@test -f assets/embeddings.csv || (echo "ERROR: assets/embeddings.csv not found — run make export-embeddings first" && exit 1)
	@echo "Creating staging table on Railway..."
	@psql $(DATABASE_URL) -c "DROP TABLE IF EXISTS _emb_stage; CREATE TABLE _emb_stage (id INT PRIMARY KEY, embedding vector(512));"
	@echo "Uploading embeddings CSV (this may take a minute)..."
	@psql $(DATABASE_URL) -c "\COPY _emb_stage (id, embedding) FROM '$(CURDIR)/assets/embeddings.csv' WITH (FORMAT CSV)"
	@echo "Updating verse_text..."
	@psql $(DATABASE_URL) -c "UPDATE verse_text v SET embedding = e.embedding FROM _emb_stage e WHERE v.id = e.id; DROP TABLE _emb_stage;"
	@echo "Import complete."

## seed-admin: Promote a user to admin role by email
##             Usage: make seed-admin EMAIL=user@example.com
seed-admin:
	@test -n "$(EMAIL)" || (echo "ERROR: EMAIL is not set. Usage: make seed-admin EMAIL=user@example.com" && exit 1)
	@go run ./api/cmd/seed_admin/ --email "$(EMAIL)"

# ---------------------------------------------------------------------------
# Docker
# ---------------------------------------------------------------------------

## docker-build: Build the production Docker image
docker-build:
	@docker build -t ws-backend:latest .

## docker-run: Run the container locally (reads .env for DB vars)
docker-run: docker-build
	@docker run --rm -p 8080:8080 \
		--env-file .env \
		-e APP_ENV=production \
		ws-backend:latest

# ---------------------------------------------------------------------------
# Database management (local / CI)
# ---------------------------------------------------------------------------

## db-setup: Create PostgreSQL user, database, and grant permissions
db-setup:
	@echo "Creating user and database via $(PSQL_MAINT)..."
	@$(PSQL_ADMIN) -c "CREATE USER $(DATABASE_USER) WITH PASSWORD '$(DATABASE_PASSWORD)';" || echo "User already exists."
	@$(PSQL_ADMIN) -c "CREATE DATABASE $(DATABASE_NAME) OWNER $(DATABASE_USER);" || echo "Database already exists."
	@echo "Granting permissions on $(DATABASE_NAME)..."
	@$(PSQL) -c "GRANT ALL PRIVILEGES ON DATABASE $(DATABASE_NAME) TO $(DATABASE_USER);"
	@$(PSQL) -c "GRANT USAGE, CREATE ON SCHEMA public TO $(DATABASE_USER);"
	@echo "Database setup complete."

source-import:
	@echo "Importing source data into $(DATABASE_NAME)..."
	@$(PSQL) -f assets/source_data/source_schema.sql
	@$(PSQL) -f assets/source_data/ws_quran_chapters_rows.sql
	@$(PSQL) -f assets/source_data/ws_quran_footnotes_rows.sql
	@$(PSQL) -f assets/source_data/ws_quran_index_rows.sql
	@$(PSQL) -f assets/source_data/ws_quran_subtitles_rows.sql
	@$(PSQL) -f assets/source_data/ws_quran_text_rows.sql
	@$(PSQL) -f assets/source_data/ws_quran_word_by_word_rows.sql
	@$(PSQL) -c "GRANT SELECT ON ALL TABLES IN SCHEMA public TO $(DATABASE_USER);"

## db-drop: Remove the database and user entirely
db-drop:
	@echo "Dropping database $(DATABASE_NAME) via $(PSQL_MAINT)..."
	@$(PSQL_ADMIN) -c "DROP DATABASE IF EXISTS $(DATABASE_NAME);"
	@$(PSQL_ADMIN) -c "DROP USER IF EXISTS $(DATABASE_USER);"

source-cleanup:
	@$(PSQL) -d $(DATABASE_NAME) -c "DROP TABLE IF EXISTS \
		public.ws_quran_chapters, \
		public.ws_quran_index, \
		public.ws_quran_text, \
		public.ws_quran_subtitles, \
		public.ws_quran_word_by_word, \
		public.ws_quran_footnotes CASCADE;"
	@rm -rf assets/source_data
	@echo "Source cleanup complete."


## db-rebuild: Full local rebuild (drop → setup → import → seed → cleanup)
db-rebuild: db-drop db-setup source-cleanup source-import seed
	@echo "Full rebuild complete."

# ---------------------------------------------------------------------------
# Railway (remote) seeding — run locally, connects via DATABASE_URL
# ---------------------------------------------------------------------------

PSQL_REMOTE=psql $(DATABASE_URL) --set ON_ERROR_STOP=1 --single-transaction

# PSQL_APP mirrors ConnectDB() priority: DATABASE_URL > individual vars.
# Ensures seed-music targets the same DB as the running app.
ifneq ($(DATABASE_URL),)
PSQL_APP=psql $(DATABASE_URL) --set ON_ERROR_STOP=1
else
PSQL_APP=$(PSQL)
endif

## seed-railway: First-time seed of the Railway database.
##               Set DATABASE_URL in .env to the Railway Postgres URL before running.
seed-railway:
	@test -n "$(DATABASE_URL)" || (echo "ERROR: DATABASE_URL is not set in .env" && exit 1)
	@echo "Dropping stale source tables (if any)..."
	@$(PSQL_REMOTE) -c "DROP TABLE IF EXISTS public.ws_quran_chapters, public.ws_quran_index, public.ws_quran_text, public.ws_quran_subtitles, public.ws_quran_word_by_word, public.ws_quran_footnotes CASCADE;"
	@echo "Importing source data into Railway..."
	@$(PSQL_REMOTE) -f assets/source_data/source_schema.sql
	@$(PSQL_REMOTE) -f assets/source_data/ws_quran_chapters_rows.sql
	@$(PSQL_REMOTE) -f assets/source_data/ws_quran_footnotes_rows.sql
	@$(PSQL_REMOTE) -f assets/source_data/ws_quran_index_rows.sql
	@$(PSQL_REMOTE) -f assets/source_data/ws_quran_subtitles_rows.sql
	@$(PSQL_REMOTE) -f assets/source_data/ws_quran_text_rows.sql
	@$(PSQL_REMOTE) -f assets/source_data/ws_quran_word_by_word_rows.sql
	@echo "Running seed migration against Railway..."
	@DATABASE_URL=$(DATABASE_URL) go run api/cmd/seed.go
	@echo "Cleaning up source tables on Railway..."
	@$(PSQL_REMOTE) -c "DROP TABLE IF EXISTS \
		public.ws_quran_chapters, \
		public.ws_quran_index, \
		public.ws_quran_text, \
		public.ws_quran_subtitles, \
		public.ws_quran_word_by_word, \
		public.ws_quran_footnotes CASCADE;"
	@echo "Railway seed complete."

## reseed-railway: Re-seed Railway without dropping tables (truncates data only).
##                 Use when seed failed mid-way or data needs to be refreshed.
reseed-railway:
	@test -n "$(DATABASE_URL)" || (echo "ERROR: DATABASE_URL is not set in .env" && exit 1)
	@echo "Truncating app tables on Railway (schema preserved)..."
	@$(PSQL_REMOTE) -c "TRUNCATE TABLE \
		languages, chapters, verses, words, \
		chapter_text, verse_text, word_text \
		RESTART IDENTITY CASCADE;"
	@echo "Dropping stale source tables (if any)..."
	@$(PSQL_REMOTE) -c "DROP TABLE IF EXISTS public.ws_quran_chapters, public.ws_quran_index, public.ws_quran_text, public.ws_quran_subtitles, public.ws_quran_word_by_word, public.ws_quran_footnotes CASCADE;"
	@echo "Re-importing source data..."
	@$(PSQL_REMOTE) -f assets/source_data/source_schema.sql
	@$(PSQL_REMOTE) -f assets/source_data/ws_quran_chapters_rows.sql
	@$(PSQL_REMOTE) -f assets/source_data/ws_quran_footnotes_rows.sql
	@$(PSQL_REMOTE) -f assets/source_data/ws_quran_index_rows.sql
	@$(PSQL_REMOTE) -f assets/source_data/ws_quran_subtitles_rows.sql
	@$(PSQL_REMOTE) -f assets/source_data/ws_quran_text_rows.sql
	@$(PSQL_REMOTE) -f assets/source_data/ws_quran_word_by_word_rows.sql
	@echo "Running seed migration against Railway..."
	@DATABASE_URL=$(DATABASE_URL) go run api/cmd/seed.go
	@$(PSQL_REMOTE) -c "DROP TABLE IF EXISTS \
		public.ws_quran_chapters, \
		public.ws_quran_index, \
		public.ws_quran_text, \
		public.ws_quran_subtitles, \
		public.ws_quran_word_by_word, \
		public.ws_quran_footnotes CASCADE;"
	@echo "Railway reseed complete."

# ---------------------------------------------------------------------------
# Misc
# ---------------------------------------------------------------------------

## clean: Remove compiled binaries and Go build cache
clean:
	@rm -rf bin/
	@go clean -cache
