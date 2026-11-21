# ADR-0004: API contract: semantics, status codes and response format

- **Status:** Accepted
- **Date:** 2026-11-21

## Context

The specification lists four endpoints and two required fields (`name`, `status`) but leaves
semantics, status codes and payload shapes open. The contract must be consistent, predictable,
and extensible without breaking clients.

## Considered options

### Success payload

1. **Envelope everything** (`{"success": true, "result": ...}`).
   - Cons: duplicates information already carried by the HTTP status; extra unwrapping.
2. **Bare resource everywhere**, including a top-level array for lists.
   - Cons: a top-level array cannot gain pagination metadata without a breaking change.
3. **Bare single resources; lists wrapped in an object** (Stripe-style).
   - Pros: minimal for single resources; lists can gain `next_cursor`, `total`, etc. later.

### Error payload

1. **Plain string message.**
   - Cons: clients must parse human text to branch.
2. **RFC 9457 Problem Details** (`application/problem+json`).
   - Pros: standard. Cons: verbose; less widely adopted by API clients.
3. **Custom object with machine-readable `code`.**
   - Pros: simple, stable codes for clients, room for field-level details.

### PUT semantics

1. **Full replacement** — all fields required.
2. **Partial update** — only provided fields change (this is PATCH semantics).

## Decision

### Resource

```json
{
  "id": 1,
  "name": "Buy milk",
  "status": 0,
  "version": 1,
  "created_at": "2026-11-21T08:00:00Z",
  "updated_at": "2026-11-21T08:00:00Z"
}
```

`status`: `0` = incomplete, `1` = completed.

### Endpoints

| Endpoint | Request body | Success | Errors |
|---|---|---|---|
| `GET /tasks` | — | `200` `{"data": [task, ...]}` ordered by `id` asc; empty → `{"data": []}` | — |
| `POST /tasks` | `name` (required), `status` (optional, default `0`) | `201` task + `Location: /tasks/{id}` | `400`, `409`, `413`, `415`, `422` |
| `PUT /tasks/{id}` | `name`, `status`, `version` — **all required** (full replacement) | `200` task | `400`, `404`, `409`, `413`, `415`, `422` |
| `DELETE /tasks/{id}` | — | `204` no body | `400`, `404`, `409`, `422` |

- `POST`, `PUT` and `DELETE` accept an optional `Idempotency-Key` header ([ADR-0007](0007-idempotency-key.md)).
- `POST` and `PUT` require `Content-Type: application/json` (parameters such as `; charset=utf-8`
  allowed); otherwise `415`.
- Request bodies are limited to **1 MB** (`http.MaxBytesReader`); larger bodies get `413`.
- `id` is taken from the path only; an `id` in the body is ignored.
- `id`, `version`, `created_at`, `updated_at` are server-managed; clients cannot set them
  (`version` in `PUT` is a precondition, not a new value).
- **Unknown request fields are ignored.**

### Errors

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "invalid request body",
    "details": [
      { "field": "name", "reason": "must not be blank" }
    ]
  }
}
```

| Situation | Status | `code` |
|---|---|---|
| Malformed JSON (syntax error) | 400 | `INVALID_JSON` |
| Field validation failed, including wrong JSON types (e.g. `"status": "1"`) | 400 | `VALIDATION_FAILED` (all failing fields in `details`) |
| Path `id` not a positive integer | 400 | `INVALID_ID` |
| Invalid `Idempotency-Key` | 400 | `IDEMPOTENCY_KEY_INVALID` |
| Task not found | 404 | `TASK_NOT_FOUND` |
| Unknown route | 404 | `ROUTE_NOT_FOUND` |
| Method not allowed on an existing route | 405 | `METHOD_NOT_ALLOWED` (with `Allow` header) |
| Version mismatch | 409 | `VERSION_CONFLICT` (includes `current` task) |
| Same `Idempotency-Key` in flight | 409 | `IDEMPOTENCY_KEY_IN_USE` |
| Body over 1 MB | 413 | `PAYLOAD_TOO_LARGE` |
| Wrong `Content-Type` | 415 | `UNSUPPORTED_MEDIA_TYPE` |
| `Idempotency-Key` reused with a different request | 422 | `IDEMPOTENCY_KEY_MISMATCH` |
| Unexpected error or panic | 500 | `INTERNAL_ERROR` (no internal details leaked) |

Router-level responses use the same format: gin's `NoRoute` and `NoMethod` handlers are set,
`HandleMethodNotAllowed` is enabled, and `RedirectTrailingSlash` is disabled so `/tasks/` is a
`404` rather than a redirect.

### Error mapping

- The domain defines sentinel errors (`ErrNotFound`, `ErrVersionConflict`) and a
  `ValidationError` type carrying field-level details.
- The application layer returns domain errors unchanged (wrapping with `%w` when adding
  context) and knows nothing about HTTP.
- The transport layer maps errors to responses in **one place**, using `errors.Is` /
  `errors.As`. Any unmapped error becomes `500 INTERNAL_ERROR`; the original error is logged,
  never returned to the client.

### Middleware chain

Order matters; outer middleware wraps inner:

```
Logger
└─ Recovery            custom; panics → 500 INTERNAL_ERROR in the standard error format
   └─ BodyLimit        1 MB via http.MaxBytesReader → 413
      └─ ContentType   POST/PUT only → 415
         └─ Idempotency  POST/PUT/DELETE, when the header is present (ADR-0007)
            └─ Handler
```

- **Body limit before idempotency:** the fingerprint buffers the body, so the limit must
  already be in place.
- **Content-Type before idempotency:** a `415` is a malformed request and must not reserve or
  store an idempotency key.
- **Recovery outside idempotency:** idempotency stores the `500` outcome for the key in its own
  `defer recover()` and re-panics, so the outer recovery still produces the `500`.
- Content-Type applies only to routes with a body (`POST /tasks`, `PUT /tasks/{id}`);
  idempotency applies to all mutating routes (`POST`, `PUT`, `DELETE`). Both are attached
  per route rather than globally.

## Consequences

- Clients branch on `code`; `message` wording can change freely.
- All validation errors are reported at once.
- List responses can be extended with pagination without breaking clients.
- Full-replacement `PUT` keeps semantics unambiguous; partial updates would need a future `PATCH`.
- Ignoring unknown fields favors forward compatibility over catching client typos.

## Compliance

- Handler tests (`httptest`) cover every row of the endpoints and errors tables.
- Panics are recovered by a custom recovery middleware returning the `INTERNAL_ERROR` format.
- Swagger annotations document every status code and payload.

## Out of scope

| Topic | Position |
|---|---|
| Authentication / authorization | Not implemented; the API is public |
| Rate limiting | Expected at the gateway in production |
| CORS | Not configured |
| Pagination, filtering, sorting options | Not implemented; additive later via query parameters ([ADR-0002](0002-in-memory-storage.md)) |
| Bounded storage | Task count is unbounded; memory grows with data |
| API versioning (`/v1`) | Paths are fixed by the specification |
| Request tracing, structured logging | `gin.Logger` only; production would use `slog` with `X-Request-ID` |

## Notes

- RFC 9457 can be adopted later in a superseding ADR if interoperability requires it.
