# ADR-0001: Layered DDD architecture

- **Status:** Accepted
- **Date:** 2026-11-21
- **Amended:** 2026-11-21 — Compliance: dependency direction enforced by `depguard` instead of code review

## Context

The service is a small CRUD API today, but it should be easy to test in isolation, to swap the
in-memory storage for a real database, and to add other entry points (e.g. gRPC, CLI) later
without touching business rules.

## Considered options

1. **Flat package** (handlers talk to a global map).
   - Pros: least code.
   - Cons: business rules mixed with HTTP and storage; hard to unit test; storage swap touches everything.
2. **Layered DDD with dependency inversion** (domain / application / infrastructure / transport).
   - Pros: clear boundaries; domain is pure and trivially testable; storage and transport are replaceable.
   - Cons: more packages and some pass-through code for a simple CRUD domain.
3. **Full tactical DDD** (aggregates, domain events, CQRS).
   - Pros: scales to complex domains.
   - Cons: over-engineering for a four-endpoint API.

## Decision

Adopt option 2, kept deliberately lean.

```
cmd/server/                    composition root: wiring only
internal/
├── domain/task/               entity, value objects, invariants, repository interface, domain errors
├── application/task/          use cases; orchestrates domain and repository
├── infrastructures/           repository implementations, config
└── transport/http/            gin router, handlers, DTOs, error mapping
```

Dependency direction: `transport → application → domain ← infrastructures`.

- The repository **interface** lives in `domain`; implementations live in `infrastructures`.
- All application code lives under `internal/` so it cannot be imported by other modules.
- No aggregates, domain events or CQRS.

## Consequences

- Domain logic is tested without HTTP or storage.
- Replacing in-memory storage with a database requires only a new repository implementation.
- The application layer may be thin for some use cases; this is accepted in exchange for a stable seam.

## Compliance

- Dependency direction is enforced by `depguard` in golangci-lint (run by `make lint` and CI):
  - `domain` may import only the standard library and `internal/domain` (strict allow list).
  - `application` must not import `transport`, `infrastructures` or gin.
  - `infrastructures` must not import `transport` or `application`.
- Test files are exempt so tests can use testify.

## Notes

- See [ADR-0002](0002-in-memory-storage.md) for the repository implementation.
