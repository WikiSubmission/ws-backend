include .env
export

PSQL=sudo -u postgres psql
BIN=bin/server

.PHONY: run build test tidy generate seed \
        db-setup db-drop db-rebuild \
        source-import source-cleanup \
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
	@echo "Creating user $(DATABASE_USER)..."
	@$(PSQL) -c "CREATE USER $(DATABASE_USER) WITH PASSWORD '$(DATABASE_PASSWORD)';" || echo "User already exists."
	@echo "Creating database $(DATABASE_NAME)..."
	@$(PSQL) -c "CREATE DATABASE $(DATABASE_NAME) OWNER $(DATABASE_USER);" || echo "Database already exists."
	@echo "Granting permissions..."
	@$(PSQL) -d $(DATABASE_NAME) -c "GRANT ALL PRIVILEGES ON DATABASE $(DATABASE_NAME) TO $(DATABASE_USER);"
	@$(PSQL) -d $(DATABASE_NAME) -c "GRANT USAGE, CREATE ON SCHEMA public TO $(DATABASE_USER);"
	@$(PSQL) -d $(DATABASE_NAME) -c "GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO $(DATABASE_USER);"
	@$(PSQL) -d $(DATABASE_NAME) -c "GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO $(DATABASE_USER);"
	@$(PSQL) -d $(DATABASE_NAME) -c "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO $(DATABASE_USER);"
	@$(PSQL) -d $(DATABASE_NAME) -c "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO $(DATABASE_USER);"
	@echo "Database setup complete."

## source-import: Load raw Supabase dumps into the public schema
source-import:
	@$(PSQL) -d $(DATABASE_NAME) -f assets/source_data/source_schema.sql
	@$(PSQL) -d $(DATABASE_NAME) -f assets/source_data/ws_quran_chapters_rows.sql
	@$(PSQL) -d $(DATABASE_NAME) -f assets/source_data/ws_quran_index_rows.sql
	@$(PSQL) -d $(DATABASE_NAME) -f assets/source_data/ws_quran_text_rows.sql
	@$(PSQL) -d $(DATABASE_NAME) -f assets/source_data/ws_quran_subtitles_rows.sql
	@$(PSQL) -d $(DATABASE_NAME) -f assets/source_data/ws_quran_footnotes_rows.sql
	@$(PSQL) -d $(DATABASE_NAME) -f assets/source_data/ws_quran_word_by_word_rows.sql
	@$(PSQL) -d $(DATABASE_NAME) -c "GRANT SELECT ON ALL TABLES IN SCHEMA public TO $(DATABASE_USER);"

## source-cleanup: Drop temporary source tables and remove local asset files
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

## db-drop: Remove the database and user entirely
db-drop:
	@$(PSQL) -c "DROP DATABASE IF EXISTS $(DATABASE_NAME);"
	@$(PSQL) -c "DROP USER IF EXISTS $(DATABASE_USER);"
	@echo "Database and user removed."

## db-rebuild: Full local rebuild (drop → setup → import → seed → cleanup)
db-rebuild: db-drop db-setup source-import seed source-cleanup
	@echo "Full rebuild complete."

# ---------------------------------------------------------------------------
# Misc
# ---------------------------------------------------------------------------

## clean: Remove compiled binaries and Go build cache
clean:
	@rm -rf bin/
	@go clean -cache
