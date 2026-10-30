---
title: "Using apic from an AI agent"
description: "Drive an API from an AI agent with apic: the shell contract, stable --json output, exit codes, the error catalogue, the MCP server and the Agent Skill."
---
apic is designed so an agent can drive an API with the same files a human
uses in their editor. Three integration styles are available.

## 1. Shell (works with any agent that can run commands)

Put this in the project's `AGENTS.md` or `CLAUDE.md`:

```markdown
## Calling the API

Requests live in `api/*.http` and are run with `apic` (install: see README).

- `apic list --json`: every request with id, method, URL and description
- `apic describe <id> --json`: variables the request needs and whether it is ready
- `apic run <id> --json`: send it; prints one JSON object with `ok`, `request`, `response`, `captures`, `asserts`
- `apic run <id> --body-only`: just the response body, for piping to jq
- `apic run <file>.http --json`: run a whole file in order as a flow (NDJSON)
- `apic run <id> --var name=value`: override a variable
- `apic run <id> --env staging`: pick an environment from http-client.env.json
- `apic curl <id>`: the equivalent curl command; `apic snippet <id> --lang python` the same in HTTPie, PowerShell, Python, JavaScript or Go
- `apic test --json`: run the Gherkin features in features/; `apic test --steps --json` lists the steps you may use
- `apic validate --json`: parse every file and report problems before running anything
- `apic run <id> --dry-run --json`: the request as it would be sent, resolved, without sending it; look before a call that changes something
- `apic run <id> --assert '<expr>' --capture name=selector`: try a check or a selector for one run before writing it into the file
- `apic fmt <file>.http#<id>`: format the one request you added and nothing else
- `apic run <id> --json --body-limit 8k`: a bounded body, marked `body_truncated`; `apic select <id> body.$.<path>` reads any part of the whole response afterwards without sending again

Exit codes: 0 ok, 1 assertion failed, 2 usage/parse/missing variable, 3 network.
On exit 2 or 3 with --json, stderr holds one object `{"error": {"code", "message", "hint", ...}}`:
branch on `error.code` (E101 missing variable, E201 unknown request, E301 could not connect, ...),
not on the message; https://apic.sh/errors/ explains each code.
Values captured with `# @capture` (like a login token) persist in `.apic/session.json`,
so run `login` once and dependent requests will find the token. A request that declares
`# @ref login` runs login by itself when the token is missing. If a request reports a
missing variable, the error says which request captures it.
```

The `--json` shape is stable:

```json
{
  "ok": true,
  "request": {"name": "get-user", "file": "users.http", "line": 10, "method": "GET", "url": "https://...", "headers": {"Accept": "application/json"}},
  "response": {"status": 200, "status_text": "OK", "headers": {"content-type": "application/json"}, "body": {"id": 42}, "duration_ms": 87, "size": 412},
  "captures": {"email": "a@b.c"},
  "asserts": [{"expr": "status == 200", "pass": true, "actual": "200", "expected": "200"}]
}
```

`response.body` is parsed JSON when the body is JSON, otherwise a string;
a body that is not text comes as base64 with `"body_encoding": "base64"`
beside it, and `saved_to` says where a `>> file` line wrote it. `errors`
appears only when something failed, as a list of messages.
`request.auth` names the auth type applied (`aws`, `oauth2` and so on) without
exposing credentials, and sensitive request headers are shown as `***`;
see [auth.md](auth.md). URL, body and captures are real values so an agent
can chain them, and `set-cookie` is masked in the response headers. When the
output is going into a stored log rather than to an agent, add `--redact`,
which masks the response body and headers as well as the request — so a
redacted run is for logs, not for chaining.

A session usually looks like this:

```console
$ apic list --json | jq -r '.requests[] | "\(.id)\t\(.method) \(.url)"'
login   POST {{baseUrl}}/auth/login
whoami  GET {{baseUrl}}/me

$ apic describe whoami --json | jq '{ready, missing: [.variables[] | select(.missing) | .name]}'
{"ready": false, "missing": ["token"]}

$ apic run login --json | jq '{ok, captured: .captures}'
{"ok": true, "captured": {"token": "mock-token"}}

$ apic run whoami --json | jq '{ok, status: .response.status, body: .response.body}'
{"ok": true, "status": 200, "body": {"email": "alice@example.com"}}
```

The agent never has to guess: `describe` says what is missing and which
request provides it, and the token persists, so `login` is run once rather
than before every call. When `whoami` declares `# @ref login`, even that
step goes away: `apic run whoami --json` prints `login`'s object and then
`whoami`'s, and `describe` reports `"ready": true` with
`"ref_runs": true` on the token.

## 2. MCP (Claude Code, Cursor, Windsurf, any MCP client)

`apic mcp` serves the project over stdio, or over HTTP for an agent that
is not on the same machine. Register it once, in the repository, so
every clone has it:

```sh
apic mcp install -C ./api --env dev        # writes .mcp.json, which Claude Code reads
apic mcp install --to .cursor/mcp.json     # Cursor reads the same shape from here
```

`apic init` writes the same `.mcp.json` into a new project. The file is
what you would write by hand, and any MCP client takes it:

```json
{"mcpServers": {"api": {"command": "apic", "args": ["mcp", "--dir", "api", "--env", "dev"]}}}
```

```sh
claude mcp add api -- apic mcp --dir ./api --env dev   # per machine instead
```

A client that installs servers from a catalogue finds apic in the
[MCP registry](https://registry.modelcontextprotocol.io/) as
`io.github.hungovercoders/apic`: the `ghcr.io/hungovercoders/apic` image
running `apic mcp` with the project mounted at `/work`.

Tools exposed:

| Tool | Purpose |
|---|---|
| `list_requests` | ids, methods, URL templates, descriptions, captures, asserts |
| `describe_request {name, env?}` | variables and sources, `ready` flag |
| `run_request {name, env?, vars?}` | send one request; returns the same JSON as `apic run --json`, with any `# @ref` dependencies nested under `ran_first` |
| `run_file {file, env?, vars?, keep_going?}` | run a file as a flow |
| `list_environments {env?}` | environments and effective variables (secrets masked) |
| `clear_session {env?, all?}` | forget captured values and cookies |
| `run_features {paths?, tags?, env?, vars?, use_session?}` | run Gherkin features; returns pass/fail counts and the failing steps. Scenarios are isolated unless `use_session` shares `.apic/session.json` with the other tools (see [testing.md](testing.md)) |
| `validate_project {}` | parse every `.http` file and report problems with file, line, column and code, without sending anything; the same shape as `apic validate --json`. For an agent that just edited a file |
| `curl_request {name, env?, vars?, raw?, lang?}` | the equivalent curl command (or with `lang`, HTTPie, PowerShell, Python, JavaScript or Go), `{"id", "lang", "command"}`, with credentials as shell placeholders and values masked unless `raw` is set; a missing variable is an error naming it, as for `apic curl` |

Each `.http` file is also exposed as a resource so the agent can read the
definitions. Only the project's own `.http` and `.rest` files can be read this
way: any other path is refused, so `http-client.private.env.json`, `.env` and
`.apic/session.json` are never served to the agent even though they sit inside
the project. The readable set is checked against the project on each read, so a
file added after the server started can be read (it is not listed until the
server restarts). Assertion failures return `ok: false` rather than a tool error;
transport and usage problems return an error the agent can act on: the text
is the message followed by `(E101 missing variable: <hint>)`, and the
structured content is the same `{"error": {…}}` object the CLI writes.

## 3. A skill (Claude Code, Codex, Cursor, any agent that loads skills)

The `AGENTS.md` paragraph above costs a few hundred tokens on every turn,
whether or not the task touches the API. A skill is the same briefing
loaded on demand: the agent sees one line describing when it applies and
reads the rest only when a task involves `.http` files or an API call.
[`skills/`](https://github.com/hungovercoders/apic/tree/main/skills) in
the repository holds a pack of three, in the
[Agent Skills](https://agentskills.io) format every major agent reads,
and the binary carries a copy:

| Skill | When the agent reads it |
|---|---|
| `apic` | A task touches `.http` files or calls an API: the workflow (validate, list, describe, run, chain), the `--json` shape and exit codes, the error codes to branch on, the rules for writing a request and where secrets belong, and when to use `apic test` or the MCP tools instead. `references/cheatsheet.md` is the [cheat sheet](cheatsheet.md) with absolute links, generated by `task skill` so the two cannot drift. |
| `api-design` | The user is building an API or an endpoint, writing or reviewing an OpenAPI document, or choosing routes, status codes or an error format: the design decisions, each proved by a request `apic run` sends or a mock `apic mock` serves, with a contract skeleton and a review checklist in its references. |
| `api-security` | The user asks whether their API is secure, adds authentication, or hardens an endpoint before release: the OWASP API Security Top 10 as checks, authentication, authorisation, input and resource limits, headers, secrets and logging, each written as an `.http` request or Gherkin scenario that fails while the control is missing, with a bank of ready checks and a release checklist in its references. |

```sh
apic skill install                          # the pack, into .claude/skills and .agents/skills of the project
apic skill install --user                   # the same under your home directory, once, for every project
apic skill install --skill apic             # one skill of it
apic skill                                  # print the apic skill, for an agent that can run commands
apic skill api-security                     # print another
npx skills add hungovercoders/apic          # the same files from the repository, for the agents the CLI finds
```

`apic init` installs the pack too, so a new project briefs its agents from
the start; commit the directories. `--to <dir>` picks another location, and
copying a directory by hand works wherever your agent reads skills from.
Use them with the MCP server or without: the `apic` skill tells the agent
to prefer the tools when they exist, and the other two tell it to prove
every design and security decision with a request the project keeps.
`apic skill --json` gained a `skills` array naming the pack; `name` and
`files` are unchanged.

## Errors

Every error has a stable code from the [error catalogue](errors.md), and
an agent should read the code rather than parse the message, whose
wording may improve between releases:

| Where | What |
|---|---|
| `apic … --json`, exit 2 or 3 | one `{"error": {"code", "title", "message", "hint", "exit", "url"}}` object on stderr, plus `suggestions` (the closest known names, best first) when an unknown request or environment is near one that exists; stdout carries only results |
| `apic run --json`, a request that could not be sent | its result has `"ok": false` and the same object as `error`, beside `errors` |
| MCP tool error | the object as structured content; the text ends with `(code title: hint)` |

The codes an agent meets most, and what to do:

| Code | Do |
|---|---|
| [E101](errors.md#e101) missing variable | the message names the request that captures it: run that, or pass `vars` |
| [E201](errors.md#e201) unknown request | retry with the first of `suggestions` when there are any; otherwise `list_requests` (or `apic list --json`) and pick an id |
| [E103](errors.md#e103) request file has errors | `validate_project` and fix what it reports |
| [E301](errors.md#e301)–[E303](errors.md#e303) network | report to the user; the server is down, slow or untrusted, and retrying the same thing will not help |

### Over HTTP

```sh
apic mcp --http 127.0.0.1:8765
APIC_MCP_TOKEN=$(openssl rand -hex 16) apic mcp --http 0.0.0.0:8765 --dir ./api
```

`--http` serves the MCP streamable HTTP transport instead of stdio, for a
hosted agent, a shared development box, or a container that also runs the
API under test. The same tools and resources are exposed. Bound to the
loopback interface it needs nothing else; bound anywhere else it refuses
to start without a bearer token (`--token`, or `APIC_MCP_TOKEN`), which
every client must send as `Authorization: Bearer <token>`. The server runs
the project's requests with the project's credentials, so that rule is
not optional. Point the client at `http://<host>:<port>/`:

```json
{"mcpServers": {"api": {"type": "http", "url": "http://127.0.0.1:8765/", "headers": {"Authorization": "Bearer <token>"}}}}
```

## Writing requests as an agent

The format is plain text, so an agent can add requests too. Keep to the
subset in [the format guide](format.md), give every request a `# @name`, add an
`# @assert status == 200` and run `apic validate` afterwards. `apic init`
writes a correct skeleton to start from, and the
[cheat sheet](cheatsheet.md) fits in a prompt.

When the task comes with an OpenAPI document and no running API,
`apic import openapi.yaml -o api` writes the requests and
`apic mock openapi.yaml` serves the document's declared responses on
localhost, so `apic run <id> -C api --var baseUrl=http://localhost:8090`
proves the requests match the contract before the backend exists; a
`Prefer: code=404` header line asks for a documented error response. It
proves nothing about the API itself, so point `baseUrl` at the real one
when there is one. See [`apic mock`](cli.md#apic-mock).

## Reading these docs as an agent

Every page here is also available as plain text, so an agent can read the
documentation without parsing HTML:

- [llms.txt](https://apic.sh/llms.txt) lists every page with a one-line
  summary, in reading order. Fetch it first and follow only the links the
  task needs.
- [llms-full.txt](https://apic.sh/llms-full.txt) is every page in one
  Markdown document, for an agent that would rather load the whole manual
  once.

Inside a project, `apic skill` prints the same briefing the Agent Skill
installs, and `apic <command> --help` is always current for the binary on
the machine, whichever docs version the agent read. `apic version --json`
names that binary's release, and the [changelog](changelog.md) says what
each release added or changed, so an agent can tell whether a command the
docs describe is in the binary it has.
