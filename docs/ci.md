# CI and linting

What the workflows run, why each linter is on, and how the slopstopper
quality suite fits beside them. `task check` is the local equivalent of
`ci.yml`; `task ss:hygiene:test` is what the pre-push hook runs.

## The Go linters

- `.golangci.yml` runs the standard set plus `bodyclose`, `copyloopvar`,
  `errorlint`, `gosec`, `misspell` and `unparam`. Each was measured against the
  tree before being enabled, so the config is quiet: a new finding means
  something. Deliberately left off: `nilerr` (its hits in `cli/test.go` are
  correct), `predeclared` and `usestdlibvars` (pure style).
- A `//nolint:gosec` needs a reason on the same line saying why the call is
  safe. The existing ones mark deliberate decisions — reading the file the user
  named, scaffolding a project directory the user then edits, `$randomInt` not
  being a nonce — and are the place to look before adding another.
- `errorlint` is on because exit codes are a public contract:
  `runner.ExitCode` uses `errors.As`, so wrapping a `TransportError` cannot
  silently turn a documented 3 into a 2. There is a test for exactly that.

## The workflows

- `ci.yml` runs the tests on Linux, macOS and Windows (the race detector on
  Linux only), the lint job (gofmt, go vet, golangci-lint and `codespell` over
  the whole tree), govulncheck, the licences check and the example projects.
  A coverage job uploads `coverage.out` as an artifact.
- `docs.yml` runs codespell, Vale (errors only: British spelling, project
  terms, repeated words) and, on pull requests, lychee for external links,
  then builds the site to prove it; see `docs-site.md`.
- CodeQL runs weekly and per pull request. Dependabot watches `gomod` and
  `github-actions` weekly.
- Workflows declare `permissions: contents: read` at the top and widen only
  where a job needs it (`docs.yml` for Pages, `codeql.yml` for
  security-events, `release.yml` for the release upload).

## The slopstopper quality suite

The `ss-*.yml` workflows, the `task ss:*` shims in `Taskfile.ss.yml`, the
composite actions under `.github/actions/ss-*/` and the two
`.claude/skills/slopstopper-*` playbooks come from
[slopstopper](https://github.com/hungovercoders/slopstopper) and are
refreshed wholesale by its installer, so customise `.slopstopper.yml`, never
those files. `mise.toml` pins the CLI, `task` and Node for CI and local runs
alike; `.ss/.workflows-installed` records which workflows the installer put
in place, so a deliberately deleted one does not come back.

- `.slopstopper.yml` points the browser checks at apic.sh and sets the
  hygiene knobs. The site builds from `website/`, so the root
  `package.json` has a `build` script that installs and builds it, and
  `mise.toml` exports `SERVE_ROOT=website/dist` and `SS_SERVER_HEADERS` for
  the bundled static server the browser checks and DAST run against on pull
  requests, so it sends the same headers as Cloudflare.
- The URL-driven checks also run on every `deployment_status` event, and the
  only deployments this repo raises are `docs.yml`'s GitHub Pages deploys of
  the legacy redirect stubs. That job declares no environment `url`, so the
  status carries only the Actions log link and `ss-resolve-url` audits
  `urls.production` instead; with a `url` the checks audited
  `hungovercoders.github.io/apic/` and failed on every docs deploy.
- `.github/dependency-review-config.yml` is the licence allowlist the
  vulnerability check on new dependencies reads; the installer seeds it and
  never overwrites it, so it is the place to accept a licence the default
  list leaves out.
- The entry-file budgets are the reason `AGENTS.md` routes to this directory
  instead of carrying everything: `AGENTS.md` stays under about 2,000
  tokens, `README.md` under 600 (badges excluded) and every doc under `docs/`
  has a trigger-first route from `AGENTS.md` or the map in `docs/README.md`.
- The complexity check is lizard, with the CCN ceiling in `.slopstopper.yml`
  at 30, golangci-lint's `gocyclo` default. lizard counts every `case`, so a
  switch-driven parser reads high: split it into a handler table or one
  function per section rather than raising the ceiling, which only ever
  comes down.
- Before diagnosing a red check, read
  `.claude/skills/slopstopper-triage/SKILL.md`, which maps each workflow to
  the local task, its report under `.ss/reports/` and where the fix lives.
