---
name: api-security
description: Secure an HTTP API you own or are authorised to test, and prove it with apic, authentication and token handling, authorisation on every object, property and function, input validation and resource limits, security headers and CORS, secrets and logging, and the OWASP API Security Top 10, each control written as an `.http` request or Gherkin scenario that fails when the control is missing. Use whenever the user asks whether an API is secure, is adding login, API keys or OAuth2, reviewing or hardening an endpoint before release, handling tokens or secrets in requests, or wants security checks in CI, even when they do not name apic.
---

# Securing an HTTP API

A security control that nothing exercises decays. This skill turns each
control into a request whose expected answer is a refusal (401, 403,
400, 413, 415, 429), kept in the project's `.http` files and features and
run on every change, so a regression fails CI like any other. The `apic`
skill beside this one has the commands; `api-design` has the contract.
[references/owasp-api-top-10.md](references/owasp-api-top-10.md) expands
each risk below, [references/checks.md](references/checks.md) is the bank
of ready requests and scenarios.

Work only on an API you own or have written permission to test, against
a non-production environment where possible, and never send real
customer data in a probe. Negative requests are cheap; use `--dry-run`
before anything that writes.

## How to work

1. Inventory: `apic list --json` (or the OpenAPI document) is the attack
   surface. Every operation, every parameter, every response field.
2. For each operation, write the refusals it must give: no token, the wrong
   user's token, a token with too little scope, a body it should reject.
   Put them in a `security.http` beside the happy paths, tagged `@security`
   in a feature.
3. Run them: `apic run security.http --json --keep-going`, then
   `apic test --tags @security --json` in CI. A check that passes against
   the mock proves only the document; the real server is the target.
4. Keep secrets out of the files: credentials live in
   `http-client.private.env.json` (gitignored) or `APIC_VAR_name`; `--redact`
   when output is stored. Never `--insecure` outside localhost.

## The OWASP API Security Top 10, as checks

| Risk | Build | Prove |
|---|---|---|
| API1 Broken object level authorisation | Check the caller owns or may see the object on every `{id}` route, in one place | `GET /orders/{otherUsersOrder}` with user A's token is 404 or 403 |
| API2 Broken authentication | Short-lived tokens, verified signature, issuer, audience and expiry; lockout and rate limits on login; no credentials in URLs | No token is 401 with `WWW-Authenticate`; an expired, tampered or `alg: none` token is 401; the key in a query string is refused |
| API3 Broken object property level authorisation | Allow-list the fields a role may read and write; never bind the request body straight to the model | `PATCH` with `role: admin` or `balance` leaves them unchanged; responses hold no `passwordHash`, internal ids or other users' data |
| API4 Unrestricted resource consumption | Body size, page size, rate, timeout and upload limits; paid actions metered | A 2 MB body is 413; `limit=100000` is clamped or 400; the 101st call in a minute is 429 with `Retry-After` |
| API5 Broken function level authorisation | Deny by default; admin routes behind a role, not a hidden path | `DELETE /users/{id}` and `/admin/*` with a plain user's token is 403 |
| API6 Unrestricted access to sensitive business flows | Rate, proof-of-work or review on flows that hurt when automated (sign-up, checkout, votes, referrals) | A burst through the flow is 429 or challenged |
| API7 Server-side request forgery | Allow-list hosts for any URL the API fetches; block link-local and private ranges; no redirects followed | A webhook or image URL of `http://169.254.169.254/` or `http://localhost/` is 400 |
| API8 Security misconfiguration | TLS only, HSTS, `nosniff`, `no-store` on private responses, CORS allow-list, verbose errors off, debug routes gone | Headers asserted on every response; a 500 has no stack trace; `OPTIONS` from an unknown origin gets no `Access-Control-Allow-Origin` |
| API9 Improper inventory management | One document that lists every version and environment; old versions sunset, debug and beta hosts closed | `/v1/` after its sunset is 410; nothing undocumented answers |
| API10 Unsafe consumption of APIs | Validate and bound what upstream APIs return; TLS verified; timeouts | The client code treats upstream bodies like user input; check it in review |

## Authentication

