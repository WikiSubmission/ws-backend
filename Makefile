include .env
export

# PostgreSQL Command Utility
# We use 'sudo -u postgres' to ensure we have permission to create users/DBs
PSQL=sudo -u postgres psql
GO_FILES=$(wildcard api/*.go)

.PHONY: db-setup db-drop db-rebuild run tidy clean seed source-import

## db-setup: Create user, database, and set permissions
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

## source-import: Import the raw Supabase dumps into the public schema
source-import:
	@echo "Creating temporary source schema..."
	@$(PSQL) -d $(DATABASE_NAME) -f assets/source_data/source_schema.sql
	@echo "Importing source data from Supabase dumps..."
	@$(PSQL) -d $(DATABASE_NAME) -f assets/source_data/ws_quran_chapters_rows.sql
	@$(PSQL) -d $(DATABASE_NAME) -f assets/source_data/ws_quran_index_rows.sql
	@$(PSQL) -d $(DATABASE_NAME) -f assets/source_data/ws_quran_text_rows.sql
	@$(PSQL) -d $(DATABASE_NAME) -f assets/source_data/ws_quran_subtitles_rows.sql
	@$(PSQL) -d $(DATABASE_NAME) -f assets/source_data/ws_quran_word_by_word_rows.sql
	@echo "Granting select on source tables to $(DATABASE_USER)..."
	@$(PSQL) -d $(DATABASE_NAME) -c "GRANT SELECT ON ALL TABLES IN SCHEMA public TO $(DATABASE_USER);"

## db-drop: Remove the database and user
db-drop:
	@echo "Dropping database $(DATABASE_NAME)..."
	@$(PSQL) -c "DROP DATABASE IF EXISTS $(DATABASE_NAME);"
	@echo "Dropping user $(DATABASE_USER)..."
	@$(PSQL) -c "DROP USER IF EXISTS $(DATABASE_USER);"
	@echo "Database and user removed."

## db-rebuild: Drop and setup again
db-rebuild: db-drop db-setup source-import seed
	@echo "Full rebuild complete. Data migrated and normalized."

tidy:
	@echo "Tidying modules..."
	@go work sync
	@cd api && go mod tidy
	@cd db && go mod tidy

## run: Run the API in development mode with pretty logs
run:
	@echo "Starting API in development mode..."
	@APP_ENV=development go run $(GO_FILES)

## build: Compile the binary
build:
	@echo "Building binary..."
	@go build -o bin/api api/*.go

## clean: Remove binaries and cache
clean:
	@rm -rf bin/
	@go clean -cache

## generate: Generate openapi types 
generate:
	@oapi-codegen --config=api/oapi-codegen.yaml api/openapi.yaml

## seed: Seed data 
seed:
	@go run api/cmd/seed.go