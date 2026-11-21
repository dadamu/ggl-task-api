# ADR-0006: Timestamps and clock injection

- **Status:** Accepted
- **Date:** 2026-11-21

## Context

Tasks expose `created_at` and `updated_at`. Calling `time.Now()` directly in business code makes
tests non-deterministic: assertions on timestamps become flaky or must be skipped.

## Considered options

1. **Call `time.Now()` directly.**
   - Cons: untestable timestamps.
2. **`Clock` interface** injected into the application layer.
   - Pros: explicit. Cons: an interface and fake type for a single method.
3. **`func() time.Time` injected into the application layer; domain receives `now` as a parameter.**
   - Pros: idiomatic and minimal; domain stays a pure function of its inputs.
4. **`testing/synctest`** (Go 1.25+) to fake time inside tests.
   - Pros: no production code change. Cons: implicit; ties tests to a newer testing facility.

## Decision

Adopt option 3.

```go
// domain: time is an input, never read from the environment
func New(name string, status Status, now time.Time) (Task, error)
func (t *Task) Update(name string, status Status, now time.Time) error

// application: clock is injected
type Service struct {
	repo  task.Repository
	clock func() time.Time
}
```

- `cmd/server` wires `time.Now`; tests pass a fixed or stepping clock.
- `POST` sets `created_at = updated_at = now`.
- `PUT` sets `updated_at = now`; `created_at` is preserved.
- All timestamps are **UTC**, truncated to **millisecond** precision when set
  (`now.UTC().Truncate(time.Millisecond)`), serialized as RFC 3339 (e.g. `2026-11-21T08:00:00.123Z`).
  This gives stable output and exact equality in tests; `UTC()` also strips the monotonic clock reading.

## Consequences

- Timestamp behavior is asserted exactly in tests.
- No production code calls `time.Now()` outside the composition root.
- `version`, not `updated_at`, is used for concurrency control
  ([ADR-0003](0003-optimistic-concurrency-control.md)); timestamps are informational only.

## Compliance

- Domain and application tests use a fixed clock and assert exact timestamps.
- Code review: `time.Now()` appears only in `cmd/server`.
