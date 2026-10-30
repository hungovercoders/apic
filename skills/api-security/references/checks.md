# Security checks, ready to paste

A bank of requests and scenarios, each the negative of a control. Copy
the ones that apply into a `security.http` and `features/security.feature`
beside the project's other files, replace the placeholders, and run them
with `apic run security.http --json --keep-going` and `apic test --tags
@security --json`. Everything here assumes `baseUrl` and two identities in
the environment; the ids and tokens come from `# @capture` on login and
fixture requests, never from the files.

## Environment shape

`http-client.env.json` (public):

```json
{
  "$shared": { "problemType": "https://api.example.com/problems/" },
  "staging": { "baseUrl": "https://staging.api.example.com", "userIdA": "usr_A", "userIdB": "usr_B" }
}
```

`http-client.private.env.json` (gitignored, or `APIC_VAR_passwordA` in CI):

```json
{
  "staging": { "passwordA": "…", "passwordB": "…", "apiKey": "…", "forgedToken": "eyJ…", "expiredToken": "eyJ…" }
}
```

Two logins capture the tokens the checks need:

```http
### Log in as A
# @name login-a
# @auth none
# @assert status == 200
# @capture tokenUserA = body.$.accessToken
POST {{baseUrl}}/auth/login
Content-Type: application/json

{"user": "{{userIdA}}", "password": "{{passwordA}}"}

### Log in as B
# @name login-b
# @auth none
# @assert status == 200
# @capture tokenUserB = body.$.accessToken
POST {{baseUrl}}/auth/login
Content-Type: application/json

{"user": "{{userIdB}}", "password": "{{passwordB}}"}
```

Set `auth.default` in `apic.yaml` to `bearer {{tokenUserA}}` so the happy
paths need no directive, and give each negative request its own `# @auth`.

## Authentication

```http
### No credentials is 401 with a challenge
# @name auth-none
# @auth none
# @assert status == 401
# @assert header.www-authenticate startsWith Bearer
# @assert header.content-type contains application/problem+json
GET {{baseUrl}}/me

### Garbage is 401, not 500
# @name auth-garbage
# @auth none
# @assert status == 401
GET {{baseUrl}}/me
Authorization: Bearer not.a.token

### Expired is 401
# @name auth-expired
# @auth none
# @assert status == 401
GET {{baseUrl}}/me
Authorization: Bearer {{expiredToken}}

### Wrong signature is 401
# @name auth-forged
# @auth none
# @assert status == 401
GET {{baseUrl}}/me
Authorization: Bearer {{forgedToken}}

### Basic auth is not an alternative door
# @name auth-basic-refused
# @auth basic {{userIdA}} {{passwordA}}
# @assert status == 401
GET {{baseUrl}}/me

### The key belongs in a header, not the URL
# @name auth-key-in-query
# @auth none
# @assert status == 401
GET {{baseUrl}}/me?api_key={{apiKey}}

### Logging out invalidates the token
# @name logout
# @ref login-b
# @auth bearer {{tokenUserB}}
# @assert status == 204
POST {{baseUrl}}/auth/logout

### After logout the token is dead
# @name auth-after-logout
# @ref logout
# @auth bearer {{tokenUserB}}
# @assert status == 401
GET {{baseUrl}}/me
```

## Authorisation

```http
### Object level: B cannot read A's order
# @name authz-read-other
# @ref login-b
# @auth bearer {{tokenUserB}}
# @assert status == 404
GET {{baseUrl}}/orders/{{orderOwnedByA}}

### Object level: B cannot change A's order
# @name authz-write-other
# @ref login-b
# @auth bearer {{tokenUserB}}
# @assert status == 404
PATCH {{baseUrl}}/orders/{{orderOwnedByA}}
Content-Type: application/merge-patch+json

{"status": "cancelled"}

### Object level: B cannot list A's orders by filter
# @name authz-list-other
# @ref login-b
# @auth bearer {{tokenUserB}}
# @assert status == 200
# @assert body.$.items[?(@.ownerId != '{{userIdB}}')].# == 0
GET {{baseUrl}}/orders?ownerId={{userIdA}}

### Function level: a user cannot reach admin routes
# @name authz-admin-route
# @assert status == 403
GET {{baseUrl}}/admin/users

### Function level: a user cannot use an admin method
# @name authz-admin-method
# @assert status == 403
DELETE {{baseUrl}}/users/{{userIdB}}

### Property level: the response hides internals
# @name authz-no-leak
# @assert status == 200
# @assert body.$.passwordHash not exists
# @assert body.$.internal not exists
GET {{baseUrl}}/me

### Property level: privileged fields are not writable
# @name authz-mass-assignment
# @assert status == 422
PATCH {{baseUrl}}/me
Content-Type: application/merge-patch+json

{"role": "admin", "verified": true, "credit": 1000000}
```

