# Single entry points for local development and CI.
.PHONY: help build run test fmt fmt-check vet lint vuln sec ci

# Tool versions pinned here; `go run pkg@version` avoids polluting go.mod and needs no global install.
GOLANGCI_LINT := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
GOVULNCHECK   := golang.org/x/vuln/cmd/govulncheck@latest
GOSEC         := github.com/securego/gosec/v2/cmd/gosec@v2.30.0

# The race detector needs cgo (a C compiler); enable it only where one exists (CI does).
RACE := $(shell command -v gcc >/dev/null 2>&1 && echo "-race")
TEST_ENV := $(if $(RACE),CGO_ENABLED=1,)

help: ## List targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

build: ## Build the binary into ./bin
	CGO_ENABLED=0 go build -trimpath -o bin/todo-api ./cmd/todo-api

run: ## Run the service locally
	go run ./cmd/todo-api

test: ## Run the full test suite (with -race when cgo is available)
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
