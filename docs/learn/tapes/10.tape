# Lesson 10: apic for AI agents. The terminal parts of the episode (steps 1
# to 3 and 5 to 7); the agent session is prerecorded in Claude Code.
# Record it with `task tapes` (scripts/tapes), which serves the demo API on
# port 8089, writes apic-demo and the files this lesson's runnable blocks
# create into a scratch directory, and runs vhs on every tape there;
# `task tapes:check` validates the syntax. Just this one:
#   go run ./scripts/tapes -only 10
Output docs/learn/tapes/10.gif
Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 720
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms

Type `apic list -C apic-demo --json | jq -r '.requests[] | "\(.id)\t\(.method) \(.url)"' | head -5` Enter Sleep 4s
Type "clear" Enter
Type "apic session clear -C apic-demo" Enter
Type "apic describe get-todo -C apic-demo --json | jq -c '{ready, missing: [.variables[] | select(.missing) | {name, captured_by}]}'" Enter Sleep 4s
Type "apic run get-todo -C apic-demo --json 2>/dev/null | jq -c '{ok, errors}'; apic run get-todo -C apic-demo --json >/dev/null 2>&1 || echo exit $?" Enter Sleep 4s
Type "clear" Enter
Type "apic run login -C apic-demo --json | jq -c '{ok, captures}'" Enter Sleep 3s
Type "apic run create-todo -C apic-demo --json --var title='From an agent' | jq -c '{ok, status: .response.status, captures}'" Enter Sleep 3s
Type "apic run get-todo -C apic-demo --json | jq -c '{ok, body: .response.body}'" Enter Sleep 4s
Type "clear" Enter
Type "apic session clear -C apic-demo" Enter
Type "apic describe whoami -C apic-demo --json | jq -c '{ready, refs}'" Enter Sleep 3s
Type "apic run whoami -C apic-demo --json | jq -c '{name: .request.name, ok, status: .response.status}'" Enter Sleep 4s
Type "clear" Enter
Type `(printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"lesson","version":"0"}}}' '{"jsonrpc":"2.0","method":"notifications/initialized"}' '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'; sleep 1) | apic mcp --dir apic-demo 2>/dev/null | jq -r 'select(.id == 2) | .result.tools[].name'` Enter Sleep 5s
Type "clear" Enter
Type "mkdir -p agent-draft && printf '### Health\n# @name health\n# @assert stauts == 200\nGET http://localhost:8089/health\n' > agent-draft/health.http" Enter
Type "apic validate -C agent-draft --json | jq -c '.diagnostics[] | {path, line, column, code, message}'" Enter Sleep 4s
Type "apic test --steps -C apic-demo --json | jq -r '.[].pattern' | head -4" Enter Sleep 4s
