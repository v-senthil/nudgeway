# Nudgeway — build, test, and dev targets.
# Everything runs against the user's LOCAL MySQL / Redis / HBase — no Docker, no K8s.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

GO           ?= go
GOFLAGS      ?=
GOOS         ?= $(shell $(GO) env GOOS)
GOARCH       ?= $(shell $(GO) env GOARCH)
PKGS         := ./...
BUILD_DIR    := ./bin
SERVER_BIN   := $(BUILD_DIR)/nudgeway
CLI_BIN      := $(BUILD_DIR)/nudgeway-cli
MCP_BIN      := $(BUILD_DIR)/nudgeway-mcp
CONFIG       ?= config/local.yaml
COVER_FILE   := coverage.out
COVER_MIN    ?= 60
COVER_DOMAIN_MIN ?= 80

TOOLS_BIN := $(shell $(GO) env GOPATH)/bin

## help: Show this help.
.PHONY: help
help:
	@awk 'BEGIN{FS=":.*## "; printf "\nTargets:\n"} /^## / {print "  " substr($$0,4)} /^[a-zA-Z0-9_-]+:.*## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

## check-infra: Verify local MySQL / Redis / HBase are reachable.
.PHONY: check-infra
check-infra:
	@./scripts/check-infra.sh $(CONFIG)

## deps: Download Go modules + install pinned tools.
.PHONY: deps
deps:
	$(GO) mod download
	@$(MAKE) tools

## tools: Install pinned dev tools (sqlc, oapi-codegen, golangci-lint, go-arch-lint, migrate).
.PHONY: tools
tools:
	@echo "Installing dev tools into $(TOOLS_BIN)..."
	GOBIN=$(TOOLS_BIN) $(GO) install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	GOBIN=$(TOOLS_BIN) $(GO) install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
	GOBIN=$(TOOLS_BIN) $(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	GOBIN=$(TOOLS_BIN) $(GO) install github.com/fe3dback/go-arch-lint@latest
	GOBIN=$(TOOLS_BIN) $(GO) install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

## fmt: Format Go + frontend code.
.PHONY: fmt
fmt:
	$(GO) fmt $(PKGS)
	@command -v goimports >/dev/null 2>&1 && goimports -w -local github.com/v-senthil/nudgeway . || true
	@[[ -d web/node_modules ]] && (cd web && npm run format) || true

## vet: Run go vet.
.PHONY: vet
vet:
	$(GO) vet $(PKGS)

## lint: Run golangci-lint + go-arch-lint.
.PHONY: lint
lint:
	$(TOOLS_BIN)/golangci-lint run ./...
	$(TOOLS_BIN)/go-arch-lint check --project-path .

## lint-openapi: Spectral lint of the OpenAPI spec.
.PHONY: lint-openapi
lint-openapi:
	@command -v npx >/dev/null 2>&1 || { echo "npx not found; install Node.js"; exit 1; }
	npx --yes @stoplight/spectral-cli lint internal/api/openapi/openapi.yaml --ruleset .spectral.yaml

## gen: Regenerate all code (sqlc queries, OpenAPI server + TS client).
.PHONY: gen
gen: gen-sqlc gen-api

## gen-sqlc: Regenerate typed queries from SQL.
.PHONY: gen-sqlc
gen-sqlc:
	$(TOOLS_BIN)/sqlc generate

## gen-api: Regenerate OpenAPI Go server + TS client.
.PHONY: gen-api
gen-api:
	$(TOOLS_BIN)/oapi-codegen -config internal/api/openapi/oapi-codegen.yaml internal/api/openapi/openapi.yaml
	@[[ -d web/node_modules ]] && (cd web && npm run gen:api) || echo "(skipped TS client gen — run 'cd web && npm install' first)"

## test: Run unit tests.
.PHONY: test
test:
	$(GO) test -race -coverprofile=$(COVER_FILE) -covermode=atomic $(PKGS)

## test-int: Run integration tests against local MySQL / Redis / HBase.
.PHONY: test-int
test-int: check-infra
	$(GO) test -race -tags=integration $(PKGS)

## test-frontend: Run Vitest.
.PHONY: test-frontend
test-frontend:
	cd web && npm run test

## e2e: Run Playwright golden-path suite.
.PHONY: e2e
e2e:
	cd web && npm run e2e

## coverage-check: Fail if coverage < thresholds.
.PHONY: coverage-check
coverage-check: test
	@total=$$($(GO) tool cover -func=$(COVER_FILE) | awk '/^total:/ {print substr($$3, 1, length($$3)-1)}'); \
	 echo "total coverage: $$total% (min $(COVER_MIN)%)"; \
	 awk -v t=$$total -v m=$(COVER_MIN) 'BEGIN{ exit (t+0 >= m+0) ? 0 : 1 }'

## verify: Everything CI runs.
.PHONY: verify
verify: fmt vet lint lint-openapi test coverage-check
	@echo "verify OK"

## migrate: Apply DB migrations. Use ARGS='down 1' etc. to override.
.PHONY: migrate
migrate:
	$(TOOLS_BIN)/migrate -path migrations -database "$$(./scripts/dsn-from-config.sh $(CONFIG))" $(ARGS)

## build: Build server + CLI binaries.
.PHONY: build
build: frontend
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags "-s -w" -o $(SERVER_BIN) ./cmd/server
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags "-s -w" -o $(CLI_BIN)    ./cmd/cli
	@ls -lh $(SERVER_BIN) $(CLI_BIN)

## mcp: Build the standalone MCP server (bin/nudgeway-mcp) that exposes every
##      OpenAPI operation as a Model Context Protocol tool over stdio.
.PHONY: mcp
mcp:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags "-s -w" -o $(MCP_BIN) ./cmd/mcp
	@ls -lh $(MCP_BIN)

## frontend: Build the Vite frontend into web/dist for embedding.
.PHONY: frontend
frontend:
	@[[ -d web/node_modules ]] || (cd web && npm install)
	cd web && npm run build

## dev: Run server + Vite dev server. Requires local MySQL/Redis/HBase.
.PHONY: dev
dev: check-infra
	@echo "Starting nudgeway server + vite dev server..."
	@trap 'kill 0' SIGINT SIGTERM EXIT; \
	 (cd web && npm run dev) & \
	 NUDGEWAY_CONFIG=$(CONFIG) $(GO) run ./cmd/server & \
	 wait

## clean: Remove build + coverage artefacts.
.PHONY: clean
clean:
	rm -rf $(BUILD_DIR) $(COVER_FILE) web/dist
