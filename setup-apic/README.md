# setup-apic

A GitHub Action that installs a pinned [apic](https://apic.sh/) release,
verified against its published `checksums.txt`, caches it in the runner's
tool cache and adds it to `PATH`. Linux, macOS and Windows runners.

```yaml
- uses: hungovercoders/apic/setup-apic@v0
  with:
    version: v0.2.0 # or "latest", the default
- run: apic validate --format github
- run: apic run smoke.http --env ci
```

| Input | Default | Meaning |
|---|---|---|
| `version` | `latest` | The release tag to install. |
| `install-dir` | the tool cache | Where to put the binary, keyed by version and platform. |
| `token` | `github.token` | Resolves `latest` through the GitHub API without the unauthenticated rate limit. |

Outputs: `version`, the tag installed, and `path`, the directory holding
the binary. The `@v0` tag follows the newest 0.x release; pin a version
tag to hold one. [The cookbook](https://apic.sh/cookbook/#a-smoke-test-in-ci-that-never-leaks-secrets)
and [lesson 8](https://apic.sh/learn/08-ci/) show a whole workflow,
secrets included.

## For agents

An AI agent running in the job gets the same tool: every apic command
takes `--json`, exits 0, 1, 2 or 3 by outcome and never prompts, so the
agent can validate, list, describe and run the project's requests from
the shell. `apic skill` prints the briefing it should read first,
[apic.sh/agents](https://apic.sh/agents/) is the contract, and `apic mcp`
serves the project as MCP tools when the agent speaks MCP.
