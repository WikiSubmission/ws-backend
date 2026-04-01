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

.PHONY: run build test tidy generate seed seed-bible \
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

PSQL_REMOTE=psql $(DATABASE_URL) --set ON_ERROR_STOP=1

## seed-railway: First-time seed of the Railway database.
##               Set DATABASE_URL in .env to the Railway Postgres URL before running.
seed-railway:
	@test -n "$(DATABASE_URL)" || (echo "ERROR: DATABASE_URL is not set in .env" && exit 1)
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