```gherkin
@security
Feature: Authorisation holds between users

  Background:
    Given I run "login-a"
    And I run "login-b"

  Scenario: An order is visible to its owner only
    When I run "get-order" with:
      | orderId | {{orderOwnedByA}} |
    Then the response status is 200
    Given the variable "token" is "{{tokenUserB}}"
    When I run "get-order" with:
      | orderId | {{orderOwnedByA}} |
    Then the response status is 404

  Scenario: A user cannot promote themselves
    When I run "authz-mass-assignment"
    Then the response is a client error
    When I run "whoami"
    Then the response body "$.role" is "user"
```

## Input validation

```http
### Malformed JSON is 400
# @name input-malformed
# @assert status == 400
# @assert header.content-type contains application/problem+json
POST {{baseUrl}}/orders
Content-Type: application/json

{"items": [

### Wrong content type is 415
# @name input-wrong-type
# @assert status == 415
POST {{baseUrl}}/orders
Content-Type: text/plain

items=1

### Unknown fields are 422 with a pointer
# @name input-unknown-field
# @assert status == 422
# @assert body.$.errors[0].pointer exists
POST {{baseUrl}}/orders
Content-Type: application/json

{"items": [{"sku": "ABC-1", "quantity": 1}], "currency": "GBP", "isAdmin": true}

### Out-of-range values are 422
# @name input-out-of-range
# @assert status == 422
# @assert body.$.errors[?(@.pointer == '/items/0/quantity')].# == 1
POST {{baseUrl}}/orders
Content-Type: application/json

{"items": [{"sku": "ABC-1", "quantity": -1}], "currency": "GBP"}

### Wrong types are 400 or 422, never 500
# @name input-wrong-type-field
# @assert status < 500
# @assert status >= 400
POST {{baseUrl}}/orders
Content-Type: application/json

{"items": "not-an-array", "currency": 12}

### Injection strings are data, not code
# @name input-injection
# @assert status < 500
GET {{baseUrl}}/orders?status=' OR 1=1 --

### Path traversal in an id is 400 or 404
# @name input-traversal
# @assert status >= 400
# @assert status < 500
GET {{baseUrl}}/files/..%2F..%2Fetc%2Fpasswd

### An oversized body is 413
# @name input-too-large
# @assert status == 413
POST {{baseUrl}}/notes
Content-Type: application/json

< ./fixtures/two-megabytes.json

### A huge page size is clamped or refused
# @name input-huge-limit
# @assert status < 500
# @assert body.$.items.# <= 100
GET {{baseUrl}}/orders?limit=1000000
```

## Rate limits and abuse

```http
### One of many (run: apic run rate-burst --data burst.csv --keep-going --json)
# @name rate-burst
# @assert status < 500
GET {{baseUrl}}/orders?limit=1

### Login is throttled (run with --data of twenty rows)
# @name rate-login
# @auth none
# @assert status != 200
POST {{baseUrl}}/auth/login
Content-Type: application/json

{"user": "{{userIdA}}", "password": "wrong-{{$randomInt 1 1000000}}"}
```

`burst.csv` is one header line (`n`) and as many rows as the limit plus
some. After the run, `apic history rate-burst --json` lists each
response; at least one should be 429 carrying `Retry-After`, and
`apic select rate-burst header.ratelimit` shows the policy the last one
advertised.

## Transport and headers

