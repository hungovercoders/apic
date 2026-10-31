# The OpenAPI document, and the review checklist

The contract is an OpenAPI 3.1 document ([the specification](https://spec.openapis.org/oas/v3.1.0)),
kept in the repository beside the `.http` files. apic reads it twice:
`apic import openapi.yaml` writes one request per operation, and
`apic mock openapi.yaml` serves the documented responses, so the design is
callable before the server exists.

## A skeleton that imports and mocks well

```yaml
openapi: 3.1.0
info:
  title: Orders API
  version: 1.0.0
servers:
  - url: https://api.example.com/v1
paths:
  /orders:
    get:
      operationId: listOrders            # becomes the request list-orders
      summary: List orders, newest first
      parameters:
        - { name: status, in: query, schema: { type: string, enum: [open, paid, cancelled] } }
        - { name: limit,  in: query, schema: { type: integer, minimum: 1, maximum: 100, default: 20 } }
        - { name: cursor, in: query, schema: { type: string } }
      responses:
        "200":
          description: A page of orders
          content:
            application/json:
              schema: { $ref: "#/components/schemas/OrderPage" }
              example:
                items: [{ id: ord_01HZX, status: open, total: { amount: "12.50", currency: GBP }, createdAt: "2026-10-30T12:00:00Z" }]
                nextCursor: null
        "400": { $ref: "#/components/responses/BadRequest" }
        "401": { $ref: "#/components/responses/Unauthorised" }
    post:
      operationId: createOrder
      parameters:
        - { name: Idempotency-Key, in: header, required: true, schema: { type: string, maxLength: 255 } }
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: "#/components/schemas/NewOrder" }
            example: { items: [{ sku: "ABC-1", quantity: 2 }], currency: GBP }
      responses:
        "201":
          description: Created
          headers:
            Location: { schema: { type: string, format: uri } }
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Order" }
              example: { id: ord_01HZX, status: open, total: { amount: "12.50", currency: GBP }, createdAt: "2026-10-30T12:00:00Z" }
        "422": { $ref: "#/components/responses/Unprocessable" }
  /orders/{orderId}:
    parameters:
      - { name: orderId, in: path, required: true, schema: { type: string } }
    get:
      operationId: getOrder
      responses:
        "200":
          description: One order
          headers:
            ETag: { schema: { type: string } }
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Order" }
              example: { id: ord_01HZX, status: open, total: { amount: "12.50", currency: GBP }, createdAt: "2026-10-30T12:00:00Z" }
        "404": { $ref: "#/components/responses/NotFound" }
components:
  securitySchemes:
    bearer: { type: http, scheme: bearer, bearerFormat: JWT }
  schemas:
    Money:
      type: object
      required: [amount, currency]
      properties:
        amount: { type: string, pattern: "^-?\\d+\\.\\d{2}$", description: Decimal as a string; never a float }
        currency: { type: string, pattern: "^[A-Z]{3}$" }
    Order:
      type: object
      required: [id, status, total, createdAt]
      properties:
        id: { type: string }
        status: { type: string, enum: [open, paid, cancelled] }
        total: { $ref: "#/components/schemas/Money" }
        createdAt: { type: string, format: date-time }
    NewOrder:
      type: object
      required: [items, currency]
      additionalProperties: false
      properties:
        items:
          type: array
          minItems: 1
          items:
            type: object
            required: [sku, quantity]
            properties:
              sku: { type: string }
              quantity: { type: integer, minimum: 1 }
        currency: { type: string, pattern: "^[A-Z]{3}$" }
    OrderPage:
      type: object
      required: [items]
      properties:
        items: { type: array, items: { $ref: "#/components/schemas/Order" } }
        nextCursor: { type: [string, "null"] }
    Problem:
      type: object
      required: [type, title, status]
      properties:
        type: { type: string, format: uri }
        title: { type: string }
        status: { type: integer }
        detail: { type: string }
        instance: { type: string }
        requestId: { type: string }
        errors:
          type: array
          items:
            type: object
            properties: { pointer: { type: string }, detail: { type: string } }
  responses:
    BadRequest:
      description: Malformed request
      content: { application/problem+json: { schema: { $ref: "#/components/schemas/Problem" }, example: { type: "https://api.example.com/problems/malformed", title: "The request could not be parsed", status: 400 } } }
    Unauthorised:
      description: Missing or invalid credentials
      headers: { WWW-Authenticate: { schema: { type: string } } }
      content: { application/problem+json: { schema: { $ref: "#/components/schemas/Problem" }, example: { type: "https://api.example.com/problems/unauthenticated", title: "Authentication required", status: 401 } } }
    NotFound:
      description: No such resource
      content: { application/problem+json: { schema: { $ref: "#/components/schemas/Problem" }, example: { type: "https://api.example.com/problems/not-found", title: "Not found", status: 404 } } }
    Unprocessable:
      description: Valid syntax, invalid content
      content: { application/problem+json: { schema: { $ref: "#/components/schemas/Problem" }, example: { type: "https://api.example.com/problems/validation", title: "The request body is invalid", status: 422, errors: [{ pointer: "/items/0/quantity", detail: "must be at least 1" }] } } }
security:
  - bearer: []
```

What the skeleton does on purpose:

- Every operation has an `operationId` in `camelCase`; `apic import` turns
  it into the kebab-case request name (`listOrders` becomes `list-orders`),
  which is then the id for `apic run`, `apic describe`, MCP and features.
- Every response has a schema and an `example`. The mock serves the example
  (or builds one from the schema when there is none), so the imported
  requests get realistic bodies, and `Prefer: code=422` on a request asks
  for that response.
- Money is a decimal string, times are `date-time`, ids are strings, enums
  are closed lists, request bodies set `additionalProperties: false` so a
  misspelt field is a 422 rather than silently dropped. Response schemas
  leave `additionalProperties` open so adding a field is not a break.
- Errors are one `Problem` schema reused through `components/responses`,
  so every endpoint's error looks the same and one feature step checks them.
- `security` applies the bearer scheme to everything; an open endpoint
  overrides it with `security: []` on the operation, which is visible in review.

## From document to proof

```sh
apic mock openapi.yaml --port 8090 &              # the documented responses, on localhost
apic import openapi.yaml                           # one request per operation, in orders.http
apic validate --json                               # every request parses
apic run orders.http --json --var baseUrl=http://localhost:8090   # the design, end to end
```

Then make the requests strict: add `# @assert body.$ matchesSchema
./schemas/order.json` where the body matters (export the schema from the
document, or keep JSON Schema files beside it), capture ids
(`# @capture orderId = body.$.id`) so the flow reads what it created, and
move the flows a release depends on into `features/*.feature`. Point
`baseUrl` at the real server and the same files are its acceptance tests;
in CI, `apic test --format junit --output report.xml`.

Keep the document and the requests in one pull request: a change to one
without the other is the review comment.

## Review checklist

Naming and shape:

- Paths are plural nouns in kebab-case, at most one level of nesting; no
  verbs except as `POST` sub-resources for state transitions.
- Ids are opaque strings; nothing sequential or guessable is exposed.
- One JSON casing, one time format (RFC 3339 UTC), money as decimal strings,
  enumerations closed and documented.
- Every operation has an `operationId`, a `summary`, and typed parameters
  with limits (`maximum`, `maxLength`, `pattern`).

Responses:

- Every status the server can return is listed, with a schema and an example.
- Success codes follow the method (201 with `Location` on create, 204 on
  delete, 202 with a status URL for long work).
- Every error is `application/problem+json` with the shared `Problem`
  schema and a stable `type`; 400 and 422, 401 and 403, 404 and 409 are
  used for what they mean; nothing returns 200 with an error inside.

Collections:

- Paged with a default and a maximum `limit`; one pagination style, stated.
- Filters and sorts are named parameters; unknown parameters are 400.
- Totals are opt-in.

Safety and change:

- `PUT` and `DELETE` are idempotent in the implementation, not only by name;
  `POST` that creates takes `Idempotency-Key`.
- Writes that can race have `ETag` and `If-Match`; reads that are cached
  have `Cache-Control` set on purpose.
- Breaking changes go in a new major path version; the old version carries
  `Deprecation` and `Sunset`.
- `securitySchemes` are declared and applied; an open endpoint says so with
  `security: []` and a comment on why.

Proof:

- One `.http` request per operation with at least a status assertion, the
  error paths included, passing against the mock and the real server.
- The flows a release depends on are `.feature` scenarios in CI.
- The document, the `.http` files and the server change in the same pull
  request.
