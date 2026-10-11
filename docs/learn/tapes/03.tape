# Lesson 3: capture, the session and flows. The terminal parts of the episode.
# Record it with `task tapes` (scripts/tapes), which serves the demo API on
# port 8089, writes apic-demo and the files this lesson's runnable blocks
# create into a scratch directory, and runs vhs on every tape there;
# `task tapes:check` validates the syntax. Just this one:
#   go run ./scripts/tapes -only 03
Output docs/learn/tapes/03.gif
Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 720
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms

Type "apic session clear -C apic-demo" Enter
Type "apic run list-todos -C apic-demo" Enter Sleep 4s
Type "clear" Enter
Type "apic run login -C apic-demo" Enter Sleep 4s
Type "apic session -C apic-demo" Enter Sleep 3s
Type "cat apic-demo/.apic/session.json" Enter Sleep 3s
Type "clear" Enter
Type "apic run todos.http -C apic-demo" Enter Sleep 6s
Type "clear" Enter
Type "apic run todos.http -C apic-demo --keep-going | tail -15" Enter Sleep 5s
Type "clear" Enter
Type "apic session clear -C apic-demo" Enter
Type "apic run whoami -C apic-demo" Enter Sleep 5s
Type "clear" Enter
Type "apic session -C apic-demo --json | jq .local.token" Enter Sleep 3s
