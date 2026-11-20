# Single entry points for local development and CI.
.PHONY: help hooks build run test fmt fmt-check vet lint vuln sec ci db-up db-psql up down reset docker-build

# Tool versions pinned here; `go run pkg@version` avoids polluting go.mod and needs no global install.
GOLANGCI_LINT := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
GOVULNCHECK   := golang.org/x/vuln/cmd/govulncheck@latest
GOSEC         := github.com/securego/gosec/v2/cmd/gosec@v2.30.0

# The race detector needs cgo (a C compiler); enable it only where one exists (CI does).
RACE := $(shell command -v gcc >/dev/null 2>&1 && echo "-race")
TEST_ENV := $(if $(RACE),CGO_ENABLED=1,)

# The compose database (compose.yaml defaults); override from the environment to use another.
DATABASE_URL ?= postgres://todo:todo@127.0.0.1:5432/todo?sslmode=disable

help: ## List targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

hooks: ## Enable the versioned git hooks (pre-commit runs `make ci`)
	git config core.hooksPath .githooks

build: ## Build the binary into ./bin
	CGO_ENABLED=0 go build -trimpath -o bin/todo-api ./cmd/todo-api

run: db-up ## Run the service locally against the compose database
	DATABASE_URL='$(DATABASE_URL)' go run ./cmd/todo-api

test: ## Run the full test suite (with -race when cgo is available; needs Docker for PostgreSQL tests)
	$(TEST_ENV) go test $(RACE) ./...

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

docker-build: ## Build the service image (todo-api:dev)
	docker build -t todo-api:dev .

up: ## Build and start the whole stack (API + PostgreSQL) with docker compose
	docker compose up --build -d --wait

down: ## Stop the stack (data is kept in the named volume)
	docker compose down

reset: ## Stop the stack and delete the database data
	docker compose down -v

db-up: ## Start only PostgreSQL and wait until it accepts connections (for `make run`)
	docker compose up -d --wait postgres

db-psql: ## Open a psql shell inside the PostgreSQL container
	docker compose exec postgres sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'
