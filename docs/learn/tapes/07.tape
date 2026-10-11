# Lesson 7: behaviour tests with Gherkin. The terminal parts of the episode.
# Record it with `task tapes` (scripts/tapes), which serves the demo API on
# port 8089, writes apic-demo and the files this lesson's runnable blocks
# create into a scratch directory, and runs vhs on every tape there;
# `task tapes:check` validates the syntax. Just this one:
#   go run ./scripts/tapes -only 07
Output docs/learn/tapes/07.gif
Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 720
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms

Type "apic test -C apic-demo --format progress" Enter Sleep 4s
Type "clear" Enter
Type "apic test --steps -C apic-demo | head -40" Enter Sleep 6s
Type "clear" Enter
Type "cat apic-demo/features/first.feature" Enter Sleep 3s
Type "apic test features/first.feature -C apic-demo" Enter Sleep 4s
Type "clear" Enter
Type "cat apic-demo/features/lesson7.feature" Enter Sleep 5s
Type "apic test features/lesson7.feature -C apic-demo | tail -3" Enter Sleep 4s
Type "clear" Enter
Type "apic test features/tagged.feature -C apic-demo --tags '@smoke && ~@slow' | tail -3" Enter Sleep 4s
Type "clear" Enter
Type "apic test features/undefined.feature -C apic-demo | tail -12" Enter Sleep 5s
Type "clear" Enter
Type "apic test -C apic-demo --format junit --output report.xml && head -2 report.xml" Enter Sleep 4s
Type "apic test -C apic-demo --format progress | grep scenarios" Enter Sleep 3s
