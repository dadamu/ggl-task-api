# ADR-0003: Optimistic concurrency control via body version

- **Status:** Accepted
- **Date:** 2026-11-21
- **Amended:** 2026-11-22 — Notes: retry guidance now points to ADR-0010

## Context

A mutex prevents data races but not **lost updates**, which span multiple requests:

```
t0  A: GET  /tasks/1 → version 1
t1  B: GET  /tasks/1 → version 1
t2  A: PUT  /tasks/1 → succeeds, version 2
t3  B: PUT  /tasks/1 → silently overwrites A's change
```

We also need to handle a task being deleted between a client's read and its update.

## Considered options

1. **Last write wins** (no detection).
   - Pros: simplest.
   - Cons: silent data loss.
2. **`ETag` + `If-Match` header** (RFC 9110 conditional requests), `412 Precondition Failed`.
   - Pros: HTTP standard; keeps metadata out of the representation; also works for `DELETE`;
     enables conditional `GET` (`If-None-Match` → `304`).
   - Cons: more implementation (quoted/weak ETags); less discoverable via curl and Swagger;
     browsers need CORS `Access-Control-Expose-Headers` to read `ETag`.
3. **`version` field in the request body**, `409 Conflict`.
   - Pros: simple; visible in Swagger and responses; precedent in Kubernetes
     (`metadata.resourceVersion` + `409`).
   - Cons: cannot protect `DELETE` (no request body); version mixes into the representation.

## Decision

Adopt option 3.

- Every task has a `version` (`int64`), starting at `1` and incremented on each successful update.
- `PUT /tasks/{id}` **requires** `version`.
- The repository's `Update` atomically checks, under the write lock:
  1. task exists, otherwise `ErrNotFound` → `404`;
  2. stored version equals the request version, otherwise `ErrVersionConflict` → `409`;
  3. writes the task with `version + 1`.
- A `409` response includes the current task so the client can merge and retry without an extra `GET`.
- `DELETE` is **not** version-checked.

## Consequences

- Lost updates are impossible: every update must prove it is based on the latest version.
- Delete-then-update is safe: the update returns `404`; a deleted task is never resurrected.
- Update-then-delete is not detected: a client may delete a task modified after its last read.
  Accepted because, for a task list, deletion expresses intent regardless of intermediate edits.
- Clients must read before writing; a naive `PUT` without `version` gets a clear `400`.

## Compliance

- Unit tests: stale version → `ErrVersionConflict`; missing task → `ErrNotFound`; success increments version.
- Handler tests: `409` body contains the current task; missing `version` → `400`.
- Concurrency test: two updates with the same version → exactly one succeeds.

## Notes

- Version preconditions make a retried `PUT` fail with `409` after the original succeeded.
  The `409` carries `current`, so the client can tell whether its change was applied
  ([ADR-0010](0010-defer-idempotency-key.md); `Idempotency-Key` from ADR-0007 is not implemented).
- Every successful `PUT` increments `version` and sets `updated_at`, even if the content is unchanged.
- If `DELETE` protection becomes necessary, add an optional `?version=` query parameter, or
  migrate to `ETag`/`If-Match` in a superseding ADR.
