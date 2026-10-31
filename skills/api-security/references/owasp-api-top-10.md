# The OWASP API Security Top 10 (2023), with apic checks

The [OWASP API Security Top 10](https://owasp.org/API-Security/editions/2023/en/0x11-t10/)
names the ten ways APIs most often fail. For each: what it is, what to
build, and the request or scenario that fails while the control is
missing. The requests assume a `security.http` beside the project's other
files, two identities in the environment (`tokenUserA`, `tokenUserB`,
`orderOwnedByA`) and the happy-path requests (`get-order`, `whoami`,
`patch-me`, `login`) already written. The
[OWASP REST Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/REST_Security_Cheat_Sheet.html)
covers the same ground as prose.

## API1 Broken object level authorisation

The caller asks for `/orders/{id}` with someone else's id and gets it.
The commonest API flaw, because the check is per route and one route
forgets. Build: resolve the caller first, then load the object *scoped to
the caller* (`WHERE id = ? AND owner_id = ?`), in one helper every handler
uses; never trust an owner id from the body. Prove, for every route with
an id:

```http
### Another user's order is not mine to read
# @name get-order-as-other-user
# @auth bearer {{tokenUserB}}
# @assert status == 404
GET {{baseUrl}}/orders/{{orderOwnedByA}}

### Nor to change
# @name patch-order-as-other-user
# @auth bearer {{tokenUserB}}
# @assert status == 404
PATCH {{baseUrl}}/orders/{{orderOwnedByA}}
Content-Type: application/merge-patch+json

{"status": "cancelled"}
```

Run the pair with `--data` over a CSV of every id-bearing route to cover
the surface in one command.

## API2 Broken authentication

Weak or missing checks on who the caller is: tokens that never expire,
signatures not verified, `alg: none` accepted, passwords without lockout,
keys in URLs. Build: a well-tested JWT library with the algorithm pinned,
`exp`, `iss` and `aud` verified; refresh tokens that rotate and are
revocable; login rate-limited and locked after a burst; API keys hashed
at rest and sent only in headers. Prove:

```http
### No credentials
# @name me-anonymous
# @auth none
# @assert status == 401
# @assert header.www-authenticate startsWith Bearer
GET {{baseUrl}}/me

### A token signed with the wrong key (or alg none)
# @name me-forged-token
# @auth none
# @assert status == 401
GET {{baseUrl}}/me
Authorization: Bearer {{forgedToken}}

### The key in the query string is not accepted
# @name me-key-in-query
# @auth none
# @assert status == 401
GET {{baseUrl}}/me?api_key={{apiKey}}

### Many wrong passwords lock the door
# @name login-wrong-password
# @auth none
# @assert status != 200
POST {{baseUrl}}/auth/login
Content-Type: application/json

{"user": "{{user}}", "password": "not-the-password-{{$randomInt 1 100000}}"}
```

Run the last with `--data attempts.csv` holding twenty rows and assert
the final ones are 429 or 423, not 401.

## API3 Broken object property level authorisation

The object is the caller's, but the response leaks fields they may not see
(`passwordHash`, `internalNotes`, another user's email) or the request
sets fields they may not change (`role`, `balance`, `verified`): mass
assignment. Build: explicit response models per role, never the database
row; explicit allow-lists of writable fields per role; `additionalProperties:
false` on request schemas. Prove:

```http
### The profile hides what the user may not see
# @name me-no-secrets
# @assert status == 200
# @assert body.$.passwordHash not exists
# @assert body.$.internalNotes not exists
GET {{baseUrl}}/me

### Promoting yourself is ignored or refused
# @name patch-me-role
# @assert status == 422
PATCH {{baseUrl}}/me
Content-Type: application/merge-patch+json

{"role": "admin"}
```

If the API ignores unknown fields instead (200), follow with `whoami` and
`# @assert body.$.role == user`.

## API4 Unrestricted resource consumption

No limits on body size, page size, upload size, request rate, concurrent
jobs, or calls to paid third parties (SMS, email, geocoding). Build: limits
at the edge and in the handler, 413 for bodies, a clamped or rejected
`limit`, 429 with `Retry-After` and `RateLimit` headers, timeouts on every
upstream call, quotas on paid actions. Prove:

```http
### A huge page is not served
# @name list-orders-huge-page
# @assert status == 400
GET {{baseUrl}}/orders?limit=1000000

### A huge body is refused
# @name create-note-huge
# @assert status == 413
POST {{baseUrl}}/notes
Content-Type: application/json

< ./fixtures/two-megabytes.json

### A burst is limited (run with --data of 200 identical rows)
# @name burst
# @assert status < 500
GET {{baseUrl}}/orders?limit=1
```

After the burst: `apic history burst --json` shows which runs were 429, and
`apic select burst header.retry-after` the back-off the last one asked for.

## API5 Broken function level authorisation

Admin or staff functions reachable by changing the path or method:
`DELETE /users/{id}`, `/admin/reports`, `POST /refunds`. Build: deny by
default, a role check in middleware keyed on the route, not on the client
hiding a button; no "internal" routes relying on obscurity. Prove:

```http
### A user cannot delete users
# @name delete-user-as-user
# @auth bearer {{tokenUserA}}
# @assert status == 403
DELETE {{baseUrl}}/users/{{userIdB}}

### A user cannot read the admin report
# @name admin-report-as-user
# @auth bearer {{tokenUserA}}
# @assert status == 403
GET {{baseUrl}}/admin/reports/revenue
```

## API6 Unrestricted access to sensitive business flows

Each call is valid; the harm is in automation: buying out stock, mass
sign-ups, vote stuffing, referral farming, scraping. Build: identify the
flows that hurt when scripted, then rate-limit them per account and per
device, add friction (a challenge, a delay, a review) above a threshold,
and watch for anomalies. Prove with `--data` over a few hundred rows of a
sign-up or checkout request and assert that the later rows are 429 or
challenged; keep the test against a non-production environment.

## API7 Server-side request forgery

The API fetches a URL the client supplied (a webhook, an avatar, an
import) and can be pointed at the cloud metadata service, localhost or
an internal host. Build: allow-list schemes (`https` only) and hosts,
resolve DNS and refuse private, loopback and link-local ranges, do not
follow redirects, short timeouts, and fetch from a network segment that
sees nothing internal. Prove:

```http
### A webhook may not point inside
# @name webhook-metadata
# @assert status == 400
POST {{baseUrl}}/webhooks
Content-Type: application/json

{"url": "http://169.254.169.254/latest/meta-data/"}

### Nor at localhost
# @name webhook-localhost
# @assert status == 400
POST {{baseUrl}}/webhooks
Content-Type: application/json

{"url": "http://127.0.0.1:8080/admin"}
```

## API8 Security misconfiguration

Missing TLS or HSTS, permissive CORS, verbose errors, default credentials,
debug endpoints and directory listings left on, unnecessary methods
allowed. Build: a hardened base configuration applied everywhere, headers
set by middleware, stack traces off outside development, `OPTIONS` answered
from an allow-list of origins. Prove with one shared scenario:

```gherkin
@security
Scenario Outline: Every response carries the security headers
  When I run "<request>"
  Then the response header "strict-transport-security" contains "max-age="
  And the response header "x-content-type-options" is "nosniff"
  And the response header "cache-control" contains "no-store"
  Examples:
    | request     |
    | whoami      |
    | list-orders |
    | get-order   |
```

```http
### An unknown origin gets no CORS grant
# @name preflight-unknown-origin
# @auth none
# @assert status < 500
# @assert header.access-control-allow-origin not exists
OPTIONS {{baseUrl}}/orders
Origin: https://evil.example.com
Access-Control-Request-Method: GET

### A 500 says nothing about the inside
# @name error-is-opaque
# @assert status == 500
# @assert header.content-type contains application/problem+json
# @assert body.$.requestId exists
# @assert body.$.stack not exists
# @assert body.$.detail matches ^[^\n]{0,200}$
GET {{baseUrl}}/debug/boom
```

## API9 Improper inventory management

Nobody knows which versions, hosts and environments are live: `/v1/` kept
running without patches, a `beta.` host with production data, a staging
API with no auth. Build: one OpenAPI document per version, in the
repository, with `servers` listing every live host; a sunset date on every
retired version answered with 410; the same authentication everywhere.
Prove: a `.http` file per live version with its assertions, and for a
retired one:

```http
### The retired version is gone, not forgotten
# @name v1-retired
# @auth none
# @assert status == 410
GET {{baseUrl}}/v1/orders
```

## API10 Unsafe consumption of APIs

The API trusts data from upstream APIs (a payment provider, a partner, a
geocoder) more than from users: follows their redirects, parses their
bodies without limits, stores their strings unescaped. Build: treat an
upstream response like a request, validate against a schema, bound its
size, verify TLS, set timeouts, and do not follow redirects blindly.
Prove in code review and unit tests; from outside, `apic mock` can stand
in for the upstream with hostile examples (an oversized field, an
unexpected type) and the API's behaviour under them is a scenario.
