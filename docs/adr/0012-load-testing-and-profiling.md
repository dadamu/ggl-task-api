# ADR-0012: Load testing and profiling

- **Status:** Accepted
- **Date:** 2026-11-22
- **Supersedes:** the Configuration table of [ADR-0008](0008-runtime-and-deployment.md)
  (adds `PPROF_ENABLED` and `PPROF_ADDR`)

## Context

Benchmarks ([docs/benchmark](../benchmark/README.md)) measure single operations in-process. They
do not answer how much traffic one instance can serve, which resource saturates first, or how the
service behaves past that point. That needs a load test against the deployed artifact (the Docker
image) and a way to see where CPU time goes under load.

## Considered options

### Metrics during a load test

1. **Prometheus + Grafana.**
   - Pros: time series, dashboards; reusable for production monitoring.
   - Cons: extra services and instrumentation for a one-off experiment; does not show *which
     code* consumes CPU.
2. **Load generator report + `docker stats` + pprof.**
   - Pros: throughput, latency percentiles and error rate come from the load generator; CPU and
     memory from the container runtime; a CPU profile identifies the hot code. No new services.
   - Cons: no long-term history; not a monitoring solution.

### Load generator

1. **k6** — scripted mixed workloads, constant-arrival-rate executor, thresholds with early
   abort, percentile summaries; runs as a pinned Docker image.
2. **vegeta / oha / hey** — simple fixed-rate attacks; mixed read-modify-write flows (create then
   update with the returned `version`) are awkward to express.

### Exposing pprof

1. **On the API router** — reachable through the public port; leaks internals and allows anyone to
   trigger CPU-heavy profiling.
2. **Separate server on its own address, opt-in** — never reachable through the API port; off by
   default.
3. **Not at all** — load-test findings would be guesses.

## Decision

- Use **k6** (Docker image `grafana/k6:2.4.0`) with a **step load test**
  ([`loadtest/run.sh`](../../loadtest/run.sh), [`loadtest/tasks.js`](../../loadtest/tasks.js),
  `make loadtest`):
  - a fresh container per rate, limited to `--cpus=2 --memory=512m`, so results describe a
    defined instance size and steps do not influence each other;
  - mixed workload: 70% `GET /tasks`, 20% `POST /tasks`, 10% `PUT /tasks/{id}`;
  - constant arrival rate, 5 s warm-up excluded, 30 s measured;
  - **pass criteria:** p99 < 50 ms, failed requests < 0.1%, dropped iterations < 1% of planned;
    `409 VERSION_CONFLICT` counts as an expected outcome;
  - **early stop:** a step aborts once failures exceed 5% or p99 exceeds 1 s, and the run stops
    at the first failing step (`STOP_ON_FAIL=false` to sweep every rate);
  - container CPU and memory sampled about every second with elapsed time; container state
    (`OOMKilled`, restarts) recorded before removal; optional pprof CPU profile per rate
    (`PROFILE_RATES`) and heap profile at a given second (`HEAP_AT`), the latter used for soak
    tests with a long `DURATION`.
- No Prometheus for load testing.
- **pprof** on a separate `http.Server` with its own mux (`cmd/server/pprof.go`), started only
  when enabled. Configuration, extending ADR-0008:

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `GIN_MODE` | `debug` (`release` in the image) | gin mode |
| `SWAGGER_ENABLED` | `true` | Register `/swagger/*` |
| `PPROF_ENABLED` | `false` | Start the pprof server |
| `PPROF_ADDR` | `localhost:6060` | pprof listen address; set `:6060` in a container and publish the port only where needed |

  Malformed boolean values fail startup, as in ADR-0008. The pprof server has no
  `WriteTimeout` because profile and trace endpoints stream for the requested duration; it shuts
  down with the API server.
- Results worth keeping are recorded under `docs/benchmark/` with raw k6 output, container
  samples and profiles; `loadtest/results/` is git-ignored.

## Consequences

- First measurements ([load test](../benchmark/2026-11-22-load-test/),
  [soak test](../benchmark/2026-11-22-soak-test/)): about **10,000 req/s on 2 CPUs** with a small
  store; CPU is the limiting resource, spent on network syscalls (~28%), access logging (~16%) and
  JSON (~13%), not the repository lock. Long runs are bounded by the number of stored tasks
  (in-memory storage), which also lowers the sustainable rate as the store grows.
- Results include the load generator sharing the host and Docker's network hop on WSL2; they are
  a lower bound for dedicated hardware and are compared by trend, not absolute value.
- pprof is off by default; enabling it on a public address would expose internals. The default
  address binds to localhost only.
- The 1% dropped-iteration budget accepts brief load-generator stalls near capacity; a stricter
  zero-dropped criterion made results flap between runs.

## Compliance

- Config tests cover the new variables, defaults and malformed values.
- pprof routes are registered only on the separate server; the API router has no `/debug` routes.
- `bash -n loadtest/run.sh` and a short low-rate run validate the scripts after changes.

## Notes

- Improvements found by the first load test (buffered or sampled access logs, `GOMEMLIMIT`,
  admission control returning `503`) are listed in the load test record and not decided here.
