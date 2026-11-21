# ADR-0007: Request idempotency via Idempotency-Key

- **Status:** Accepted
- **Date:** 2026-11-21

## Context

Clients retry requests when a response is lost (timeout, dropped connection). Without
protection, retries are unsafe:

- **`PUT` with optimistic locking is not idempotent** ([ADR-0003](0003-optimistic-concurrency-control.md)):
  `PUT` with `version: 1` succeeds and the task becomes version 2; the response is lost; the retry
  with `version: 1` gets `409 Conflict` although the client's change was applied.
- **`POST` creates duplicates**: each retry creates a new task with a new ID.

- **`DELETE` retries are ambiguous**: `DELETE` is idempotent in effect (RFC 9110 §9.2.2 defines
  idempotency by server state, and a retry leaves the task equally absent), but the retry
  returns `404`. The client cannot tell "my delete succeeded" from "wrong ID, never existed".

## Considered options

### Retry safety for `POST` and `PUT`

1. **Content-based idempotency for `PUT`**: if the stored task already equals the requested
   content, return `200` regardless of version.
   - Pros: zero storage; no client changes.
   - Cons: `PUT` only; does not solve duplicate `POST`.
2. **`Idempotency-Key` header** (Stripe-style; IETF draft *The Idempotency-Key HTTP Header Field*).
   - Pros: uniform for `POST` and `PUT`; exact replay of the original response.
   - Cons: key storage with expiry; in-flight concurrency handling; client must generate keys.
3. **Client-side reconciliation**: on `409`, the client compares `current` with its intent.
   - Cons: pushes complexity to every client; does not solve duplicate `POST`.

### Retry ambiguity for `DELETE`

1. **Always `204`**, even if the task does not exist (as S3 `DeleteObject` does).
   - Pros: simplest for clients.
   - Cons: a wrong ID silently "succeeds", hiding client bugs.
2. **Keep `404`** and document that it is still idempotent.
   - Cons: retry ambiguity remains.
3. **`410 Gone` for deleted IDs.** IDs are monotonic and never reused
   ([ADR-0002](0002-in-memory-storage.md)), so `id <= counter` but absent means deleted (`410`),
   and `id > counter` means never existed (`404`); no extra storage.
   - Pros: retries are distinguishable without client changes.
   - Cons: adds a status code to the contract; `PUT` would need `410` too for consistency;
     does not say *who* deleted it.
4. **Apply `Idempotency-Key` to `DELETE`.**
   - Pros: same mechanism as `POST`/`PUT`; a retry replays the original `204`; a wrong ID
     without a key still gets `404`; contract unchanged.

### Outcome of failed executions (`5xx`, panic)

1. **Release the key** so the same key can be retried.
   - Pros: transient failures are retryable without a new key.
   - Cons: if the failure happened after side effects, the retry executes again and can
     create a duplicate, defeating the purpose of the key.
