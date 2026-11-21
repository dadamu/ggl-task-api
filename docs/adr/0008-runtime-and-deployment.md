# ADR-0008: Runtime and deployment

- **Status:** Accepted
- **Date:** 2026-11-21

## Context

The specification requires a Go 1.18+ runtime and a Dockerfile to run the API. The server must
also be robust against slow or abusive clients and shut down cleanly.

## Considered options

### Go version

1. **Declare `go 1.18`** (literal minimum from the specification).
   - Cons: forces gin ≤ v1.9.0 (has CVE-2023-29401, fixed in v1.9.1 which needs Go 1.20) and old
     transitive dependencies; loses Go 1.22 per-iteration loop variables, `slices`, `min`/`max`,
     `log/slog`.
2. **Declare the dependency minimum (`go 1.25.0`)**, required by gin v1.12.0.
   - Pros: widest compatibility for local builds.
   - Cons: permits builds with toolchains missing recent standard library security fixes.
     Concretely, Go 1.25 is out of support and does not receive the CVE-2026-111943 fix (below).
3. **Declare the latest patched release (`go 1.27.4`).**
   - Pros: every build uses a toolchain with current standard library security fixes; latest
     dependencies.
   - Cons: local builds need Go ≥ 1.27.4 (Go 1.21+ downloads it automatically).

**CVE-2026-111943** (Go issue [#82345](https://github.com/golang/go/issues/82345)): private keys of
eMudhra intermediate CAs were compromised. Go 1.27.3 and 1.26.10 add a hardcoded deny list to
`crypto/x509` that rejects chains through those keys. The deny list is **compiled into the
binary**: updating the OS or image CA bundle does not apply it; only rebuilding with a fixed
toolchain does.

### Base image

1. **`alpine`** — small, has a shell; musl libc and a package manager enlarge the attack surface.
2. **`gcr.io/distroless/static-debian12:nonroot`** — no shell or package manager, runs as non-root;
   requires a static binary (`CGO_ENABLED=0`).
3. **`scratch`** — smallest; no CA certificates, timezone data or non-root user out of the box.

## Decision

### Go version

- `go.mod` declares **`go 1.27.4`** (option 3). "Go 1.18+" is read as a lower bound, which this satisfies.
- 1.27.4 ≥ 1.27.3, so every build includes the CVE-2026-111943 deny list.
- The Docker build stage pins the same version, so the shipped binary is always built with a
  patched toolchain; reviewers need only Docker.
- The version is bumped, and images rebuilt, when Go releases security patches.

### Dockerfile

- Multi-stage build:
  1. `golang:1.27.4` builder: `go mod download` in a cached layer, then
     `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"`.
  2. `gcr.io/distroless/static-debian12:nonroot` runtime with only the binary.
- Swagger files are committed, so the image build does not run `swag`.
- Runs as non-root; `EXPOSE 8080`; `GIN_MODE=release` by default.
- No `HEALTHCHECK` (distroless has no shell or curl); orchestrators probe `GET /healthz`.

### HTTP server

| Setting | Value | Purpose |
|---|---|---|
| `ReadHeaderTimeout` | 5s | Mitigate Slowloris |
| `ReadTimeout` | 10s | Bound time to read headers + body |
| `WriteTimeout` | 10s | Bound handler + response time |
| `IdleTimeout` | 60s | Reclaim idle keep-alive connections |
| Shutdown grace period | 10s | Drain in-flight requests on `SIGINT`/`SIGTERM` |

### Configuration

Environment variables only:

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `GIN_MODE` | `debug` (`release` in the image) | gin mode |
| `SWAGGER_ENABLED` | `true` | Register `/swagger/*` routes |

### Swagger UI

- Served at `/swagger/index.html` when `SWAGGER_ENABLED=true`.
- Enabled by default, including in the image, so the API can be explored right after
  `docker run`.
- Production deployments should set `SWAGGER_ENABLED=false` to avoid exposing the API surface
  publicly. When disabled, the routes are not registered (`404 ROUTE_NOT_FOUND`).

### Health check

- `GET /healthz` returns `200 {"status":"ok"}` while the process can serve HTTP.
- It is a **liveness** signal only: the service has no external dependencies, so there is
  nothing to check for readiness. If dependencies are added (e.g. a database), a separate
  `/readyz` should check them.

## Consequences

- One command to run: `docker build -t ggl-task-api . && docker run -p 8080:8080 ggl-task-api`.
- Small image with no shell, reducing attack surface; debugging inside the container is harder.
- Local builds require Go ≥ 1.27.4.
- Slow clients cannot hold connections indefinitely; long-running handlers are cut at 10s.

## Compliance

- CI builds with the version from `go.mod`.
- CI builds the Docker image.
- Smoke test: run the image and `GET /healthz` returns `200`.

## Notes

- Dependabot (or Renovate) for Go modules, the Go toolchain, GitHub Actions and base images is a follow-up.
