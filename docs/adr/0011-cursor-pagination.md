# ADR-0011: Cursor pagination for GET /tasks

- **Status:** Accepted
- **Date:** 2026-11-22
- **Supersedes:** the pagination row of [ADR-0004](0004-api-contract.md) "Out of scope" and its
  `GET /tasks` response; the `List` part of [ADR-0002](0002-in-memory-storage.md) Decision

## Context

`GET /tasks` returned every task. Benchmarks (`make bench`; AMD Ryzen 7 9700X, Go 1.27.4,
in-process, no network; full record in
[docs/benchmark/2026-11-22-list-pagination.md](../benchmark/2026-11-22-list-pagination.md))
measured the unpaginated endpoint:

| Tasks | Latency | Response size | Allocated per request |
|---|---|---|---|
| 1,000 | 0.33 ms | 124 KB | 0.47 MB |
| 10,000 | 3.8 ms | 1.26 MB | 6.4 MB |
| 100,000 | 43 ms | 12.8 MB | 81 MB |

Cost grows linearly. Server latency stays tolerable, but response size and allocations do not:
12.8 MB takes ~10 s to download at 10 Mbps, and 10 concurrent requests allocate ~800 MB. Without
authentication all clients share one list, so the task count is unbounded.

The repository spent ~40% of the time copying all tasks and sorting them by ID; JSON encoding
spent the rest.

## Considered options

### Pagination style

1. **Offset** (`?offset=40&limit=20`).
   - Pros: simple; allows jumping to a page.
   - Cons: deletions during paging skip tasks and insertions repeat them; the server still
     walks past `offset` items.
2. **Cursor / keyset** (`?after=<id>&limit=20`).
   - Pros: stable under concurrent inserts and deletes; IDs are monotonic and never reused
     (ADR-0002), so `id > after` is a natural cursor.
   - Cons: no random page access.

### Default when no parameters are given

1. **Return everything** — keeps the old behavior but leaves responses unbounded.
2. **Default page size** — bounds every response; a client unaware of pagination sees only the
   first page and must follow `next_cursor`.

### Repository implementation

1. **Scan the map, filter, sort, truncate** — O(n log n) per page regardless of page size.
2. **B-tree keyed by ID** — O(log n + limit); adds a dependency.
3. **Walk IDs upward from `after`** — IDs come from an increasing counter and are never reused,
   so looking up `after+1, after+2, …` in the map yields tasks already in ID order.
   O(limit + skipped deleted IDs), no sort, no dependency.

## Decision

- `GET /tasks?limit=<1..100>&after=<id>`; cursor (keyset) pagination.
- `limit` defaults to **20**, maximum **100**. `after` defaults to 0 (start).
- Invalid `limit` or `after` (not an integer, out of range) → `400 VALIDATION_FAILED` with all
  invalid parameters in `details`; values are not silently clamped.
- Response: `{"data": [...], "next_cursor": <id> | null}`. `next_cursor` is the ID of the last
  task in the page when more tasks exist, otherwise `null`. The application asks the repository
  for `limit + 1` tasks to decide.
- Repository: `List(ctx, ListQuery{After, Limit})`, implemented by walking IDs upward
  (option 3) under `RLock`.

## Consequences

Re-measured with the default page size of 20:

| Tasks | `GET /tasks` latency | Response size | Repository page | Repository, 99.98% deleted (worst case) |
|---|---|---|---|---|
| 1,000 | 8 µs | 2.4 KB | 0.5 µs | — |
| 100,000 | 8 µs | 2.4 KB | 0.4 µs | 2.1 ms |

- Latency and response size no longer depend on the number of tasks.
- The worst case is many deleted IDs before the cursor; it is bounded by the highest ID ever
  assigned and still faster than the previous full scan and sort (17 ms at 100,000 tasks).
- Clients must follow `next_cursor` to see more than one page; a client that ignores it sees
  only the first 20 tasks.
- No B-tree is needed for ID-ordered pagination; it would only matter for other sort orders.

## Compliance

- Handler tests: default page size, `limit` and `after`, last page with `next_cursor: null`,
  following `next_cursor` visits every task exactly once, invalid parameters.
- Application tests: `next_cursor` on exact-fit pages, cursor stability after deletions.
- Repository tests: `after` on deleted IDs and past the last ID.
- Benchmarks: `BenchmarkList`, `BenchmarkListSparse`, `BenchmarkListTasksHTTP`.

## Notes

- Filtering (e.g. `?status=0`) is still out of scope. Combining it with ID walking would scan
  until `limit` matches are found; a secondary index would be the next step.
