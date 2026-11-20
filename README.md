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
| Dockerfile (multi-stage) + docker compose | 🚧 |
| Kubernetes manifests | 🚧 |
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
and for the race detector. Docker with Compose v2 runs the local PostgreSQL.

```bash
make run            # starts the compose PostgreSQL, then the API against it
curl -i localhost:8080/livez
curl -i localhost:8080/readyz
```

### Local PostgreSQL

```bash
make db-up     # start PostgreSQL 17 and wait until it accepts connections
make db-psql   # psql shell inside the container (no local psql needed)
make db-down   # stop it; data is kept in a named volume
make db-reset  # stop it and wipe the data
```

It listens on `127.0.0.1:5432` only, with user, password and database `todo` (override with
`POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_PORT`). The API service joins `compose.yaml`
once the Dockerfile lands.

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
make test              # go test ./... (with -race when cgo is available); PostgreSQL tests skip
make test-integration  # same, plus PostgreSQL tests against the compose database (`make db-up`)
make ci     # fmt-check → vet → golangci-lint → govulncheck → gosec → test
```

The same `make ci` runs:

- **before every commit**, through a versioned git hook. Enable it once per clone with `make hooks`.
- **on every push**, in GitHub Actions (`.github/workflows/ci.yml`), with a PostgreSQL service so the
  PostgreSQL tests run there too.
Tool versions are pinned in the `Makefile` and run via `go run`, so nothing needs to be installed globally.

---

## Docker 🚧

Multi-stage build from the official Go image, exposing port 8080; `docker compose up` will start the
API together with PostgreSQL.

## Kubernetes deployment 🚧

Manifests for: Deployment (2 replicas, CPU/memory requests and limits, liveness/readiness probes),
Service (`LoadBalancer`), ConfigMap, Secret, and PostgreSQL as a StatefulSet with a PersistentVolumeClaim.
Tested locally on kind/k3d and demoed on DigitalOcean Kubernetes (DOKS).

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
- **In-memory**, used only to keep HTTP handler unit tests fast and dependency-free.

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
internal/todo/todotest/  Store contract suite every storage backend must pass
internal/store/memstore/ in-memory Store (tests only)
internal/store/pgstore/  PostgreSQL Store, embedded schema migration, readiness gate
internal/retry/    exponential backoff used to wait for PostgreSQL on startup
internal/httpapi/  HTTP only: routing, JSON decoding, Problem Details, health probes
internal/server/   HTTP server lifecycle and graceful shutdown
```

The domain package knows nothing about HTTP or SQL. The HTTP layer depends on a small `TodoService`
interface. The wire format (`todoResponse`) is kept separate from the domain type, so the JSON contract
can't change by accident.

`Store.Update` takes a *change function* rather than a full item. This makes read-modify-write atomic:
two concurrent PATCH requests can't lose each other's changes. The in-memory store holds a lock, and the
PostgreSQL store uses a `SELECT … FOR UPDATE` transaction. A shared contract suite (`todotest`)
runs against every store implementation, which proves this property, including under concurrency.

A panic in a handler is recovered into a logged 500, so one bad request can't take down other requests.

Dependencies are injected through constructors: the env lookup, the readiness dependency and the
logger are all parameters. Tests use named fakes instead of globals. Logs are structured JSON
(`log/slog`), ready for any log collector.

### 7. Workflow

Developed in short test-first increments. Every commit passes the full CI gate, so `main` is always
deployable.
