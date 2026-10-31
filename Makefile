SERVER_DIR := server
WEB_DIR    := web
COMPOSE    := docker compose -f deploy/docker/compose.dev.yaml
GOLANGCI_LINT_VERSION := v2.15.0
GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

# Local defaults match deploy/docker/compose.dev.yaml; CI overrides them.
ADMIN_DATABASE_URL      ?= postgres://admin:admin@127.0.0.1:15432/base_admin?sslmode=disable
ADMIN_REDIS_ADDR        ?= 127.0.0.1:16380
ADMIN_TEST_DATABASE_URL ?= $(ADMIN_DATABASE_URL)
# Tests lease free Redis databases 1-15 and never touch database 0 or ones
# holding other data, so sharing the development Redis is safe.
ADMIN_TEST_REDIS_ADDR   ?= $(ADMIN_REDIS_ADDR)
# Local development only: plain-HTTP cookies and a known first password
# (the admin must change it at first login).
ADMIN_AUTH_COOKIE_SECURE        ?= false
ADMIN_BOOTSTRAP_ADMIN_PASSWORD  ?= Admin@dev-2026
export ADMIN_DATABASE_URL ADMIN_REDIS_ADDR ADMIN_TEST_DATABASE_URL ADMIN_TEST_REDIS_ADDR
export ADMIN_AUTH_COOKIE_SECURE ADMIN_BOOTSTRAP_ADMIN_PASSWORD

# The Vite dev server proxies /api here; override if :8080 is taken.
API_URL ?= http://127.0.0.1:8080

.PHONY: help deps-up deps-down run migrate-up new-migration migrate-version codegen-web-check build test test-integration cover lint fmt ci \
	web-install web-dev web-check web-build e2e

help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-18s %s\n", $$1, $$2}'

deps-up: ## Start PostgreSQL and Redis for local development
	$(COMPOSE) up -d --wait

deps-down: ## Stop local dependencies (data volumes are kept)
	$(COMPOSE) down

run: ## Run the API server with console logs
	cd $(SERVER_DIR) && ADMIN_LOG_FORMAT=console go run ./cmd/admin serve

migrate-up: ## Apply database migrations
	cd $(SERVER_DIR) && go run ./cmd/admin migrate up

new-migration: ## Create a migration pair in server/migrations/app; NAME=create_<table> starts from the table template (TRACK=base for scaffold migrations)
	@sh scripts/new-migration.sh "$(NAME)" "$(TRACK)"

migrate-version: ## Print the schema version of each track
	cd $(SERVER_DIR) && go run ./cmd/admin migrate version

build: ## Compile everything, including integration tests
	cd $(SERVER_DIR) && go build ./... && go vet -tags integration ./...

test: ## Unit tests
	cd $(SERVER_DIR) && go test -race ./...

test-integration: ## Unit + integration tests (needs deps-up)
	cd $(SERVER_DIR) && go test -race -tags integration ./...

cover: ## Coverage report including integration tests (needs deps-up)
	cd $(SERVER_DIR) && go test -tags integration -coverpkg=./internal/...,./cmd/... -coverprofile=coverage.out \
		$$(go list -tags integration -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./...) \
		&& go tool cover -func=coverage.out | tail -1

lint: ## Static checks
	cd $(SERVER_DIR) && $(GOLANGCI_LINT) run ./...

fmt: ## Format code
	cd $(SERVER_DIR) && $(GOLANGCI_LINT) fmt ./...

ci: build lint test-integration web-check web-build ## Everything CI runs

web-install: ## Install frontend dependencies
	cd $(WEB_DIR) && pnpm install --frozen-lockfile

web-dev: ## Run the frontend on :8848, proxying /api to API_URL
	cd $(WEB_DIR) && VITE_API_PROXY_TARGET=$(API_URL) pnpm dev

web-check: ## Frontend lint, type-check and unit tests (check only, nothing is rewritten)
	cd $(WEB_DIR) && pnpm lint && pnpm typecheck && pnpm test

web-build: ## Frontend production build into web/dist
	cd $(WEB_DIR) && pnpm build

codegen-web-check: ## Format, lint and type-check the frontend the code generator writes (needs web/node_modules)
	cd $(SERVER_DIR) && CODEGEN_WEB_CHECK=1 go test -count=1 -run TestGeneratedWebChecks ./internal/devtools/codegen/

e2e: ## Playwright E2E (needs a running backend; E2E_ADMIN_USERNAME/E2E_ADMIN_PASSWORD)
	cd $(WEB_DIR) && VITE_API_PROXY_TARGET=$(API_URL) pnpm test:e2e
