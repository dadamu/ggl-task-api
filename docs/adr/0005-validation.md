# ADR-0005: Validation responsibilities and rules

- **Status:** Accepted
- **Date:** 2026-11-21

## Context

Input must be validated, but rules can live in the transport layer (gin binding tags backed by
`go-playground/validator`) or in the domain. Rules enforced only at the HTTP edge are bypassed
by any other entry point (tests calling the application layer, future gRPC/CLI), allowing
invalid tasks to exist.

Two Go-specific pitfalls also apply:

- JSON decoding cannot distinguish a missing `int` field from an explicit `0`.
- `binding:"required"` rejects `""` but accepts whitespace-only strings such as `"   "`.

## Considered options

1. **All rules in gin binding tags** (plus custom validators).
   - Pros: declarative; errors produced before reaching business code.
   - Cons: domain invariants unenforced for non-HTTP callers; custom validators needed anyway.
2. **All rules in the domain.**
   - Pros: single source of truth.
   - Cons: presence of fields is a transport concern the domain cannot see after decoding.
3. **Split: transport checks shape, domain enforces invariants.**

## Decision

Adopt option 3. Each rule is written exactly once.

| Layer | Responsibility | Mechanism |
|---|---|---|
| Transport | Field presence and JSON types | gin `binding:"required"`; pointer fields (`*int`, `*int64`) so missing ≠ zero |
| Domain | Business invariants | `task.New(...)` / `task.Update(...)` return field-aware validation errors |

### Domain invariants

| Field | Rule |
|---|---|
| `name` | Not blank: `strings.TrimSpace(name) != ""` |
| `name` | At most **255 characters**, counted as runes (`utf8.RuneCountInString`), not bytes |
| `name` | Stored as provided (not trimmed) |
| `status` | Must be `0` or `1` |

Domain validation errors carry the field name and reason; the transport layer maps them to
`400 VALIDATION_FAILED` with `details` (see [ADR-0004](0004-api-contract.md)).

## Consequences

- Invariants hold regardless of entry point; an invalid `Task` cannot be constructed.
- Domain validation is tested with plain table-driven tests, no HTTP required.
- Two layers produce errors; the transport layer must merge binding and domain errors into one
  response format.

## Compliance

- Table-driven domain tests: empty, whitespace-only, 255 and 256 runes (including multi-byte
  characters), status `-1`, `0`, `1`, `2`.
- Handler tests: missing `name`, missing `status` on `PUT`, missing `version` on `PUT`.
