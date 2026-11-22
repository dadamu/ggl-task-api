# Benchmarks

Performance measurements used as evidence for design decisions. Each task has its own folder:
a `README.md` record stating the question, environment, results and the decision it informed,
plus the raw output (`go test`, k6, container samples, profiles) so the numbers can be re-checked
or compared later.

Numbers depend on hardware and Go version. Compare trends and ratios across records, not
absolute values from different machines.

## Current conclusions

One container, 2 CPUs, 512 MiB; workload 70% `GET` / 20% `POST` / 10% `PUT`; pass criteria
p99 < 50 ms and < 0.1% failures. Measured on a shared WSL2 host, so treat values as lower bounds.

| Question | Answer | Source |
|---|---|---|
| Maximum rate, small store | ~10,000 req/s (~5,000 per CPU); knee 10,000–15,000; collapse above ~20,000 | [Load test](2026-11-22-load-test/) |
| Bottleneck | CPU spent on HTTP plumbing: network syscalls ~28%, access logging ~16%, JSON ~13%; repository ~2%, no lock contention | [Load test](2026-11-22-load-test/) |
| Failure mode past the limit | Queueing, then connections closed by the 10 s `WriteTimeout`; throughput drops | [Load test](2026-11-22-load-test/) |
| Sustained load | 5,000 req/s for 10 min: 0 errors, p99 1.86 ms, no degradation over time | [Soak test](2026-11-22-soak-test/) |
| Long-run bound | Stored task count, not rate: ~205 B per task, RSS ≈ 2.6× live heap, ~950,000 tasks in 512 MiB (estimated) | [Soak test](2026-11-22-soak-test/) |
| Rate with a large store | Lower than 10,000 req/s: memory headroom and GC work grow with the store; not measured precisely | [Soak test](2026-11-22-soak-test/) |
| `GET /tasks` cost vs data size | Constant with pagination (~8 µs in-process at 100,000 tasks) | [List pagination](2026-11-22-list-pagination/) |

For the expected load of this service (well under 1,000 req/s) one 2-CPU instance has about 10×
headroom; for long-running deployments the in-memory store, not request rate, is the limit.

## Records

| Date | Record | Question | Informed |
|---|---|---|---|
| 2026-11-22 | [List pagination](2026-11-22-list-pagination/) | When does `GET /tasks` need pagination, and how should the repository serve a page? | [ADR-0011](../adr/0011-cursor-pagination.md) |
| 2026-11-22 | [Load test](2026-11-22-load-test/) | How much traffic can one 2-CPU instance handle, what saturates first, and how does it fail? | [ADR-0012](../adr/0012-load-testing-and-profiling.md) (method); capacity baseline |
| 2026-11-22 | [Soak test](2026-11-22-soak-test/) | Does it hold under sustained load, and what bounds a long run with in-memory storage? | Measurement only; summarizes system limits |

## Running

```bash
make bench                                   # all benchmarks, with -benchmem
go test -run='^$' -bench=BenchmarkList -benchmem -count=5 ./internal/infrastructures/persistence/memory/
```

Use `-count=5` or more when recording results, and compare runs with
[`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat).

| Benchmark | Package | Measures |
|---|---|---|
| `BenchmarkList` | `internal/infrastructures/persistence/memory` | Repository `List` for 100–100,000 tasks: whole list, first page, middle page |
| `BenchmarkListSparse` | same | Worst case for ID walking: 99.98% of IDs deleted |
| `BenchmarkListTasksHTTP` | `internal/transport/http/router` | `GET /tasks` in-process through router, handler, application, repository and JSON encoding; reports `resp_bytes` |

Benchmarks run in-process (no network) with access logs discarded.

## Load tests

`make loadtest` builds the image and runs [`loadtest/run.sh`](../../loadtest/run.sh): for each
rate it starts a fresh, resource-limited container (`--cpus=2 --memory=512m`, pprof enabled on a
separate port), runs the [k6 script](../../loadtest/tasks.js) at a constant arrival rate, samples
container CPU and memory, and optionally captures a CPU profile (`PROFILE_RATES`). Results go to
`loadtest/results/` (git-ignored); copy the ones worth keeping into a task folder here.

## Adding a record

1. Create a folder `YYYY-MM-DD-<topic>/` with the record in `README.md` and the raw output next
   to it (descriptive names, e.g. `before-<change>.txt`, `k6-rate-<n>.txt`, `cpu-<label>.pprof`).
2. Include: question, environment (CPU, threads, OS, Go version), command, results table,
   interpretation, decision, and links to the ADR or PR it informed.
3. Add a row to the table above.