- Bearer JWT for users and services, with `exp` minutes not days, `aud`
  and `iss` checked, the algorithm pinned server-side, and a refresh token
  that rotates. Reject before touching the database.
- API keys for machine callers: random, shown once, stored hashed, sent in
  a header (`X-Api-Key` or `Authorization`), never a query string (it lands
  in logs and browser history), scoped and revocable.
- OAuth2 client credentials between services, authorisation code with PKCE
  for users; mTLS where both ends are yours.
- A 401 says `WWW-Authenticate: Bearer realm="api"`; a 403 says nothing more
  than the status. Neither reveals whether the account exists.

Each form is a `# @auth` line in apic (`bearer`, `basic`, `apikey`,
`oauth2`, `aws`, `exec`), so the happy path and the refusals share a file:

```http
### Without credentials the API refuses
# @name whoami-anonymous
# @auth none
# @assert status == 401
# @assert header.www-authenticate startsWith Bearer
GET {{baseUrl}}/me

### With a token that expired yesterday it refuses too
# @name whoami-expired
# @auth none
# @assert status == 401
GET {{baseUrl}}/me
Authorization: Bearer {{expiredToken}}
```

## Authorisation

Object-level (API1) and function-level (API5) failures are the commonest
and the costliest, and both are caught the same way: two identities in
the environment, and a request that uses the wrong one.

```gherkin
@security
Scenario: A user cannot read another user's order
  Given the variable "token" is "{{tokenUserB}}"
  When I run "get-order" with:
    | orderId | {{orderOwnedByA}} |
  Then the response status is 404

@security
Scenario: A user cannot promote themselves
  When I run "patch-me" with:
    | body | {"role": "admin"} |
  Then the response is successful
  When I run "whoami"
  Then the response body "$.role" is "user"
```

Keep the ids and tokens of both users as variables (captured by two login
requests, or fixtures in the private env file), so the scenarios run in
any environment. Decide once whether a forbidden object is 403 or 404
(404 hides existence; 403 is clearer for a first-party client) and assert it.

## Input and resources

- Validate every field: type, length, range, format, enumeration; reject
  unknown fields in writes (`additionalProperties: false`) rather than
  ignore them. Prefer 422 with the field pointer.
- Enforce `Content-Type` (415 otherwise), a body size limit (413), a
  maximum page size, a request timeout, and upload type and size checks.
- Parameterise every query; never build a filter, a path or a shell
  command from a request value. Treat ids as opaque strings, not numbers
  to arithmetic on.
- Rate limit per caller (not per IP alone), answer 429 with `Retry-After`,
  and send `RateLimit` headers so good clients back off.

```http
### An oversized body is refused before it is parsed
# @name create-note-too-large
# @assert status == 413
POST {{baseUrl}}/notes
Content-Type: application/json

< ./fixtures/two-megabytes.json
```

## Transport, headers and errors

Serve HTTPS only, redirect nothing (an API client that sent a token over
HTTP has already leaked it; refuse with 400 and fix the client), and send
on every response: `Strict-Transport-Security`, `X-Content-Type-Options:
nosniff`, `Cache-Control: no-store` for anything authenticated, and a CORS
allow-list of exact origins with `Vary: Origin`, never `*` beside
`Access-Control-Allow-Credentials: true`. Errors are problem documents
with a `type`, a `title` and a request id, nothing about the stack, the
query or the host. Assert the headers in one shared scenario and run it
against every endpoint that matters.

## Secrets and logging

Credentials belong in `http-client.private.env.json`, `.env` or the shell,
never in a `.http` file, the public env file or a commit; the output masks
`Authorization` and other sensitive headers in its output and `--redact`
masks bodies and captures too. Server-side: hash API keys and passwords
(bcrypt, scrypt or Argon2), rotate signing keys with a `kid`, log the
request id and the caller id but never a token, a password or a full
card number, publish a `/.well-known/security.txt` (RFC 9116), and keep
dependencies patched; an API is as safe as its JSON parser.

## Before release

Run [the release checklist](references/checks.md#release-checklist):
every route authenticates, every object route authorises, every write
validates and bounds, every response carries the headers, every error is
a problem document, every secret is out of the repository, and
`apic test --tags @security` is green in CI against the real server.
