# Lesson 2: variables and environments. The terminal parts of the episode.
# Record it with `task tapes` (scripts/tapes), which serves the demo API on
# port 8089, writes apic-demo and the files this lesson's runnable blocks
# create into a scratch directory, and runs vhs on every tape there;
# `task tapes:check` validates the syntax. Just this one:
#   go run ./scripts/tapes -only 02
Output docs/learn/tapes/02.gif
Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 720
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms

Type "apic describe login -C apic-demo" Enter Sleep 4s
Type "clear" Enter
Type "cat apic-demo/http-client.env.json" Enter Sleep 3s
Type "apic env -C apic-demo" Enter Sleep 4s
Type "clear" Enter
Type "apic describe login -C apic-demo --env staging" Enter Sleep 4s
Type "clear" Enter
Type "echo user=dave > apic-demo/.env" Enter
Type "apic describe login -C apic-demo | grep user" Enter Sleep 3s
Type "apic describe login -C apic-demo --var user=erin | grep user" Enter Sleep 3s
Type "APIC_VAR_user=frank apic describe login -C apic-demo | grep user" Enter Sleep 3s
Type "rm apic-demo/.env" Enter
Type "clear" Enter
Type `apic describe whoami -C apic-demo --var baseUrl=http://example.com --json | jq '.variables[] | select(.name=="baseUrl") | .source'` Enter Sleep 4s
