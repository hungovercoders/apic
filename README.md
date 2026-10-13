<h1 align="center">
  <img src="website/src/assets/logo.svg" width="72" alt=""><br>
  apic
</h1>

<p align="center"><strong>apic is epic</strong>: run API requests from plain <code>.http</code> files, in the terminal, in CI, or from an AI agent, on any platform, with one static binary.</p>

<div align="center">

[![CI](https://github.com/hungovercoders/apic/actions/workflows/ci.yml/badge.svg)](https://github.com/hungovercoders/apic/actions/workflows/ci.yml) [![Docs](https://github.com/hungovercoders/apic/actions/workflows/docs.yml/badge.svg)](https://apic.sh/) [![Latest release](https://img.shields.io/github/v/release/hungovercoders/apic?color=7c5cff&label=release)](https://github.com/hungovercoders/apic/releases/latest) ![Go 1.25+](https://img.shields.io/badge/go-1.25%2B-00ADD8) [![MIT licence](https://img.shields.io/badge/licence-MIT-7c5cff)](LICENSE)

</div>

<p align="center"><img src="website/public/assets/apic-demo.svg" width="820" alt="apic ui --demo"></p>

```sh
apic demo                           # terminal 1: a fake API and a project for it in ./apic-demo
cd apic-demo                        # terminal 2
apic run login                      # POST, capture the token
apic run whoami                     # reuse the token, check assertions
apic run auth.http --json | jq      # a file as a flow, one JSON line per request
apic test                           # Gherkin features against the same requests
claude mcp add api -- apic mcp      # an agent calls the same requests as tools
apic ui --demo                      # or just the UI on the fake API, no project
```

## Install

```sh
brew install hungovercoders/tap/apic                              # macOS / Linux
go install github.com/hungovercoders/apic/cmd/apic@latest         # Go 1.25+
curl -fsSL https://raw.githubusercontent.com/hungovercoders/apic/main/install.sh | sh
irm https://raw.githubusercontent.com/hungovercoders/apic/main/install.ps1 | iex   # Windows
```

Packages, the Docker image and the GitHub Action:
[getting started](https://apic.sh/getting-started/).

## Documentation

Everything is on **[apic.sh](https://apic.sh/)**: [getting started](https://apic.sh/getting-started/),
[the course](https://apic.sh/learn/), the [CLI reference](https://apic.sh/cli/),
the [`.http` format](https://apic.sh/format/) and [authentication](https://apic.sh/auth/).

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) first, [AGENTS.md](AGENTS.md) if you
are an AI agent, and [docs/README.md](docs/README.md) for the routing table for
every doc in this repo. Report a credential that escapes privately
([SECURITY.md](SECURITY.md)).

## Pipeline status

[![slopstopper](https://img.shields.io/badge/quality-slopstopper-2c7be5?style=flat-square)](https://slopstopper.dev/)

### 🔒 Security
[![DAST](https://github.com/hungovercoders/apic/actions/workflows/ss-security-dast-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-security-dast-check.yml)
[![SAST](https://github.com/hungovercoders/apic/actions/workflows/ss-security-sast-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-security-sast-check.yml)
[![Secrets](https://github.com/hungovercoders/apic/actions/workflows/ss-security-secrets-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-security-secrets-check.yml)
[![Dependency CVEs](https://github.com/hungovercoders/apic/actions/workflows/ss-security-vulnerability-all-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-security-vulnerability-all-check.yml)
[![Dependency Review](https://github.com/hungovercoders/apic/actions/workflows/ss-security-vulnerability-new-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-security-vulnerability-new-check.yml)

### 🧹 Hygiene
[![Auto-label PRs](https://github.com/hungovercoders/apic/actions/workflows/ss-hygiene-auto-label-pr.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-hygiene-auto-label-pr.yml)
[![Complexity](https://github.com/hungovercoders/apic/actions/workflows/ss-hygiene-complexity-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-hygiene-complexity-check.yml)
[![CSP Exceptions](https://github.com/hungovercoders/apic/actions/workflows/ss-hygiene-csp-exceptions-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-hygiene-csp-exceptions-check.yml)
[![Docs Accuracy](https://github.com/hungovercoders/apic/actions/workflows/ss-hygiene-docs-accuracy-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-hygiene-docs-accuracy-check.yml)
[![Docs Size](https://github.com/hungovercoders/apic/actions/workflows/ss-hygiene-docs-size-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-hygiene-docs-size-check.yml)
[![Docs Structure](https://github.com/hungovercoders/apic/actions/workflows/ss-hygiene-docs-structure-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-hygiene-docs-structure-check.yml)
[![Entry Files](https://github.com/hungovercoders/apic/actions/workflows/ss-hygiene-entry-files-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-hygiene-entry-files-check.yml)

### ✅ Reliability
[![Accessibility](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-accessibility-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-accessibility-check.yml)
[![Broken Links](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-broken-links-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-broken-links-check.yml)
[![Core Web Vitals](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-core-web-vitals.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-core-web-vitals.yml)
[![E2E](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-e2e-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-e2e-check.yml)
[![llms.txt](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-llms-txt-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-llms-txt-check.yml)
[![robots.txt](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-robots-txt-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-robots-txt-check.yml)
[![SEO](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-seo-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-seo-check.yml)
[![Sitemap](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-sitemap-check.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-sitemap-check.yml)
[![Smoke](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-smoke-tests.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-reliability-smoke-tests.yml)

### 🤖 Operational
[![Deployed Checks](https://github.com/hungovercoders/apic/actions/workflows/ss-cloudflare-deployed.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-cloudflare-deployed.yml)
[![Workflow Failures](https://github.com/hungovercoders/apic/actions/workflows/ss-workflow-failure-issue.yml/badge.svg?branch=main)](https://github.com/hungovercoders/apic/actions/workflows/ss-workflow-failure-issue.yml)

## Licence

[MIT](LICENSE); every release archive ships the notices for the modules the
binary links.
