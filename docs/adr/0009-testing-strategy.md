# ADR-0009: Testing strategy

- **Status:** Accepted
- **Date:** 2026-11-21

## Context

The specification requires unit tests. The design relies on properties that are easy to get
wrong and hard to observe manually: atomic compare-and-set, unique IDs under concurrency,
idempotent replay, and exact validation boundaries. Tests must prove these, run fast and be
deterministic.

## Considered options

### Assertion library

1. **Standard library only** (`testing`, `httptest`).
   - Pros: no dependencies; idiomatic. Cons: verbose struct comparisons; easy to forget
     `t.Fatal` vs `t.Error`, letting a test continue after a failed precondition.
2. **Standard library + `go-cmp`** for struct diffs.
   - Pros: readable diffs with one small dependency. Cons: still manual fail/stop control.
3. **`testify` (`require` / `assert`).**
   - Pros: most widely used Go assertion library; `require` stops the
     test immediately on failure, preventing cascading failures and nil-pointer panics;
     built-in diffs make `go-cmp` unnecessary; misuse is caught by `testifylint`.
   - Cons: one extra test-only dependency.

### Handler test scope

1. **Mock the application layer.**
   - Cons: tests mostly verify mocks; misses wiring bugs.
2. **Real application + in-memory repository behind `httptest`.**
   - Pros: tests the actual HTTP contract end to end in-process; the in-memory store is fast
     and deterministic, so no mock is needed.

### Test doubles

1. **Generated mocks (`go.uber.org/mock` / gomock).**
   - Pros: any interface can be mocked; interaction verification.
   - Cons: `mockgen` code generation and regenerated files on every interface change; extra
     dependency; tests tend to assert call sequences, coupling them to implementation.
     Mocks do not add coverage by themselves; executing real code does.
2. **Real implementations everywhere.**
   - Cons: the in-memory repository never returns unexpected errors, so the
     `500 INTERNAL_ERROR` path is unreachable.
3. **Real implementations + small hand-written stubs for error injection.**
   - Pros: no generation or dependency; stubs embed the real repository and override only the
     method under test.

## Decision

`github.com/stretchr/testify` for assertions, real in-process wiring for handler tests, and
hand-written stubs only for error injection (no gomock).

- **`require` by default**: a failed assertion stops the test (`t.FailNow`).
- **`assert` only** when reporting several independent checks at once is more useful (e.g.
  comparing multiple response fields).
- Errors are checked with `require.ErrorIs` / `require.ErrorAs`, not equality.
- `testifylint` is enabled in golangci-lint (argument order, error assertions, etc.).

Error-injection stub:

```go
type stubRepo struct {
	task.Repository // real repository; non-overridden methods behave normally
	updateErr error
}

func (s stubRepo) Update(ctx context.Context, t task.Task) (task.Task, error) {
	if s.updateErr != nil {
		return task.Task{}, s.updateErr
	}
	return s.Repository.Update(ctx, t)
}
```

| Layer | What is tested | How |
|---|---|---|
| `domain` | Invariants and boundaries | Table-driven; no I/O |
| `application` | Use-case orchestration, timestamps | Fixed clock; in-memory repository |
| `infrastructures/persistence/memory` | CAS semantics, ID uniqueness, ordering, copy isolation | Unit + concurrency tests under `-race` |
| `idempotency` | Reserve/complete atomicity, expiry, sweeper | Unit + concurrency tests under `-race`; fake clock |
| `transport/http/middleware` | Idempotency replay, mismatch, in-flight, panic stored as `500` and replayed; middleware order | `httptest`; stub handlers |
| `transport/http` | Full API contract: every status code and error `code` in [ADR-0004](0004-api-contract.md) | `httptest` against the real router, application and in-memory repository |

### Rules

- **Table-driven** tests with `t.Run` subtests.
- **Deterministic:** no `time.Sleep`, no wall clock; the clock is injected ([ADR-0006](0006-timestamps-and-clock-injection.md)).
- **Concurrency tests** use many goroutines synchronized by a start barrier to maximize
  interleaving, and assert invariants (N creates → N distinct IDs; same-version updates →
  exactly one success; update vs delete → never resurrected).
- **Race detector always on:** `make test` runs `go test -race -count=1 ./...`.
- **Black-box packages** (`package foo_test`) for public behavior.
- Each ADR's **Compliance** section lists tests that must exist.

### Coverage

- Target: **≥ 80%** statement coverage for `internal/`, excluding generated code (`swagger/`)
  and `cmd/`. Reported by `make cover`, not enforced in CI.

## Consequences

- Handler tests double as executable API documentation.
- Little mocking: only the clock is faked, plus stubs for unexpected-error paths.
- Revisit gomock if interfaces multiply or interaction verification with external systems
  becomes necessary.
- Concurrency tests are probabilistic; the race detector and invariant assertions make
  failures likely to surface, but a passing run is not a proof.

## Compliance

- CI runs `make check`, which includes `go test -race`.
- Code review checks that new behavior adds a table case or test.
