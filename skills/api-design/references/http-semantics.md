# HTTP semantics for API design

The tables the briefing summarises, with the request that proves each
row. Sources: [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110) (HTTP
semantics), [RFC 9111](https://www.rfc-editor.org/rfc/rfc9111) (caching),
[RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) (problem details),
[RFC 9205](https://www.rfc-editor.org/rfc/rfc9205) (building protocols on
HTTP), [RFC 8594](https://www.rfc-editor.org/rfc/rfc8594) (the `Sunset`
header) and the IETF drafts for
[rate limit headers](https://datatracker.ietf.org/doc/html/draft-ietf-httpapi-ratelimit-headers)
and the [`Idempotency-Key` header](https://datatracker.ietf.org/doc/html/draft-ietf-httpapi-idempotency-key-header).

## Methods

| Method | Safe | Idempotent | Request body | Typical success | Notes |
|---|---|---|---|---|---|
| `GET` | yes | yes | no | 200 | Cacheable. A body on `GET` has no defined meaning; use query parameters or a `POST …/search` for a complex query. |
| `HEAD` | yes | yes | no | 200 | `GET` without the body; same headers. |
| `OPTIONS` | yes | yes | no | 204 | CORS preflight; `Allow` lists the methods. |
| `POST` | no | no | yes | 201, 200, 202 | Create (201 with `Location`), act (200), start long work (202). Make it retry-safe with `Idempotency-Key`. |
| `PUT` | no | yes | yes | 200, 204, 201 | Replace the whole representation; 201 when the client chose the id and it did not exist. |
| `PATCH` | no | no | yes | 200 | Partial update. Name the format: `application/merge-patch+json` ([RFC 7396](https://www.rfc-editor.org/rfc/rfc7396)) or `application/json-patch+json` ([RFC 6902](https://www.rfc-editor.org/rfc/rfc6902)). |
| `DELETE` | no | yes | rarely | 204, 200 | 200 when the response carries something (a job, a receipt); otherwise 204. |

## Status codes worth using

| Code | Use it for | Not for |
|---|---|---|
| 200 OK | a read, a replace or an action that returns a body | a failure described in the body |
| 201 Created | a `POST` or `PUT` that made something; send `Location` | an update |
| 202 Accepted | work that continues after the response; send a status URL | a synchronous result |
| 204 No Content | success with nothing to say (`DELETE`, some `PUT`) | a `GET` (return an empty list or 404) |
| 304 Not Modified | an `If-None-Match` or `If-Modified-Since` hit | anything without a validator |
| 400 Bad Request | malformed syntax: bad JSON, a wrong type in a query parameter, an unknown parameter | a well-formed body that fails business rules |
| 401 | no credentials or bad credentials; send `WWW-Authenticate` | a caller who is known but not allowed |
| 403 Forbidden | a known caller who may not do this | hiding that a resource exists (use 404) |
| 404 Not Found | a missing resource, or one this caller may not know exists | an empty collection |
| 405 Method Not Allowed | the path exists, the method does not; send `Allow` | an unknown path |
| 406 Not Acceptable | the `Accept` header asks for a type you cannot produce | |
| 409 Conflict | a state clash: a duplicate unique value, a transition the current state forbids | validation of a single field |
| 410 Gone | removed on purpose, permanently (a sunset version) | something that may come back |
| 412 Precondition Failed | `If-Match` did not match: someone else wrote first | |
| 413 Content Too Large | a body over the documented limit | |
| 415 Unsupported Media Type | a `Content-Type` you do not accept | a body you accept but cannot parse (400) |
| 422 Unprocessable Content | well-formed, understood, rejected by the rules | malformed input (400) |
| 429 Too Many Requests | a rate or quota limit; send `Retry-After` | a server under load (503) |
| 500 Internal Server Error | the server's own fault; log the request id, say nothing else | anything the client could fix |
| 503 Service Unavailable | down on purpose or overloaded; `Retry-After` when known | a bug (500) |

Codes a client should never see: 418 and the other jokes, 200 for an
error, 500 for a validation failure, 403 where 401 is meant (it tells the
client to re-authenticate rather than give up).

## The problem details shape

```json
{
  "type": "https://api.example.com/problems/validation",
  "title": "The request body is invalid",
  "status": 422,
  "detail": "2 fields failed validation",
  "instance": "/orders/ord_01HZX",
  "requestId": "4f1c…",
  "errors": [
    {"pointer": "/items/0/quantity", "detail": "must be at least 1"},
    {"pointer": "/currency", "detail": "must be a three-letter ISO 4217 code"}
  ]
}
```

- `type` is a stable identifier (a URL you control is conventional; it
  need not resolve) that a client switches on. Add a new `type` rather
  than changing the meaning of an old one.
- `title` is the same for every occurrence of a `type`; `detail` is
  specific to this one and safe to show a user.
- Extension members (`requestId`, `errors`) are allowed; document them.
- Set `Content-Type: application/problem+json`, so a client can tell an
  error body from a success body without parsing it.

The one check every endpoint shares, as a request and as a scenario:

```http
### A malformed body is a 400 problem
# @name create-order-malformed
# @assert status == 400
# @assert header.content-type contains application/problem+json
# @assert body.$.type exists
# @assert body.$.title exists
# @assert body.$.status == 400
POST {{baseUrl}}/orders
Content-Type: application/json

{not json
```

```gherkin
Scenario: Every error is a problem document
  When I run "create-order-malformed"
  Then the response is a client error
  And the response header "content-type" contains "application/problem+json"
  And the response body "$.type" exists
  And the response body "$.status" is "400"
```

## Headers an API should send or read

| Header | Direction | Why |
|---|---|---|
| `Content-Type` | both | Always, with `charset=utf-8` for text; reject bodies with the wrong one (415). |
| `Accept` | request | Honour it or answer 406; default to JSON. |
| `Location` | response | Where the created resource lives (201) or where to poll (202). |
| `ETag` / `If-Match` / `If-None-Match` | both | Optimistic concurrency and conditional reads. |
| `Cache-Control` | response | Say what may be cached; `no-store` for per-user data. |
| `Idempotency-Key` | request | A client-chosen key that makes a `POST` retry-safe. |
| `Retry-After` | response | Seconds (or a date) with 429 and 503. |
| `RateLimit` / `RateLimit-Policy` | response | The limit, what remains and when it resets, per the IETF draft. |
| `Link` | response | `rel="next"` for cursor pagination, `rel="describedby"` for the document. |
| `Deprecation` / `Sunset` | response | A version or endpoint on its way out, with the date. |
| `X-Request-Id` | both | One id to find a request in the logs; echo the client's when it sends one. |

Check a header the same way as a body: `# @assert header.location startsWith
{{baseUrl}}/orders/` or `Then the response header "etag" exists`.

## Pagination in detail

Cursor:

```http
### First page
# @name list-orders
# @assert status == 200
# @assert body.$.items.# <= 50
# @capture nextCursor = body.$.nextCursor
GET {{baseUrl}}/orders?limit=50

### Next page
# @name list-orders-next
# @ref list-orders
# @assert status == 200
GET {{baseUrl}}/orders?limit=50&cursor={{nextCursor}}
```

The cursor is opaque (an encoded sort key, not a page number), a page
holds the items plus `nextCursor` (absent or null on the last page), and
`limit` above the maximum is clamped or rejected, never silently obeyed.

Offset: `?offset=100&limit=50` with `total` optional. Simple, sortable by
any field, and wrong under concurrent inserts (an item appears twice or
never). Fine for admin screens, wrong for feeds and exports.

## Long-running work

```
POST /exports            → 202 Accepted, Location: /exports/exp_01HZ…
GET  /exports/exp_01HZ…  → 200 {"state": "running"}
GET  /exports/exp_01HZ…  → 200 {"state": "done", "resultUrl": "…"}
```

The status resource is a normal resource: it has an id, it is readable
by `GET`, its `state` is a small enumeration (`queued`, `running`, `done`,
`failed`), and a failure carries a problem document under `error`. A
client that polls deserves a `Retry-After` hint on the status response.
`# @retry 20 2s` on the `GET` with `# @assert body.$.state == done` turns
the wait into one request.
