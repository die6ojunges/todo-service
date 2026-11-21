# ToDo Microservice — Go · Docker · Kubernetes

A small RESTful ToDo service written in Go, packaged with Docker and deployed to Kubernetes
with health probes, externalized configuration and graceful shutdown.

> **Status: work in progress.** Built in small, always-green increments.
> Sections marked 🚧 describe what is planned and will be filled in as each slice lands.

| Area | Status |
|---|---|
| Configuration from environment (ConfigMap-ready) | ✅ |
| Liveness / readiness probes (`/livez`, `/readyz`) | ✅ |
| Graceful shutdown (SIGTERM → drain → shutdown) | ✅ |
| CI gates (fmt, vet, lint, vuln, sec, tests) | ✅ |
| ToDo REST API (CRUD + PATCH) | ✅ |
| PostgreSQL storage | ✅ |
| Dockerfile (multi-stage) + docker compose | ✅ |
| Kubernetes manifests (tested on kind) | ✅ |
| Cloud demo (DigitalOcean) | 🚧 |

---

## API specification

### Health

| Method | Path | 200 | 503 |
|---|---|---|---|
| GET | `/livez` | process is alive | — (never checks dependencies) |
| GET | `/readyz` | ready to take traffic | dependency down, or shutting down |

### ToDo

Resource:

```json
{ "id": "3f1c9a52-6a0e-4b8e-9a51-0d2b6f1e7c11", "title": "Write README", "completed": false }
```

| Method | Path | Success | Errors |
|---|---|---|---|
| GET | `/todos` | 200 + array (`[]` when empty) | — |
| GET | `/todos/{id}` *(extra)* | 200 | 400 invalid id, 404 |
| POST | `/todos` | 201 + `Location: /todos/{id}` + body | 400 / 422 |
| PUT | `/todos/{id}` | 200 + body (full replacement) | 400 / 422, 404 |
| PATCH | `/todos/{id}` *(extra)* | 200 + body (partial update) | 400 / 422, 404 |
| DELETE | `/todos/{id}` | 204, no body | 404 |

- **Request bodies** must be a single JSON object, up to 1 MiB.
  - POST and PUT require `Content-Type: application/json`.
  - PATCH accepts `application/merge-patch+json` or `application/json`.
  - Otherwise the API returns 415, or 413 for an oversized body.
- **Writable fields** are `title` and `completed`.
  - An unknown field returns 400. This includes `id`, which the server assigns.
  - Malformed JSON or a wrong type also returns 400.
- **Validation** returns 422.
  - `title` is trimmed and must be 1–200 characters.
  - PUT requires both fields.
  - `null` is rejected: in JSON Merge Patch it means "remove", and every field is required.
