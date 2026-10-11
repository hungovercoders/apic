# Lesson 1: install apic and send your first request. The terminal parts of the episode.
# Record it with `task tapes` (scripts/tapes), which serves the demo API on
# port 8089, writes apic-demo and the files this lesson's runnable blocks
# create into a scratch directory, and runs vhs on every tape there;
# `task tapes:check` validates the syntax. Just this one:
#   go run ./scripts/tapes -only 01
Output docs/learn/tapes/01.gif
Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 720
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms

Type "apic version" Enter Sleep 2s
Type "clear" Enter
Type "apic list -C apic-demo" Enter Sleep 4s
Type "clear" Enter
Type "apic describe login -C apic-demo" Enter Sleep 4s
Type "clear" Enter
Type "apic run login -C apic-demo" Enter Sleep 4s
Type "clear" Enter
Type "apic run not-found -C apic-demo; echo exit $?" Enter Sleep 4s
Type "clear" Enter
Type "apic run login -C apic-demo --json | jq .ok" Enter Sleep 3s