```gherkin
@security
Feature: Every response is hardened

  Scenario Outline: Security headers are present
    When I run "<request>"
    Then the response header "strict-transport-security" contains "max-age="
    And the response header "x-content-type-options" is "nosniff"
    And the response header "cache-control" contains "no-store"
    And the response header "x-request-id" exists
    And the response header "server" does not exist
    Examples:
      | request     |
      | whoami      |
      | list-orders |
      | auth-none   |
```

```http
### CORS: an unknown origin gets nothing
# @name cors-unknown-origin
# @auth none
# @assert header.access-control-allow-origin not exists
OPTIONS {{baseUrl}}/orders
Origin: https://evil.example.com
Access-Control-Request-Method: GET

### CORS: the known origin gets itself back, not *
# @name cors-known-origin
# @auth none
# @assert header.access-control-allow-origin == https://app.example.com
# @assert header.vary contains Origin
OPTIONS {{baseUrl}}/orders
Origin: https://app.example.com
Access-Control-Request-Method: GET

### Methods that are not offered are 405 with Allow
# @name method-not-allowed
# @assert status == 405
# @assert header.allow exists
TRACE {{baseUrl}}/orders

### Plain HTTP is refused, not redirected with the token
# @name plain-http
# @no-redirect
# @assert status >= 400
GET {{plainHttpBaseUrl}}/me
```

## Errors and information leakage

```http
### Errors are problem documents, with a request id and nothing else
# @name error-shape
# @assert status == 404
# @assert header.content-type contains application/problem+json
# @assert body.$.type startsWith {{problemType}}
# @assert body.$.requestId exists
# @assert body.$.stack not exists
# @assert body.$.trace not exists
GET {{baseUrl}}/orders/{{$uuid}}

### Debug and documentation routes are closed in production
# @name no-debug-routes
# @auth none
# @assert status == 404
GET {{baseUrl}}/debug/pprof/

### Nothing unexpected is served at the root
# @name root-is-quiet
# @auth none
# @assert status == 404
# @assert header.content-type contains application/problem+json
GET {{baseUrl}}/
```

## Release checklist

Authentication:

- Every route except the ones listed as public requires credentials, and
  the public ones are listed in the OpenAPI document with `security: []`.
- Tokens expire in minutes, carry `aud` and `iss`, and the algorithm is
  pinned; refresh tokens rotate and can be revoked.
- Login is rate-limited and locks after a burst; 401 never says whether the
  account exists.
- API keys are random, hashed at rest, header-only, scoped and revocable.

Authorisation:

- Every route with an id loads the object scoped to the caller through one
  shared helper; `authz-read-other` and `authz-write-other` pass for each.
- Admin and staff routes deny by default; `authz-admin-route` passes.
- Request schemas allow-list writable fields (`additionalProperties: false`);
  response models are per role; `authz-mass-assignment` and `authz-no-leak` pass.

Input and resources:

- Every field is validated for type, range, length, format; unknown fields
  are refused; errors carry pointers.
- `Content-Type` is enforced, bodies are capped, page sizes are capped,
  uploads are checked, upstream calls have timeouts.
- Per-caller rate limits answer 429 with `Retry-After` and `RateLimit`;
  sensitive flows have extra friction.
- URLs the API fetches are allow-listed and never private, loopback or
  link-local; redirects are not followed.

Transport, headers and errors:

- HTTPS only, HSTS on, plain HTTP refused rather than redirected.
- `X-Content-Type-Options: nosniff`, `Cache-Control: no-store` on
  authenticated responses, CORS from an exact allow-list with `Vary: Origin`.
- Every error is `application/problem+json` with a stable `type` and a
  request id; no stack traces, SQL, hostnames or `Server` versions.
- Debug, profiling and documentation routes are off in production;
  unused methods are 405.

Secrets and operations:

- No credential in a `.http` file, the public env file, a commit or a log;
  `http-client.private.env.json` is gitignored; CI passes secrets as
  `APIC_VAR_name`; stored output is `--redact`-ed.
- Passwords and keys are hashed; signing keys rotate with a `kid`.
- Logs hold the request id and caller id, never tokens or card numbers.
- A `/.well-known/security.txt` ([RFC 9116](https://www.rfc-editor.org/rfc/rfc9116))
  says how to report a problem; dependencies are scanned and patched.
- `apic test --tags @security` runs in CI against the real server, and a
  failure blocks the release.
