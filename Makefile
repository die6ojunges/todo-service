# Single entry points for local development and CI.
.PHONY: help hooks build run test fmt fmt-check vet lint vuln sec ci db-up db-psql up down reset docker-build k8s-secret kind-up kind-deploy kind-port-forward kind-down do-auth do-up do-deploy do-url do-down

# Tool versions pinned here; `go run pkg@version` avoids polluting go.mod and needs no global install.
GOLANGCI_LINT := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
GOVULNCHECK   := golang.org/x/vuln/cmd/govulncheck@latest
GOSEC         := github.com/securego/gosec/v2/cmd/gosec@v2.30.0
KIND          := go run sigs.k8s.io/kind@v0.33.0
DOCTL         := go run github.com/digitalocean/doctl/cmd/doctl@v1.186.0

# The race detector needs cgo (a C compiler); enable it only where one exists (CI does).
RACE := $(shell command -v gcc >/dev/null 2>&1 && echo "-race")
TEST_ENV := $(if $(RACE),CGO_ENABLED=1,)

# The compose database (compose.yaml defaults); override from the environment to use another.
DATABASE_URL ?= postgres://todo:todo@127.0.0.1:5432/todo?sslmode=disable

help: ## List targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-18s %s\n", $$1, $$2}'

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

# Every kubectl call names the kind context explicitly, so a deploy can never hit another cluster.
KIND_CLUSTER := todo
KUBECTL_KIND := kubectl --context kind-$(KIND_CLUSTER)
K8S_SECRET_ENV := deploy/k8s/postgres.env

k8s-secret: ## Create the gitignored Kubernetes credentials file with a random password (kept if present)
	@test -f $(K8S_SECRET_ENV) || { sed "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$$(openssl rand -hex 16)/" \
		deploy/k8s/postgres.env.example > $(K8S_SECRET_ENV) && echo "created $(K8S_SECRET_ENV)"; }

kind-up: ## Create the local kind cluster (no-op if it exists)
	@$(KIND) get clusters 2>/dev/null | grep -qx $(KIND_CLUSTER) || $(KIND) create cluster --name $(KIND_CLUSTER) --wait 60s

kind-deploy: kind-up docker-build k8s-secret ## Build the image, load it into kind and apply the manifests
	$(KIND) load docker-image todo-api:dev --name $(KIND_CLUSTER)
	$(KUBECTL_KIND) apply -k deploy/k8s
	$(KUBECTL_KIND) -n todo rollout status statefulset/postgres --timeout=120s
	$(KUBECTL_KIND) -n todo rollout restart deployment/todo-api
	$(KUBECTL_KIND) -n todo rollout status deployment/todo-api --timeout=120s

kind-port-forward: ## Expose the kind Service on localhost:8080 (LoadBalancer stays pending on kind)
	$(KUBECTL_KIND) -n todo port-forward service/todo-api 8080:80

kind-down: ## Delete the local kind cluster
	$(KIND) delete cluster --name $(KIND_CLUSTER)

# DigitalOcean demo (README "Cloud demo"). Billed hourly: run `make do-down` when done.
DO_REGION   ?= nyc3
DO_CLUSTER  ?= todo
DO_NODES    ?= 2
DO_SIZE     ?= s-1vcpu-2gb
# Registry names are global on DigitalOcean; override if this one is taken.
DO_REGISTRY ?= die6ojunges-todo
# Tagging with the commit makes every deploy traceable and forces a rollout.
DO_IMAGE    := registry.digitalocean.com/$(DO_REGISTRY)/todo-api:$(shell git rev-parse --short HEAD)
KUBECTL_DO  := kubectl --context do-$(DO_REGION)-$(DO_CLUSTER)

do-auth: ## Store a DigitalOcean API token for doctl (interactive, once)
	$(DOCTL) auth init

do-up: ## Create the registry and the DOKS cluster (costs money while it exists)
	$(DOCTL) registry get $(DO_REGISTRY) >/dev/null 2>&1 || \
		$(DOCTL) registry create $(DO_REGISTRY) --subscription-tier starter --region $(DO_REGION)
	$(DOCTL) kubernetes cluster get $(DO_CLUSTER) >/dev/null 2>&1 || \
		$(DOCTL) kubernetes cluster create $(DO_CLUSTER) --region $(DO_REGION) --size $(DO_SIZE) --count $(DO_NODES) --wait
	$(DOCTL) kubernetes cluster registry add $(DO_CLUSTER)
	$(DOCTL) kubernetes cluster kubeconfig save $(DO_CLUSTER)

do-deploy: k8s-secret ## Build and push the image, apply the manifests and print the public URL
	docker build -t $(DO_IMAGE) .
	$(DOCTL) registry login
	docker push $(DO_IMAGE)
	kubectl kustomize deploy/k8s | sed 's#image: todo-api:dev#image: $(DO_IMAGE)#' | $(KUBECTL_DO) apply -f -
	$(KUBECTL_DO) -n todo rollout status statefulset/postgres --timeout=180s
	$(KUBECTL_DO) -n todo rollout status deployment/todo-api --timeout=180s
	@$(MAKE) --no-print-directory do-url

do-url: ## Wait for the LoadBalancer IP and print the public URL
	@until ip=$$($(KUBECTL_DO) -n todo get service todo-api -o jsonpath='{.status.loadBalancer.ingress[0].ip}') && [ -n "$$ip" ]; do \
		echo "waiting for load balancer IP..."; sleep 10; done; echo "http://$$ip"

do-down: ## Delete the cluster with its load balancer and volumes, then list what is left
	$(DOCTL) kubernetes cluster delete $(DO_CLUSTER) --dangerous --force
	@echo "load balancers left:"; $(DOCTL) compute load-balancer list --format Name,IP --no-header
	@echo "volumes left:"; $(DOCTL) compute volume list --format Name,Size --no-header
