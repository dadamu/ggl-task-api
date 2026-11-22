# Benchmarks

Performance measurements used as evidence for design decisions. Each record states the question,
the environment, the results and the decision it informed, and keeps the raw `go test` output
next to it so the numbers can be re-checked or compared later.

Numbers depend on hardware and Go version. Compare trends and ratios across records, not
absolute values from different machines.

## Records

| Date | Record | Question | Informed |
|---|---|---|---|
| 2026-11-22 | [List pagination](2026-11-22-list-pagination.md) | When does `GET /tasks` need pagination, and how should the repository serve a page? | [ADR-0011](../adr/0011-cursor-pagination.md) |

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

## Adding a record

1. Name it `YYYY-MM-DD-<topic>.md`; save raw output as `YYYY-MM-DD-<topic>-<variant>.txt`.
2. Include: question, environment (CPU, threads, OS, Go version), command, results table,
   interpretation, decision, and links to the ADR or PR it informed.
3. Add a row to the table above.
