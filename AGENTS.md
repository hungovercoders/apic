# Working in this repository

apic is a Go CLI that runs `.http` request files for humans and AI agents.
This file carries what most tasks need; the routes at the end say when to
open anything else. `CLAUDE.md` is `@AGENTS.md`.

## Layout

`cmd/apic` is the entry point; `internal/httpfile` parses and formats
`.http` files; `internal/project` discovers and validates a project;
`internal/runner` resolves variables, applies auth, sends, captures and
asserts, with `env`, `template`, `session`, `auth`, `selector` and
`assert` beneath it; `internal/output` renders for the CLI, the UI and
MCP; `internal/lsp` is `apic lsp`, the language server; `internal/bdd` and
`internal/phrase` are `apic test`; `internal/cli` holds the commands (one
file per command) and `internal/ui` the terminal UI; `internal/demoapi` is
the offline API and project behind `apic demo`; `examples/` are the static
sample projects CI validates and format-checks; `editors/vscode/` is the
extension, released on its own `vscode-v*` tags; `setup-apic/` the GitHub
Action; `skills/apic/` the Agent Skill users install into their own
projects (`SKILL.md` is hand-written, `references/cheatsheet.md`
generated), embedded by `skills/embed.go` for `apic skill` and `apic init`;
`website/` is [apic.sh](https://apic.sh/), the user docs, with the course
under `src/content/docs/learn/`; `docs/` is reference for working on apic
rather than using it; and the generators are under `scripts/`.

Two things worth knowing: the OpenAPI and Postman readers are hand-written
walkers, not libraries, and the VS Code extension never parses `.http`
files itself (it reads `--json`).

## Commands

- `task build` / `go build -o bin/apic ./cmd/apic`
- `task test` / `go test ./...`: tests use `net/http/httptest`, no network needed
- `task lint`: gofmt, go vet, golangci-lint (config in `.golangci.yml`); `task fmt` actually formats the tree (`lint` only checks)
- `task race`, `task cover`, `task vuln`: the race detector, `coverage.out` with a per-package summary, govulncheck
- `task validate-examples`: the example projects, as CI checks them
- `task check`: what CI runs — lint, test, race, licences and validate-examples
- `task ss:hygiene:test`: the slopstopper hygiene suite, which the pre-push hook runs once `git config core.hooksPath .githooks` is set (CONTRIBUTING's setup does); `task --list` shows every `ss:*` check
- Generated files each have a task: `task docs:cli` (the site's `cli.md`), `task docs:errors` (its `errors.md`, from `runner.Catalogue`), `task skill` (`skills/apic/references/cheatsheet.md`) and `task schemas` have a test that fails when the output is stale; `task man`, `task shots`, `task tapes`, `task episodes` and `task notices` write what is not committed

## Conventions

- Keep the `.http` dialect compatible with VS Code REST Client and JetBrains: new features go in `# @directive` comments before the request line, never new syntax in the request itself. Document any addition in the site's `format.md`.
- The `--json` output shape and exit codes are a public contract; change them only with a note on the site's `agents.md` page.
- Every command must work non-interactively (no prompts) and respect `--json`. `apic ui` is the single, deliberate exception: it is interactive, has no `--json`, and exits 2 when stdout is not a terminal. `apic mcp` and `apic lsp` are servers that speak their own JSON protocol on stdout, so `--json` has nothing to change for them.
- A new error gets a code from the catalogue: `runner.Usage(runner.CodeX, msg)` or `usagef(CodeX, …)`, never a bare message; a new code needs an entry in `Catalogue` and a case in `internal/cli/errors_test.go`, which runs a real command for every code.
- New commands need a section in the site's `cli.md` and a line in its `cheatsheet.md`; a new or changed flag needs `task docs:cli`. A flag's usage text must not contain backticks: cobra reads the first backticked word as the value's name (a test checks).
- The UI is tested through `Update` and `View`, and driven headlessly by `Press`/`Resize` in `internal/ui/headless.go`; the screenshot generator uses the same entry points, so a screenshot cannot drift from what the UI draws.
- The UI has its own terminal layer rather than a TUI framework. Bubble Tea was tried and removed: its package `init` queries the terminal for its background colour with a five-second timeout, which every apic command would pay on terminals that do not answer. Keep anything with an init-time terminal query out of the binary.
- Keep the dependency list small: prefer a few hundred lines of code over a large SDK (the AWS signer and the OpenAPI reader are the precedents), written from the spec rather than copied from another project. Permissive licences only (MIT, BSD, Apache-2.0; the MPL-2.0 modules godog pulls in are the existing exception). Check the stripped binary size with `task build && ls -la bin/apic` when adding a dependency.
- Add a test next to any parser or runner change; parser cases go in `internal/httpfile/testdata/sample.http`.
- A new step in the vocabulary needs: its regex and shapes in `internal/phrase/builtin.go` (`Builtin`), a handler bound by name in `internal/bdd/steps.go`, a row in `bdd.Vocabulary`, a scenario in `bdd_test.go`, and the table in the site's `testing.md`.
- Prose is British English: Vale fails on `color` or `behavior` outside code spans, and codespell runs over the whole tree in CI. Keep `LICENSE` at the root: CI's `licences` job fails without it.

## Routes: read before you act

| When you are… | Do this |
| --- | --- |
| changing the runner, adding a directive, a step or a command, or touching `--json` or an exit code | Read [docs/architecture.md](docs/architecture.md): the package map, the request lifecycle and the three contracts every change is checked against |
| editing the site, the course, a lesson, the skill, a generated page or any prose check | Read [docs/docs-site.md](docs/docs-site.md): the Astro layout, the link contract the binary depends on, the generators and Vale, codespell and lychee |
| changing a workflow, a linter setting, adding a `//nolint`, or fixing a failing slopstopper check | Read [docs/ci.md](docs/ci.md): what CI runs, why each linter is on, and the quality suite's config |
| adding a dependency, or touching a licence or notices file | Read [docs/licensing.md](docs/licensing.md): the permissive-only rule and the notices generator |
| cutting a release, or changing the installers, the packages, the image, the cask or the extension's release | Read [docs/releasing.md](docs/releasing.md): signing, SBOMs, the archive names three installers depend on, and the moving `v0` tag |
| writing a lesson for the course | Read [docs/learn/_template.md](docs/learn/_template.md): the template every lesson is copied from and the rules it follows |
| cutting a lesson's YouTube episode | Read [docs/learn/_episode-template.md](docs/learn/_episode-template.md): the title, thumbnail and description conventions |

For any task not covered above, read [docs/README.md](docs/README.md) for the routing table for every doc in this repo.
