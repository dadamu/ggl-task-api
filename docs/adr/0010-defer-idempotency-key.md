# ADR-0010: Defer Idempotency-Key

- **Status:** Accepted
- **Date:** 2026-11-22
- **Supersedes:** [ADR-0007](0007-idempotency-key.md); the Idempotency-Key parts of
  [ADR-0004](0004-api-contract.md) and [ADR-0009](0009-testing-strategy.md)

## Context

[ADR-0007](0007-idempotency-key.md) designed an `Idempotency-Key` header so that retried
`POST`, `PUT` and `DELETE` requests replay the original response. Re-evaluating the value of
that mechanism for this API, the consequence of each unprotected retry is:

| Retried request | Outcome without Idempotency-Key | Impact |
|---|---|---|
| `POST /tasks` | A duplicate task is created | Low: a task list has no side effects beyond the record; the duplicate can be deleted |
| `PUT /tasks/{id}` | `409 VERSION_CONFLICT` | None: the `409` already carries the `current` task ([ADR-0003](0003-optimistic-concurrency-control.md)), so the client can see whether its change was applied and resend only if needed |
| `DELETE /tasks/{id}` | `404 TASK_NOT_FOUND` | None: the task is absent either way |

Optimistic locking already makes `PUT` retries safe, and `DELETE` is idempotent in effect. Only
`POST` gains anything, and the gain is avoiding a harmless duplicate.

The cost is a key store with expiry and a background sweeper, a middleware that buffers and
fingerprints request bodies, three extra error codes, and roughly 400 lines of code and tests.

## Considered options

1. **Implement ADR-0007 as designed.**
   - Pros: exact replay for every retried mutation.
   - Cons: the cost above for a benefit limited to avoiding duplicate tasks.
2. **Defer: do not implement; keep ADR-0007 as the design to use if needed.**
   - Pros: smaller surface; no state beyond tasks; retries rely on mechanisms that already exist.
   - Cons: a retried `POST` after a lost response can create a duplicate.

## Decision

Adopt option 2. `Idempotency-Key` is not implemented.

The following no longer apply:

- [ADR-0007](0007-idempotency-key.md) in full.
- [ADR-0004](0004-api-contract.md): the optional `Idempotency-Key` header; the error codes
  `IDEMPOTENCY_KEY_INVALID`, `IDEMPOTENCY_KEY_IN_USE` and `IDEMPOTENCY_KEY_MISMATCH` and the
  `409`/`422` responses they produce; the `Idempotency` step of the middleware chain. The chain is
  `Logger → Recovery → BodyLimit → ContentType (POST/PUT) → Handler`.
- [ADR-0009](0009-testing-strategy.md): the `idempotency` and idempotency middleware rows of the
  test table.

Client guidance for retries after a lost response:

- `POST`: list tasks and check for the task before retrying, or accept a possible duplicate.
- `PUT`: on `409`, compare `current` with the intended change; resend with `current.version`
  only if the change is not already applied.
- `DELETE`: treat `404` on retry as success.

## Consequences

- No idempotency state, sweeper or body fingerprinting; the request path stays
  `Logger → Recovery → BodyLimit → ContentType → Handler`.
- A retried `POST` may create a duplicate task.
- The ADR-0007 design remains the reference if retries become automatic and duplicates matter.

## Compliance

- No `Idempotency-Key` handling exists in the code; the header is ignored.

## Notes

- Revisit when clients retry automatically (e.g. mobile apps on unreliable networks) and
  duplicate tasks become a real problem, or when a mutation with costly side effects is added.
