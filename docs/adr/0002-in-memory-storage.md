# ADR-0002: In-memory storage with map and RWMutex

- **Status:** Accepted; `List` implementation superseded by [ADR-0011](0011-cursor-pagination.md)
- **Date:** 2026-11-21

## Context

The specification allows any in-memory storage. `net/http` serves each request on its own
goroutine, so storage is accessed concurrently. Requirements:

- Concurrent creates must never produce duplicate IDs.
- Check-and-write operations (e.g. "exists and version matches, then update") must be atomic.
- Listing must return a consistent, deterministically ordered snapshot.
- No data races (verified with `-race`).

### Operation shapes

| Operation | Shape |
|---|---|
| `POST /tasks` | Insert new key + generate ID |
| `PUT /tasks/{id}` | Read–compare-version–write on one key (compound, must be atomic) |
| `DELETE /tasks/{id}` | Remove one key |
| `GET /tasks` | **Full scan** + sort |

There is **no single-key read** (`GET /tasks/{id}` is not in the specification); the only read
is a full scan. The specification does not state a read/write ratio, and no assumption is made.

## Considered options

### ID type

1. **Auto-increment `int64`**
   - Pros: simple, human-friendly, naturally sortable by creation order.
   - Cons: enumerable; requires central coordination (not an issue for a single process).
2. **UUID (v4/v7)**
   - Pros: no coordination, not enumerable.
   - Cons: unnecessary for a single-process store with low create volume.

### Concurrent map

1. **`sync.Map`**
   - Its advantage is lock-free **single-key `Load`** for (a) write-once/read-many keys or
     (b) disjoint key sets per goroutine. Neither applies: there are no single-key reads, and
     `PUT` rewrites existing keys.
   - Correct compound updates *are* possible (store immutable `*Task`, `CompareAndSwap` on the
     pointer, retry loop; `atomic.Int64` for IDs), but require an immutability discipline and
     retry logic that reviewers must reason about.
   - `Range` does not provide a consistent snapshot, and the full scan still costs O(n) copy + sort.
   - Untyped (`any`).
   - Net: pays its complexity without using its strength.
2. **`map[int64]Task` guarded by `sync.RWMutex`**
   - Any compound operation (ID generation + insert; exists + version check + write) is atomic
     inside one critical section.
   - Consistent snapshots under `RLock`; type-safe; obvious to readers.
   - Recommended by the Go documentation for most use cases.

### Ordered listing

1. **Sort on read** (`map` + sort by ID in `List`)
   - Pros: no extra dependency; O(1) lookup and write.
   - Cons: O(n log n) sort per `List`, negligible at expected data size.
2. **Ordered structure keyed by ID** (B-tree, e.g. `github.com/tidwall/btree`, `github.com/google/btree`)
   - Pros: ordered iteration without sorting; efficient range scans
     (`id > cursor LIMIT k` in O(log n + k)).
   - Cons: extra dependency; O(log n) lookup and write; `List` still needs an O(n) copy under
     `RLock`, so only the sort is saved.
   - Does **not** help filtering by `status`: the tree is ordered by ID, so a status filter is
     still a full scan. That requires a secondary index regardless of the primary structure.

## Decision

- IDs are auto-increment `int64`, generated **inside the same write lock** that inserts the task.
  The counter only increases, so **IDs are never reused** after deletion; a stale ID held by a
  client can never address an unrelated new task.
- All repository methods take `context.Context` as the first parameter (unused in memory), so a
  database implementation can honor cancellation and deadlines without an interface change.
- Storage is `map[int64]Task` guarded by `sync.RWMutex`:
  - `List` takes `RLock`, copies values, sorts by ID ascending (sort on read; no B-tree).
  - `Create`, `Update`, `Delete` take `Lock`.
- The map stores **values, not pointers**, and every read returns a copy, so callers can never
  mutate shared state outside the lock.
- `Update` performs existence check, version check and write in one critical section
  (see [ADR-0003](0003-optimistic-concurrency-control.md)).

## Consequences

- No duplicate IDs regardless of create concurrency.
- A task deleted between a read and an update cannot be resurrected: the update sees it missing
  and returns `ErrNotFound`.
- Data is lost on restart, as allowed by the specification.

### Trade-offs accepted

Throughput and scalability are traded for correctness that is easy to verify and simple code:

- A single global lock **serializes all writes**, and each write blocks all readers.
- `List` holds `RLock` for an O(n) copy, blocking writers for that duration.
- On high core counts, `RLock`'s shared reader counter causes cache-line contention.

These costs are negligible for the expected load: critical sections are tens of lines with no
I/O, so JSON encoding and network dominate request latency.

### Revisit when

| Signal | Option |
|---|---|
| Write lock contention across different tasks | Sharded map |
| Reads dominate writes, `List` latency matters | Copy-on-write snapshot |
| `GET /tasks/{id}` added and read-dominated | Re-evaluate `sync.Map` |
| Cursor pagination by ID over large data sets | Ordered structure (B-tree) keyed by ID |
| Filtering by `status` becomes slow on large data sets | Secondary index `map[Status]map[int64]struct{}` |
| Persistence or multi-instance needed | Real database with `UPDATE ... WHERE version = ?` |

## Compliance

- Tests run with `go test -race`.
- Concurrency tests: N goroutines creating tasks concurrently yield N distinct IDs; concurrent
  update/delete on the same ID never resurrects a deleted task.

## Notes

- A persistent implementation only needs to satisfy the same repository interface and provide
  the same atomic compare-and-set semantics (e.g. `UPDATE ... WHERE id = ? AND version = ?`).
- Filtering and sorting (e.g. `GET /tasks?status=0&sort=-id`) are out of scope. They can be added
  later as optional query parameters without breaking clients (see
  [ADR-0004](0004-api-contract.md)); the repository `List` method would accept a filter, and
  in-memory filtering is an O(n) scan, the same cost as the current `List`.
