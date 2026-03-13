include .env
export


ifneq ($(DATABASE_URL),)
    PSQL_CONN=$(DATABASE_URL)
    # Extract the base connection without the DB name for setup/drop tasks
    # This replaces the DB name at the end with 'postgres'
    PSQL_MAINT=$(shell echo $(DATABASE_URL) | sed 's/\/[^/]*$$/\/postgres/')
else
    PSQL_BASE=postgresql://$(DATABASE_USER):$(DATABASE_PASSWORD)@$(DATABASE_HOST):$(DATABASE_PORT)
    PSQL_CONN=$(PSQL_BASE)/$(DATABASE_NAME)
    PSQL_MAINT=$(PSQL_BASE)/postgres
endif
# For general queries and imports
PSQL=psql $(PSQL_CONN) --set ON_ERROR_STOP=1
# For creating/dropping the database itself
PSQL_ADMIN=psql $(PSQL_MAINT) --set ON_ERROR_STOP=1

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

db-drop:
	@echo "Dropping database $(DATABASE_NAME) via $(PSQL_MAINT)..."
	@$(PSQL_ADMIN) -c "DROP DATABASE IF EXISTS $(DATABASE_NAME);"
	@$(PSQL_ADMIN) -c "DROP USER IF EXISTS $(DATABASE_USER);"

source-cleanup:
	@$(PSQL) -c "DROP TABLE IF EXISTS public.ws_quran_chapters, public.ws_quran_index, ... CASCADE;"

## db-rebuild: Full local rebuild (drop → setup → import → seed → cleanup)
db-rebuild: db-setup source-import seed source-cleanup
	@echo "Full rebuild complete."

# ---------------------------------------------------------------------------
# Misc
# ---------------------------------------------------------------------------

## clean: Remove compiled binaries and Go build cache
clean:
	@rm -rf bin/
	@go clean -cache
