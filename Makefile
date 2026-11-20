# Single entry points for local development and CI.
.PHONY: help hooks build run test fmt fmt-check vet lint vuln sec ci db-up db-down db-reset db-psql test-integration

# Tool versions pinned here; `go run pkg@version` avoids polluting go.mod and needs no global install.
GOLANGCI_LINT := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
GOVULNCHECK   := golang.org/x/vuln/cmd/govulncheck@latest
GOSEC         := github.com/securego/gosec/v2/cmd/gosec@v2.30.0

# The race detector needs cgo (a C compiler); enable it only where one exists (CI does).
RACE := $(shell command -v gcc >/dev/null 2>&1 && echo "-race")
TEST_ENV := $(if $(RACE),CGO_ENABLED=1,)

# PostgreSQL tests skip without it. Defaults to the compose database; override to use another.
TEST_DATABASE_URL ?= postgres://todo:todo@127.0.0.1:5432/todo?sslmode=disable

help: ## List targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

hooks: ## Enable the versioned git hooks (pre-commit runs `make ci`)
	git config core.hooksPath .githooks

build: ## Build the binary into ./bin
	CGO_ENABLED=0 go build -trimpath -o bin/todo-api ./cmd/todo-api

run: ## Run the service locally
	go run ./cmd/todo-api

test: ## Run the full test suite (with -race when cgo is available)
	$(TEST_ENV) go test $(RACE) ./...

test-integration: db-up ## Full suite including PostgreSQL tests, against the compose database
	$(TEST_ENV) TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go test $(RACE) -count=1 ./...

fmt: ## Format code
	gofmt -w .

fmt-check: ## Fail if any file is not gofmt-formatted
	@test -z "$$(gofmt -l .)" || (echo "not gofmt-formatted:"; gofmt -l .; exit 1)

vet: ## go vet
	go vet ./...

lint: ## golangci-lint
	go run $(GOLANGCI_LINT) run ./...

vuln: ## Known-vulnerability scan of dependencies
	go run $(GOVULNCHECK) ./...

sec: ## Static security analysis
	go run $(GOSEC) -quiet ./...

ci: fmt-check vet lint vuln sec test ## Everything CI runs, in order

db-up: ## Start local PostgreSQL and wait until it accepts connections
	docker compose up -d --wait postgres

db-down: ## Stop local PostgreSQL (data is kept in the named volume)
	docker compose down

db-reset: ## Stop local PostgreSQL and delete its data
	docker compose down -v

db-psql: ## Open a psql shell inside the local PostgreSQL container
	docker compose exec postgres sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'