- **Errors** use [RFC 9457 Problem Details](https://www.rfc-editor.org/rfc/rfc9457) (`application/problem+json`):

```json
{"type":"about:blank","title":"Unprocessable Entity","status":422,
 "detail":"invalid completed: is required for a full replacement (PUT)","instance":"/todos/0c38f020-…"}
```

### Examples

```bash
B=localhost:8080
curl -i -X POST $B/todos -H 'Content-Type: application/json' -d '{"title":"Record demo"}'
# HTTP/1.1 201 Created
# Location: /todos/0c38f020-cb85-47dc-8373-8ae30b8b7750
curl $B/todos
curl $B/todos/<id>
curl -X PUT   $B/todos/<id> -H 'Content-Type: application/json' -d '{"title":"Record demo","completed":true}'
curl -X PATCH $B/todos/<id> -H 'Content-Type: application/merge-patch+json' -d '{"completed":false}'
curl -i -X DELETE $B/todos/<id>   # 204 No Content
```

---

## Running locally

Requirements: Go 1.25+. `make` and a C compiler (`build-essential`) are needed for `make` targets
and for the race detector. Docker (with Compose v2) runs the local PostgreSQL and the PostgreSQL tests.

```bash
make up             # whole stack in Docker: builds the image, starts API + PostgreSQL
make run            # or: API from source (go run) against the compose PostgreSQL
curl -i localhost:8080/livez
curl -i localhost:8080/readyz
make down           # stop everything; data is kept (make reset also wipes it)
```

### Local PostgreSQL

```bash
make db-up     # start only PostgreSQL 17 and wait until it accepts connections
make db-psql   # psql shell inside the container (no local psql needed)
```

It listens on `127.0.0.1:5432` only, with user, password and database `todo` (override with
`POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_PORT`). Data lives in a named volume.

### Configuration

All settings come from environment variables (in Kubernetes: a ConfigMap, plus a Secret for credentials).

| Variable | Default | Meaning |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `SHUTDOWN_TIMEOUT` | `15s` | Max time for in-flight requests to finish after SIGTERM |
| `READINESS_DRAIN_DELAY` | `5s` | Time `/readyz` reports 503 before the server stops |
| `DATABASE_URL` | — (required) | PostgreSQL URL, e.g. `postgres://user:password@host:5432/db` (in Kubernetes: a Secret) |

Invalid values fail fast at startup with a message naming the variable, the value and the expected shape
(all invalid variables are reported at once). `DATABASE_URL` is never echoed, since it holds the password.

### Quality gates

```bash
make test   # go test ./... (with -race when cgo is available)
make ci     # fmt-check → vet → golangci-lint → govulncheck → gosec → test
```

The same `make ci` runs:

- **before every commit**, through a versioned git hook. Enable it once per clone with `make hooks`.
- **on every push**, in GitHub Actions (`.github/workflows/ci.yml`).

The PostgreSQL store is tested against a real, disposable PostgreSQL started by
[Testcontainers](https://golang.testcontainers.org/) (same image as `compose.yaml`), so `make test`
needs Docker running but no setup. It fails rather than skips without Docker, so a green run always
covers the database.
Tool versions are pinned in the `Makefile` and run via `go run`, so nothing needs to be installed globally.

---

## Docker

The `Dockerfile` is a multi-stage build:

1. **Build** on the official `golang:1.25-alpine` image: a static binary (`CGO_ENABLED=0`, `-trimpath`,
   stripped), with BuildKit cache mounts so rebuilds are fast.
2. **Run** on `gcr.io/distroless/static-debian12:nonroot`: only the binary, no shell or package manager,
   running as a non-root user. The image is about 20 MB and exposes port 8080.

```bash
make docker-build   # docker build -t todo-api:dev .
docker compose up --build   # API + PostgreSQL (what `make up` runs, detached)
```

To run the image on its own, point it at a reachable PostgreSQL. With the compose database running
(`make db-up`), join its network:

```bash
docker run --rm -p 127.0.0.1:8080:8080 --network todo-service_default \
  -e DATABASE_URL='postgres://todo:todo@postgres:5432/todo?sslmode=disable' todo-api:dev
```

The compose file gives the API a 25s `stop_grace_period`, because Docker's default of 10s would kill it
during the graceful shutdown (drain plus in-flight requests).

## Kubernetes deployment

```mermaid
flowchart LR
  client([Client / curl]) -->|HTTP :80| lb[Service todo-api<br/>LoadBalancer]
  kubelet([kubelet]) -.->|/livez · /readyz| api1 & api2
  subgraph ns[namespace: todo]
    lb --> api1[Pod todo-api]
    lb --> api2[Pod todo-api]
    cm[(ConfigMap<br/>todo-api-config)] -.->|env| api1 & api2
    sec[(Secret<br/>postgres-credentials)] -.->|env| api1 & api2 & pg
    api1 & api2 -->|SQL :5432| svc[Service postgres<br/>headless]
    svc --> pg[StatefulSet<br/>postgres-0]
    pg --- pvc[(PVC 1 Gi)]
  end
```

Manifests live in `deploy/k8s/` and are applied together with kustomize (built into `kubectl`):

| File | What |
|---|---|
| `api-deployment.yaml` | API Deployment: 2 replicas, CPU/memory requests and limits, liveness (`/livez`) and readiness (`/readyz`) probes, 30s termination grace period, non-root, read-only root filesystem, no capabilities |
| `api-service.yaml` | `LoadBalancer` Service on port 80 (public IP in the cloud) |
| `configmap.yaml` | `PORT`, `SHUTDOWN_TIMEOUT`, `READINESS_DRAIN_DELAY` |
| `postgres-statefulset.yaml` | PostgreSQL 17 with a 1 Gi PersistentVolumeClaim |
| `postgres-service.yaml` | Headless Service giving the database the stable name `postgres` |
| `kustomization.yaml` | Namespace `todo`, plus the `postgres-credentials` Secret generated from `postgres.env` |

The database password is never committed. `postgres.env` is gitignored, and `make k8s-secret` creates it
from `postgres.env.example` with a random password. The API builds `DATABASE_URL` from the same Secret,
so the password exists in one place only.

### Local cluster (kind)

[kind](https://kind.sigs.k8s.io/) runs through `go run` at a pinned version, so only Docker and `kubectl`
are needed:

```bash
make kind-deploy        # create cluster, build + load image, create secret, apply, wait for rollout
make kind-port-forward  # LoadBalancer stays <pending> on kind; this maps the Service to localhost:8080
curl -i localhost:8080/todos
make kind-down          # delete the cluster
```

Every `kubectl` call in the Makefile pins the `kind-todo` context, so it can never deploy to another cluster.

Behavior verified on kind:

- Both replicas serve the same data (written through one pod, read through the other).
- Data survives a restart of the PostgreSQL pod (PVC).
- With PostgreSQL down, `/livez` stays 200 and `/readyz` returns 503. Both pods leave the Service
  endpoints **without restarting**, and come back on their own when the database returns.
- Deleting an API pod drains it for 5s and shuts it down cleanly, well inside the grace period.

---

## Design decisions and rationale

### 1. PostgreSQL instead of BoltDB / in-memory storage

The challenge asks for **2 replicas** and suggests in-memory storage or an embedded store such as BoltDB.
Neither works correctly with more than one replica:

- **BoltDB** holds an exclusive file lock, so only one process can open the database.
  A `ReadWriteOnce` PersistentVolumeClaim cannot be mounted by pods on different nodes either.
- **In-memory** storage gives each replica its own data, so clients behind the Service see
  inconsistent results depending on which pod answers.

So the API is **stateless** and state lives in a shared **PostgreSQL**. Replicas can scale horizontally
and survive restarts without losing data. The challenge words storage as an option ("may", "e.g."),
and this choice keeps its intent, which is persistence.

The store sits behind a `Store` interface with two implementations:

- **PostgreSQL**, used in every environment.
- **In-memory fake** (`todotest.FakeStore`), used only in tests of the service and HTTP layers, to keep
  them fast and dependency-free. It passes the same contract suite as PostgreSQL, so it can be trusted.

For the time-boxed demo, PostgreSQL runs **in the cluster** as a StatefulSet with a PVC, which covers
the optional PVC item of the challenge. Its credentials are in a Secret. In production a managed
database (e.g. DigitalOcean Managed PostgreSQL, Cloud SQL or RDS) would be the better choice. That is
a deliberate trade-off for cost and simplicity.

### 2. UUIDs as identifiers

IDs are generated by the server. UUIDs don't leak item counts or creation order and need no
central sequence.

### 3. REST semantics

- **`PUT` is a full replacement** (RFC 9110): `title` and `completed` are both required.
  It is not an upsert, since the server owns ID generation, so an unknown ID returns 404.
- **`PATCH` is a partial update** using JSON Merge Patch (RFC 7396). Only the fields sent change.
- **`POST`** returns `201 Created` with a `Location` header. **`DELETE`** returns `204 No Content`.
- `GET /todos/{id}` and `PATCH` are not required by the challenge. They are included because a
  resource collection without them is not idiomatic REST.

### 4. Health probes are implemented by the application

Kubernetes does not provide health endpoints. The kubelet only calls an HTTP endpoint the application
exposes and judges it by the status code. Two distinct questions, two endpoints (naming follows the
kube-apiserver's own `/livez` and `/readyz`):

- **Liveness (`/livez`): "is the process alive?"** It deliberately does **not** check the database.
  If it did, a database outage would make Kubernetes restart every replica in a loop without fixing
  anything.
- **Readiness (`/readyz`): "can this pod take traffic?"** It pings the database with a short timeout
  and returns 503 during shutdown, so the pod leaves the Service endpoints before it stops.

PostgreSQL may come up after the service. So at startup the service begins serving at once and keeps
retrying the connection and schema migration with exponential backoff (0.5s up to 10s). Until both
succeed, `/readyz` returns 503 while `/livez` stays 200, so Kubernetes waits instead of restarting the pod.

### 5. Graceful shutdown

On `SIGTERM` (what Kubernetes sends on pod termination):

1. `/readyz` starts returning 503.
2. The server keeps serving for `READINESS_DRAIN_DELAY`, while Kubernetes removes the pod from
   the Service endpoints.
3. `http.Server.Shutdown` lets in-flight requests finish, bounded by `SHUTDOWN_TIMEOUT`.
4. Resources such as the database pool are closed.

`terminationGracePeriodSeconds` in the manifest is set above drain + shutdown time. The HTTP server
also sets read, write and header timeouts, which protects against slow-client (Slowloris) attacks.

### 6. Project layout

```
cmd/todo-api/      entrypoint — only wires dependencies
internal/config/   environment → typed Config, validated at startup
internal/todo/     domain: model, validation rules, Store interface, Service
internal/todo/todotest/  Store contract suite, and the in-memory FakeStore used by tests
internal/store/pgstore/  PostgreSQL Store, embedded schema migration, readiness gate
internal/retry/    exponential backoff used to wait for PostgreSQL on startup
internal/httpapi/  HTTP only: routing, JSON decoding, Problem Details, health probes
internal/server/   HTTP server lifecycle and graceful shutdown
```

The domain package knows nothing about HTTP or SQL. The HTTP layer depends on a small `TodoService`
interface. The wire format (`todoResponse`) is kept separate from the domain type, so the JSON contract
can't change by accident.

`Store.Update` takes a *change function* rather than a full item. This makes read-modify-write atomic:
two concurrent PATCH requests can't lose each other's changes. The in-memory fake holds a lock, and the
PostgreSQL store uses a `SELECT … FOR UPDATE` transaction. A shared contract suite (`todotest`)
runs against every store implementation, which proves this property, including under concurrency.

A panic in a handler is recovered into a logged 500, so one bad request can't take down other requests.

Dependencies are injected through constructors: the env lookup, the readiness dependency and the
logger are all parameters. Tests use named fakes instead of globals. Logs are structured JSON
(`log/slog`), ready for any log collector.

### 7. Workflow

Developed in short test-first increments. Every commit passes the full CI gate, so `main` is always
deployable.
