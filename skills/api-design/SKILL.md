---
name: api-design
description: Design or review an HTTP API with apic as the proving ground, resources and URLs, methods and status codes, error bodies, pagination, filtering, versioning, idempotency, caching, rate limits, async work and the OpenAPI document that describes them, each decision checked by a request `apic run` or a mock `apic mock` serves. Use whenever the user is building a new API or endpoint, writing or reviewing an OpenAPI document, naming routes, choosing status codes or an error format, planning a breaking change, or asks whether an API is well designed, even when they do not name apic.
---

# Designing an HTTP API

Design the contract before the code, then prove the code against the
contract with apic. The `apic` skill beside this one covers the commands;
this one covers the decisions, each with the request that checks it.
[references/http-semantics.md](references/http-semantics.md) has the full
method, status code and header tables; [references/openapi.md](references/openapi.md)
a contract skeleton and the review checklist.

## The loop

1. Write (or change) the OpenAPI document first: paths, operations with an
   `operationId`, request and response schemas, an example on every response.
2. `apic mock openapi.yaml --port 8090`, then `apic import openapi.yaml`:
   one request per operation, named from the `operationId`. Run them with
   `--var baseUrl=http://localhost:8090` and the design is callable before
   a line of server code exists; a `Prefer: code=404` header line asks the
   mock for another documented response.
3. Turn the imported requests into the acceptance tests: `# @assert` on
   status, headers and body shape, `# @capture` the ids later calls need,
   `# @step` phrases and `features/*.feature` for the flows that matter.
4. Build the server until `apic run api.http --json` and `apic test --json`
   pass against it, not the mock; keep both in CI.

Edit the document when a request reveals a gap, never only the server:
the document is the contract, and `apic validate --json` plus `apic import`
again keeps the requests honest.

## Resources and URLs

- Nouns, plural, lower-case, kebab-case: `/order-items`, not `/getOrderItem`.
  One level of nesting when the child cannot exist alone (`/orders/{id}/items`);
  otherwise a top-level collection with a filter (`/items?orderId=`).
- Identifiers are opaque strings the client never constructs or parses.
  Prefer UUIDs or prefixed ids (`ord_01H…`) to sequential integers, which
  are guessable and leak volume.
- An action that is not a CRUD verb is a sub-resource you `POST` to:
  `POST /orders/{id}/cancel`, `POST /reports/{id}/runs`. Never a verb in
  the path of a `GET`.
- Query parameters filter, sort and page a collection; the path identifies.
  No trailing slashes, no file extensions, no version or auth in the query.
- Pick one JSON casing (`camelCase` is the common choice) and one time format
  (RFC 3339 UTC, `2026-10-30T12:00:00Z`) and hold to them everywhere;
  `apic validate` cannot catch inconsistency, so a shared schema can.

## Methods and status codes

| Do | Method and success | Then |
|---|---|---|
| Read one or many | `GET` 200; 404 when the one is missing, 200 with an empty list when the many are | safe, cacheable, no body |
| Create | `POST` 201 with `Location` and the created body | 409 when it already exists, 422 when it is invalid |
| Replace | `PUT` 200 (or 204) with the whole representation | idempotent: the same call twice leaves the same state |
| Change part | `PATCH` 200 with `Content-Type: application/merge-patch+json` | document the patch format; it is not plain JSON |
| Remove | `DELETE` 204; 404 (or 204, decide once) when already gone | idempotent |

Rules of thumb: 400 for a request the server cannot parse, 422 for one it
parsed but rejects; 401 when the caller is unknown, 403 when known and
not allowed; 404 for a missing resource and for one the caller may not
know exists; 409 for a state conflict; 429 with `Retry-After` for a limit;
5xx only for the server's own fault, never for a client mistake; and
never 200 with an error inside the body. Check each in the file:

```http
### A missing order is a 404 problem, not a 200 with null
# @name get-missing-order
# @assert status == 404
# @assert header.content-type contains application/problem+json
GET {{baseUrl}}/orders/{{$uuid}}
```

## Errors

Return RFC 9457 problem details, `application/problem+json`, for every
non-2xx: `type` (a stable URI the client can switch on), `title`,
`status`, `detail` (for a person, safe to show), `instance`, plus an
`errors` array for validation (`pointer` and `detail` per field). Never a
stack trace, an SQL fragment, a hostname or a different shape per endpoint.
Send a request id (`X-Request-Id`) on every response and log it with the
error, so a report quotes one string. Assert the shape once, in a
shared feature step, and every endpoint inherits the check.

## Collections

- Page every list, with a default and a maximum `limit` (say 20 and 100),
  and never return everything because nobody asked for a page.
- Cursor pagination (`?cursor=` and a `nextCursor` in the body or a
  `Link: <…>; rel="next"` header) is stable under inserts; offset
  pagination (`?page=` or `?offset=`) is fine for small, slow-changing sets.
  Say which, and only one.
- Filter with named parameters that mirror fields (`?status=done`), sort with
  `?sort=-createdAt,name`, and reject unknown parameters with 400 rather than
  silently ignoring a typo that returns everything.
- Totals are expensive: offer `?include=total` or omit, do not compute by default.
- Prove the contract with the `.#` count and a filter selector:

```http
### Only completed todos come back, at most a page
# @name list-done-todos
# @assert status == 200
# @assert body.$.items[?(@.done != true)].# == 0
# @assert body.$.items.# <= 20
GET {{baseUrl}}/todos?done=true&limit=20
```

## Change and versions

Add fields, endpoints and optional parameters freely: clients ignore what
they do not know, and the document's `additionalProperties` should let them.
A breaking change (removing or renaming a field, changing a type, tightening
validation, changing an error `type`) gets a new major version, `/v2/` in
the path, the form every client, proxy and `.http` file handles, and the
old one keeps working with `Deprecation` and `Sunset` headers until the
announced date. Keep a `.http` file per live version so both stay provable.

## Idempotency, concurrency, caching and long work

- Clients retry, so make retrying safe: `PUT` and `DELETE` by definition;
  `POST` by an `Idempotency-Key` header the server remembers for a day and
  answers with the original response.
- Protect against lost updates with `ETag` on reads and `If-Match` on
  writes, answering 412 when it does not match; serve 304 to `If-None-Match`.
- Set `Cache-Control` on purpose: `no-store` for anything per-user,
  `max-age` for public, slow-changing data. Silence means every proxy guesses.
- Work longer than a few seconds returns 202 with a status URL; the client
  polls it until `state` leaves `running`. Write that as the request, and
  the poll as `# @retry`:

```http
### Wait for the export to finish
# @name wait-for-export
# @retry 20 2s
# @assert body.$.state == done
GET {{baseUrl}}/exports/{{exportId}}
```

## The OpenAPI document

Write OpenAPI 3.1: one `operationId` per operation (it becomes the
request name), every response with a schema and an `example` (the mock
serves it), shared schemas under `components` (the problem shape, the page
envelope, each resource), `securitySchemes` declared and applied, and
`servers` holding the real base URLs. Lint it in CI, keep it in the
repository beside the `.http` files, and treat a diff to it as the review:
the server follows the document, not the other way round.

## Before you call it done

Walk [the checklist](references/openapi.md#review-checklist): every
operation named, every response typed and exampled, errors one shape,
lists paged, writes idempotent, ids opaque, times RFC 3339, auth declared,
and one `.http` request with assertions per operation, passing against the
real server with `apic run --json` and in CI with `apic test`.
