# GGL Task API

A RESTful task API in Go ([gin](https://github.com/gin-gonic/gin)) with in-memory storage, built with
a layered DDD architecture. Every significant design decision is recorded as an
[ADR](docs/adr/README.md), and capacity was measured with benchmarks and load tests
([docs/benchmark](docs/benchmark/README.md)).

## Quick start

With Docker only:

```bash
docker build -t ggl-task-api .
docker run --rm -p 8080:8080 ggl-task-api
```

Or with Go 1.27.4+ and Make:

```bash
make run              # http://localhost:8080
```

Then:

- Swagger UI: <http://localhost:8080/swagger/index.html>
- Health check: <http://localhost:8080/healthz>

```bash
curl -X POST localhost:8080/tasks -H 'Content-Type: application/json' -d '{"name":"Buy milk"}'
curl localhost:8080/tasks
```

### Notes for reviewers

Three behaviors that differ from a minimal CRUD API, each intentional:

- **`POST` and `PUT` require `Content-Type: application/json`.** `curl -d` without
  `-H 'Content-Type: application/json'` sends a form body and gets `415`.
- **`PUT` requires `version`** (optimistic locking): send the `version` from the last read, e.g.
  `{"name":"Buy oat milk","status":1,"version":1}`. Without it you get `400`; with a stale one,
  `409` plus the current task.
- **`GET /tasks` returns 20 tasks by default.** Follow `next_cursor` (`?after=<next_cursor>`) or
  raise `limit` (max 100) to see more.

## API

| Method | Path | Success | Description |
|---|---|---|---|
| `GET` | `/tasks?limit=&after=` | `200` | List tasks, one page at a time |
| `POST` | `/tasks` | `201` + `Location` | Create a task |
| `PUT` | `/tasks/{id}` | `200` | Replace a task's name and status (optimistic locking) |
| `DELETE` | `/tasks/{id}` | `204` | Delete a task |

A task:

```json
{
  "id": 1,
  "name": "Buy milk",
  "status": 0,
  "version": 1,
  "created_at": "2026-11-22T08:19:48.475Z",
  "updated_at": "2026-11-22T08:19:48.475Z"
}
```

| Field | Type | Notes |
|---|---|---|
| `id` | integer | Assigned by the server, increasing, never reused |
| `name` | string | Required; not blank; at most 255 characters |
| `status` | integer | `0` incomplete, `1` completed; defaults to `0` on create |
| `version` | integer | Starts at 1, incremented on every update |
| `created_at`, `updated_at` | string | UTC, RFC 3339, millisecond precision |

### Create

```bash
curl -i -X POST localhost:8080/tasks -H 'Content-Type: application/json' \
  -d '{"name":"Buy milk"}'
```

```
HTTP/1.1 201 Created
Location: /tasks/1

{"id":1,"name":"Buy milk","status":0,"version":1,"created_at":"...","updated_at":"..."}
```

`status` is optional (default `0`). Unknown fields, and server-managed fields such as `id`, are ignored.

### List (cursor pagination)

```bash
curl 'localhost:8080/tasks?limit=1'
```

```json
{"data":[{"id":1,"name":"Buy milk",...}],"next_cursor":1}
```

```bash
curl 'localhost:8080/tasks?limit=1&after=1'
```

```json
{"data":[{"id":2,"name":"Pay rent",...}],"next_cursor":null}
```

- Tasks are ordered by `id` ascending.
- `limit`: 1–100, **default 20**. `after`: return tasks with `id` greater than this cursor.
- **Without parameters only the first 20 tasks are returned.** Pass `next_cursor` as `after` to get
  the next page; `next_cursor` is `null` on the last page.

### Update (optimistic locking)

`PUT` replaces `name` and `status`; all three fields are required. `version` is the version you last
read: the update succeeds only if it is still current.

```bash
curl -X PUT localhost:8080/tasks/1 -H 'Content-Type: application/json' \
  -d '{"name":"Buy oat milk","status":1,"version":1}'
```

```json
{"id":1,"name":"Buy oat milk","status":1,"version":2,...}
```

If someone else updated the task first, you get `409` with the current task, so you can merge and
retry with `current.version`:

```json
{
  "error": {
    "code": "VERSION_CONFLICT",
    "message": "task has been modified; reload and retry with the current version",
    "current": {"id":1,"name":"Buy oat milk","status":1,"version":2,...}
  }
}
```

### Delete

```bash
curl -i -X DELETE localhost:8080/tasks/1     # 204 No Content; 404 if it does not exist
```

### Errors

All errors share one format; `code` is stable for programs, `message` is for humans:

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "invalid request body",
    "details": [
      {"field": "name", "reason": "must not be blank"},
      {"field": "status", "reason": "must be 0 or 1"}
    ]
  }
}
```

| Status | `code` | When |
|---|---|---|
| 400 | `INVALID_JSON` | Malformed or empty JSON body |
| 400 | `VALIDATION_FAILED` | Missing or invalid fields or query parameters (`details` lists them) |
| 400 | `INVALID_ID` | Path `id` is not a positive integer |
| 404 | `TASK_NOT_FOUND` | No task with that `id` |
| 404 | `ROUTE_NOT_FOUND` | Unknown path |
| 405 | `METHOD_NOT_ALLOWED` | Method not supported on the path (see `Allow`) |
| 409 | `VERSION_CONFLICT` | Stale `version` on `PUT` (includes `current`) |
| 413 | `PAYLOAD_TOO_LARGE` | Body over 1 MB |
| 415 | `UNSUPPORTED_MEDIA_TYPE` | `POST`/`PUT` without `Content-Type: application/json` |
| 500 | `INTERNAL_ERROR` | Unexpected error; details are logged, never returned |

## Configuration

Environment variables; malformed values make the server exit at startup.

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `GIN_MODE` | `debug` (`release` in the Docker image) | gin mode |
| `SWAGGER_ENABLED` | `true` | Serve `/swagger/*`; set `false` in production |
| `PPROF_ENABLED` | `false` | Start a pprof server on a separate address |
| `PPROF_ADDR` | `localhost:6060` | pprof listen address |

## Development

Requires Go 1.27.4+, Make and (for images and load tests) Docker. Tool binaries (`golangci-lint`,
`swag`) are installed into `bin/tools/` with pinned versions on first use.

```bash
make help        # list all targets
make check       # swag, tidy, fmt, vet, lint, test -race (what CI runs)
make coverage    # coverage report for internal/ (98.8%)
make bench       # Go benchmarks
make loadtest    # k6 step load test against a 2-CPU / 512 MiB container
make docker-run  # build and run the image on :8080
```

CI runs `make check`, verifies that generated files are committed, and builds and smoke-tests the
Docker image.

### Project layout

```
cmd/server/                      entrypoint and wiring (composition root)
internal/
├── domain/task/                 entity, invariants, repository port, domain errors
├── application/task/            use cases
├── infrastructures/             in-memory repository, config
└── transport/http/              router, handlers, DTOs, error mapping, middleware
swagger/                         generated by swag, do not edit
loadtest/                        k6 load test scripts
docs/adr/                        architecture decision records
docs/benchmark/                  benchmark and load test records
```

Dependency direction `transport → application → domain ← infrastructures` is enforced by
`depguard` in the linter.

## Design decisions

Every significant decision — architecture, storage and concurrency, API contract, validation,
pagination, runtime, testing and performance — is recorded with its alternatives and trade-offs in
the [Architecture Decision Records](docs/adr/README.md).

## Scope and trade-offs

**Beyond the specification, and why**

- Optimistic locking — concurrent `PUT`s would otherwise silently overwrite each other.
- Cursor pagination — measured: an unpaginated list reaches 12.8 MB at 100,000 tasks.
- Uniform error format, request size limit, server timeouts — predictable failures and basic hardening.
- Benchmarks and load tests — to state capacity with evidence rather than guesses.

**Intentionally not done**

- Persistence — in-memory storage as allowed by the specification; data is lost on restart and
  bounded by RAM. The repository is an interface, so a database is a new implementation.
- Authentication, rate limiting, CORS — out of scope; all clients share one task list.
- Idempotency-Key — low value here ([ADR-0010](docs/adr/0010-defer-idempotency-key.md)).
- Filtering and sorting options, metrics and tracing — not required; noted in the ADRs.

## Performance

One container with 2 CPUs and 512 MiB, mixed workload (70% reads), p99 < 50 ms
([details](docs/benchmark/README.md)):

- About **10,000 req/s** with a small data set; CPU goes to HTTP plumbing (network, access log,
  JSON), not the repository or its lock.
- 5,000 req/s sustained for 10 minutes with no errors and p99 1.86 ms.
- Long runs are bounded by the number of stored tasks (~950,000 in 512 MiB), since in-memory data
  only grows.
