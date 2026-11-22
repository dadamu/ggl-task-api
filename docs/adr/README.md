# Architecture Decision Records

This directory records significant design decisions for the Task API, following the
[AWS Prescriptive Guidance ADR process](https://docs.aws.amazon.com/prescriptive-guidance/latest/architectural-decision-records/adr-process.html).

Each ADR captures one decision: its context, the options considered, the decision, and its consequences.

Once accepted:

- **Context, Considered options and Decision are immutable.** A changed decision is recorded
  as a new ADR that supersedes the old one.
- **Compliance, Notes and links may be amended** to stay accurate (e.g. a manual check
  replaced by an automated one). Each amendment adds an `Amended` line with the date and reason.

## Index

| ADR | Title | Status |
|-----|-------|--------|
| [0001](0001-layered-ddd-architecture.md) | Layered DDD architecture | Accepted |
| [0002](0002-in-memory-storage.md) | In-memory storage with map and RWMutex | Accepted |
| [0003](0003-optimistic-concurrency-control.md) | Optimistic concurrency control via body version | Accepted |
| [0004](0004-api-contract.md) | API contract: semantics, status codes and response format | Accepted |
| [0005](0005-validation.md) | Validation responsibilities and rules | Accepted |
| [0006](0006-timestamps-and-clock-injection.md) | Timestamps and clock injection | Accepted |
| [0007](0007-idempotency-key.md) | Request idempotency via Idempotency-Key | Superseded by 0010 |
| [0008](0008-runtime-and-deployment.md) | Runtime and deployment | Accepted |
| [0009](0009-testing-strategy.md) | Testing strategy | Accepted |
| [0010](0010-defer-idempotency-key.md) | Defer Idempotency-Key | Accepted |
| [0011](0011-cursor-pagination.md) | Cursor pagination for GET /tasks | Accepted |

## Template

```markdown
# ADR-NNNN: Title

- **Status:** Proposed | Accepted | Deprecated | Superseded by ADR-XXXX
- **Date:** YYYY-MM-DD
- **Amended:** YYYY-MM-DD — reason (optional, one line per amendment)

## Context
What is the issue that motivates this decision?

## Considered options
Options evaluated, with pros and cons.

## Decision
What we decided to do.

## Consequences
What becomes easier or harder as a result, including trade-offs accepted.

## Compliance
How we verify the decision is followed (tests, lint, review).

## Notes
Related links, follow-ups, open questions.
```