2. **Store the failure and replay it** (Stripe's behavior).
   - Pros: at-most-once execution per key; simpler store (no release path).
   - Cons: transient failures require a new key. In this in-memory service `5xx` indicates a
     bug rather than a transient dependency failure, so the cost is low.

## Decision

Adopt `Idempotency-Key` (option 2 for `POST`/`PUT`, option 4 for `DELETE`) on
`POST /tasks`, `PUT /tasks/{id}` and `DELETE /tasks/{id}`, storing failed outcomes (option 2
for failed executions).

### Protocol

- Header: `Idempotency-Key: <opaque string>`, **optional**. Without it, behavior is unchanged
  (no retry protection).
- Key: 1–255 printable ASCII characters; clients should use a UUID. Otherwise
  `400 IDEMPOTENCY_KEY_INVALID`.
- Not applied to `GET` (safe; nothing to replay).
- `DELETE` has no body, so its fingerprint covers method and path only.

### Server behavior

Each key maps to a record `{fingerprint, state, response, expires_at}`, where
`fingerprint = SHA-256(method + path + body)`.

| Situation | Result |
|---|---|
| New key | Reserve as `in_progress`, run the handler, store the response, mark `completed` |
| Completed key, same fingerprint | **Replay** the stored status, body and `Location`; add `Idempotent-Replayed: true` |
| Completed key, different fingerprint | `422 IDEMPOTENCY_KEY_MISMATCH` |
| Key `in_progress` (concurrent duplicate) | `409 IDEMPOTENCY_KEY_IN_USE`; client retries later |
| Handler returned `5xx` | Store and mark `completed`; replays return the same `5xx` |
| Handler panicked | Store a standard `500 INTERNAL_ERROR` response, mark `completed`, re-panic |

**Storage rule: once execution has started, the outcome is stored, whatever it is.**

| Stage | Stored? |
|---|---|
| Rejected before execution: invalid key (`400`), key in progress (`409`), fingerprint mismatch (`422`) | No |
| Any outcome after execution started: `2xx`, `4xx`, `5xx`, panic | Yes, replayed |

- This gives **at-most-once execution** per key. A `5xx` or panic may occur after side effects
  (e.g. the task was created, then response encoding panicked); the middleware cannot know,
  so it never allows a second execution under the same key.
- After a `5xx`, clients check the actual state (`GET /tasks`) and retry with a **new** key.

#### Client recovery after a `5xx`

| Method | Failure before side effect | Failure after side effect | Is a retry with a new key safe? |
|---|---|---|---|
| `POST` | Task not created; retry creates it | Task created; retry **creates a duplicate** | **No**, unless the client confirms the task does not exist |
| `PUT` | Unchanged (`v1`); retry with `v1` → `200` | Applied (`v2`); retry with `v1` → `409` with `current` showing the change | **Yes**: the version check prevents double application |
| `DELETE` | Task exists; retry → `204` | Task gone; retry → `404` (already deleted) | **Yes**: effect is idempotent |

Only `POST` relies on stored failures to prevent duplicates; `PUT` and `DELETE` are protected
by optimistic locking and idempotent effect respectively. The same rule applies to all three
for a single, predictable client flow.
- Records expire after **24 hours**. Expired records are removed lazily on access and by a
  periodic sweeper that stops on shutdown.
- Reserve and complete are atomic operations on the store (single mutex). There is no
  release operation: every reservation ends `completed`.

### Placement

```
internal/idempotency/           Store interface, Record/State types, errors, in-memory Store + sweeper
internal/transport/http/middleware/
                                gin middleware: header validation, fingerprint, reserve/replay/complete
```

- `internal/idempotency` is a standalone package with no dependency on gin or HTTP. It stores
  opaque responses (`status`, `headers`, `body`) keyed by idempotency key.
- The middleware in `transport/http/middleware` imports it. Dependency direction stays
  `transport → idempotency`, consistent with [ADR-0001](0001-layered-ddd-architecture.md);
  `infrastructures` is not involved, so no package depends on `transport`.
- The store can be unit-tested without HTTP; the middleware is tested with `httptest`.
- The middleware runs inside the recovery middleware, so it uses its own `defer recover()`
  to store the `500` outcome on panic and then re-panics for the outer recovery
  (see the middleware chain in [ADR-0004](0004-api-contract.md)).
- Expiry uses the injected clock ([ADR-0006](0006-timestamps-and-clock-injection.md)).

## Consequences

- With a key, retrying `POST` never creates duplicates, retrying `PUT` returns the original
  `200` instead of a spurious `409`, and retrying `DELETE` returns the original `204` instead
  of an ambiguous `404`.
- Without a key, `DELETE` of a missing task returns `404`, so wrong IDs are still reported.
- Optimistic locking is unchanged: a *different* request with a stale version still gets `409`.
- Memory grows with distinct keys within 24 hours; bounded by the sweeper.
- Request bodies are buffered to compute the fingerprint; bounded by the 1 MB body limit
  ([ADR-0004](0004-api-contract.md)).
- The fingerprint is computed over the **raw body bytes**. Semantically equal JSON with different
  whitespace or key order is a different request and gets `422` when reusing a key. This is
  deliberate (same as Stripe): clients retry by resending the identical bytes, and JSON
  canonicalization would add cost and edge cases.
- The idempotency record and the task write are not one atomic unit. Irrelevant in memory (a
  crash loses both); a persistent implementation must store both in the same transaction.

## Compliance

- Middleware tests: replay returns identical status, body and `Location` with
  `Idempotent-Replayed: true`; fingerprint mismatch → `422`; concurrent same key → exactly one
  handler execution and `409` for the other; `5xx` and panic are stored and replayed as the same
  `5xx` without re-running the handler; expiry via fake clock.
- Handler tests: `POST` retried with the same key creates exactly one task; `PUT` retried after
  success returns `200`; `DELETE` retried after success returns `204`; `DELETE` of a missing
  task without a key returns `404`.

## Notes

- Request tracing (`X-Request-ID`) is a separate concern and out of scope.
- **`POST` reconciliation limit:** after a `5xx` on `POST`, a client can only look for its task by
  `name`, which is not unique. A client-supplied reference (e.g. a unique `client_ref` field)
  would make reconciliation exact; out of scope.
- Keys share one global namespace. With authentication, keys should be scoped per client so
  one client cannot receive another's replayed response by reusing a key.
