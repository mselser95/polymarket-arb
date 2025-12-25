.PHONY: help build lint test test-unit test-bench test-race test-all test-execution test-execution-verbose test-execution-coverage run run-single list-markets watch clean
.PHONY: docker-build docker-up docker-down docker-logs docker-clean
.PHONY: migrate-up migrate-down db-shell dev

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary
	@echo "Building polymarket-arb..."
	@go build -o polymarket-arb .

lint: ## Run golangci-lint
	@echo "Running linter..."
	@golangci-lint run --timeout=5m ./...

test: ## Run tests
	@echo "Running tests..."
	@go test -v -race -cover ./...

test-unit: ## Run unit tests without race detector
	@echo "Running unit tests..."
	@go test -v -cover ./...

test-race: ## Run tests with race detector
	@echo "Running tests with race detector..."
	@go test -v -race ./...

test-bench: ## Run benchmarks
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem ./...

test-coverage: ## Generate coverage report
	@echo "Generating coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-integration: ## Run integration tests (E2E)
	@echo "Running integration tests..."
	@go test -tags=integration -v -race ./...

test-execution: ## Run execution package tests
	@echo "Running execution tests..."
	@go test -v -race -cover ./internal/execution/...

test-execution-verbose: ## Run execution tests with detailed output
	@echo "Running execution tests with verbose output..."
	@go test -v -race -cover ./internal/execution/... -args -test.v

test-execution-coverage: ## Generate execution coverage report
	@echo "Generating execution coverage report..."
	@go test -coverprofile=execution-coverage.out ./internal/execution/...
	@go tool cover -html=execution-coverage.out -o execution-coverage.html
	@echo "Coverage report: execution-coverage.html"

test-all: test-unit test-integration test-race ## Run all tests

run: ## Run the bot locally (console mode)
	@echo "Running bot in console mode..."
	@STORAGE_MODE=console go run . run

run-single: ## Run bot on single market (pass MARKET=slug)
	@echo "Running bot on single market: $(MARKET)"
	@STORAGE_MODE=console go run . run --single-market $(MARKET)

list-markets: ## List active Polymarket markets
	@go run . list-markets --limit 20

watch: ## Watch orderbook for a market (pass MARKET=slug)
	@go run . watch-orderbook $(MARKET)

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -f polymarket-arb
	@go clean

# Docker commands
docker-build: ## Build Docker image
	@echo "Building Docker image..."
	@docker-compose build

docker-up: ## Start services with Docker Compose
	@echo "Starting services..."
	@docker-compose up -d
	@echo "Waiting for database..."
	@sleep 3
	@echo "Services started! Logs: make docker-logs"

docker-down: ## Stop services
	@echo "Stopping services..."
	@docker-compose down

docker-logs: ## Show logs
	@docker-compose logs -f app

docker-clean: ## Stop and remove volumes (⚠️  deletes data!)
	@echo "⚠️  This will delete all data!"
	@read -p "Continue? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		docker-compose down -v; \
		echo "Volumes deleted."; \
	fi

# Database commands
migrate-up: ## Run database migrations (inside Docker)
	@echo "Running migrations..."
	@docker-compose exec postgres psql -U polymarket -d polymarket_arb -f /docker-entrypoint-initdb.d/001_initial_schema.up.sql

migrate-down: ## Rollback database migrations (inside Docker)
	@echo "Rolling back migrations..."
	@docker-compose exec postgres psql -U polymarket -d polymarket_arb -f /docker-entrypoint-initdb.d/001_initial_schema.down.sql

db-shell: ## Open PostgreSQL shell
	@docker-compose exec postgres psql -U polymarket -d polymarket_arb

# Development
dev: ## Run locally with live reload (requires air)
	@air

