# Changelog

What changed for people using apic, release by release. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the versions [Semantic Versioning](https://semver.org/): while apic is 0.x, a minor bump adds a command, a directive, a flag or an MCP tool, and a patch bump fixes something without changing what the binary accepts. Every release's notes on GitHub are its section here, so a change a user would notice gets a line under Unreleased in the pull request that makes it.

## [Unreleased]

### Added

- The Agent Skill is now a pack of three. `apic` briefs an agent on using apic as before; `api-design` on designing the HTTP API the requests call (resources and URLs, methods and status codes, problem-details errors, pagination, versioning, idempotency, caching, long-running work and the OpenAPI document, each decision proved by a request or an `apic mock`); and `api-security` on keeping it safe and showing it (the OWASP API Security Top 10 as checks, authentication, authorisation, input and resource limits, headers and CORS, secrets and logging, each a request or Gherkin scenario that fails while the control is missing). `apic skill <name>` prints one. The `apic` skill is about using apic, so `apic skill install` and `apic init` write it as before; the other two are opinions about your API, so `apic skill install --all` adds them (or `--skill <name>` picks exactly the skills wanted), `--user` writes the chosen skills under your home directory once for every project, and `apic skill --json` gained a `skills` array naming the pack. Each skill has evals under `skills/<name>/evals/` that `task agent:eval` runs with and without it.
- This changelog, on [apic.sh/changelog](https://apic.sh/changelog/) and attached to each release on GitHub as its notes, in place of a list of commits.

## [0.2.0] - 2026-10-27

### Added

- `apic mock <spec>` serves an OpenAPI 3 document as a stateless mock API on localhost, answering from the document's declared responses and examples, so the requests `apic import` wrote can be proved against the contract before the API exists. A `Prefer: code=404` header line asks for another documented response. It proves nothing about the real API: point `baseUrl` at that one when there is one.
- `apic mcp install` registers the MCP server in the project's `.mcp.json`, the project-scoped file Claude Code reads from the directory it starts in and the other clients accept in the same shape, with `--dir` and `--env` filled in from `-C` and `--env`. It merges with what the file holds, reports an identical entry as unchanged, refuses a file that is not a JSON object rather than overwriting it, and `--to .cursor/mcp.json` picks Cursor's own file.
- `apic init` writes the same `.mcp.json` into a new project, beside the Agent Skill; `--no-mcp` leaves it out, and an entry a team has changed is kept unless `--force`.
- Each release publishes apic to the [MCP registry](https://registry.modelcontextprotocol.io/) as `io.github.hungovercoders/apic`, the `ghcr.io/hungovercoders/apic` image running `apic mcp` with the project mounted at `/work`, so an MCP client that installs servers from a catalogue can find it.
- The `setup-apic` GitHub Action has a README, with a paragraph for an agent running in the job.

### Changed

- A plain (not `--json`) error written to a pipe or a log ends with one more line saying `--json` prints it as one object to branch on, since the reader there is more often a script or an agent than a person. A terminal never shows the line, and `apic ui`, which refuses `--json`, never prints it.

## [0.1.4] - 2026-10-25

### Added

- E201 unknown request and E204 unknown environment suggest the closest known names, in the message and as `suggestions` in the `--json` error object, and every error shows the catalogue's hint under it.
- `apic init` and `apic demo` take an empty directory as the target, and every command's `--help` has examples.
- In the terminal UI, `E` steps to the previous environment, and the arrow keys move through the list while a filter is being typed.
- The licence and third-party notices ship in every archive, package and the image.

### Changed

- The user docs moved to [apic.sh](https://apic.sh/), with the course, `llms.txt` and `llms-full.txt` for agents, and a privacy page.

### Fixed

- Ctrl-C cancels a run cleanly, with exit 3 and E305, instead of leaving a half-written session.
- The terminal UI stops reading the keyboard while an editor it opened runs, and passes `--wait` to VS Code; four rendering fixes, and VT output on the Windows console; keys the filter prompt ignores no longer reset the cursor.
- `apic list` lines up its columns across the files of a project and never pads the end of a line.

## [0.1.1] to [0.1.3] - 2026-10-10

The release pipeline only: the Homebrew cask's tap token reached goreleaser. The binary did not change.

## [0.1.0] - 2026-10-10

The first release: `.http` files run from the terminal, CI or an AI agent, with one static binary for Linux, macOS and Windows.

- `apic run` with environments from `http-client.env.json`, variables from flags, the shell, the private env file and `.env`, captured values that persist in `.apic/session.json`, `# @ref` dependencies, assertions, retries, polling, redirects that drop credentials and a per-host TLS setting; a file runs in order as a flow.
- `--json` output with a stable shape and exit codes 0, 1, 2 and 3, an error catalogue with a code, a hint and a URL for every error, and `--dry-run`, `--body-only`, `--body-limit` and `--redact`.
- Authentication: bearer, basic, AWS SigV4 from apic's own signer, OAuth2 client credentials, device code and authorisation code, and a command's output.
- `apic test` runs Gherkin features against the project's requests with a built-in vocabulary and `# @step` phrases.
- `apic list`, `describe`, `env`, `session`, `history`, `history diff`, `select`, `validate`, `fmt`, `curl`, `snippet` and `import` from OpenAPI 3, Postman collections and curl commands.
- `apic mcp`, an MCP server over stdio or HTTP, and `apic lsp`, a language server for any editor; the VS Code extension drives the binary.
- `apic ui`, a terminal UI with its own terminal layer, and `apic demo`, an offline API and project to try everything on.
- `apic skill` and `apic init` carry the Agent Skill that briefs an AI agent on all of the above.

[Unreleased]: https://github.com/hungovercoders/apic/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/hungovercoders/apic/compare/v0.1.4...v0.2.0
[0.1.4]: https://github.com/hungovercoders/apic/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/hungovercoders/apic/compare/v0.1.2...v0.1.3
[0.1.1]: https://github.com/hungovercoders/apic/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/hungovercoders/apic/releases/tag/v0.1.0
