# List pagination

- **Date:** 2026-11-22
- **Informed:** [ADR-0011: Cursor pagination for GET /tasks](../../adr/0011-cursor-pagination.md)
- **Raw output:** [before](before-pagination.txt) · [after](after-pagination.txt)

## Question

The specification does not require pagination, and [ADR-0004](../../adr/0004-api-contract.md) had
left it out of scope. Two questions:

1. At what task count does an unpaginated `GET /tasks` become a problem, and which cost
   dominates: server time, response size or memory?
2. If pages are served, how should the in-memory repository find them?

## Environment

| | |
|---|---|
| CPU | AMD Ryzen 7 9700X, 16 threads |
| OS | Linux (WSL2) |
| Go | 1.27.4 |
| Mode | In-process (`httptest`), no network; gin release mode, access logs discarded |
| Runs | `-count=5`, averages below |

## Before: unpaginated `GET /tasks`

The repository copied every task under `RLock`, sorted by ID, and the handler encoded all of them.

| Tasks | `GET /tasks` | Response | Allocated / request | Repository `List` |
|---|---|---|---|---|
| 100 | 32 µs | 12 KB | 50 KB | 7.3 µs |
| 1,000 | 0.34 ms | 124 KB | 0.47 MB | 110 µs |
| 10,000 | 3.8 ms | 1.26 MB | 6.4 MB | 1.4 ms |
| 100,000 | 43 ms | 12.8 MB | 82 MB | 17 ms |

**Interpretation**

- Every cost is linear in the task count. The repository is ~40% of request time; JSON encoding
  and gin are the rest.
- Server time stays tolerable; size and memory do not. At 100,000 tasks the response takes
  ~10 s to download at 10 Mbps, and 10 concurrent requests allocate ~800 MB.
- Without authentication every client shares one list, so the task count is unbounded.

| Tasks in one list | Need for pagination |
|---|---|
| < 1,000 | None: < 130 KB, < 0.5 ms |
| 1,000 – 10,000 | Depends on clients; noticeable on mobile networks |
| > 10,000 | Needed: responses exceed 1 MB |

## Repository strategies for a page

| Strategy | 100,000 tasks, 20-item page | Allocated | Notes |
|---|---|---|---|
| Scan map, filter `id > after`, sort, truncate (first attempt) | 23.8 ms | 50 MB | Slower than listing everything: sorts all matches and regrows a slice sized for the page |
| B-tree keyed by ID | not measured | — | O(log n + limit); adds a dependency |
| **Walk IDs upward from `after`** (chosen) | **0.35 µs** | **1.8 KB** | IDs are monotonic and never reused (ADR-0002), so map lookups `after+1, after+2, …` return tasks already in order; no scan, no sort |

The first-attempt figure comes from a 3-run measurement during development; its raw output was
not kept.

## After: cursor pagination, default page size 20

| Tasks | `GET /tasks` | Response | Allocated / request |
|---|---|---|---|
| 100 | 8.1 µs | 2.4 KB | 16 KB |
| 1,000 | 8.4 µs | 2.4 KB | 16 KB |
| 10,000 | 8.0 µs | 2.4 KB | 16 KB |
| 100,000 | 7.9 µs | 2.4 KB | 16 KB |

Repository `List`:

| Tasks | First page (20) | Middle page (20) | Whole list |
|---|---|---|---|
| 100 | 0.47 µs | 0.48 µs | 2.3 µs |
| 1,000 | 0.51 µs | 0.51 µs | 24 µs |
| 10,000 | 0.55 µs | 0.56 µs | 0.28 ms |
| 100,000 | 0.35 µs | 0.36 µs | 3.6 ms |

Worst case (`BenchmarkListSparse`): 100,000 tasks with 99,980 deleted, first page →
**2.3 ms**, 1.8 KB. Every deleted ID before the page is one map lookup.

**Interpretation**

- Page cost no longer depends on the total number of tasks; neither does response size.
- Listing everything also got ~4.7× faster (17 ms → 3.6 ms at 100,000) because walking IDs
  needs no sort.
- The worst case is bounded by the highest ID ever assigned and is still faster than the old
  average case (17 ms).

## Decision

Adopt cursor pagination with a default page size of 20 and a maximum of 100, served by walking
IDs upward. Recorded in [ADR-0011](../../adr/0011-cursor-pagination.md).

## Revisit when

- Filtering (e.g. `?status=0`) is added: walking IDs would continue until `limit` matches are
  found, which degrades with selective filters; a secondary index would be the next step.
- IDs stop being monotonic (e.g. UUIDs): ID walking no longer applies; use an ordered structure.
- Deletion patterns leave long runs of deleted IDs and the sparse case shows up in latency.
