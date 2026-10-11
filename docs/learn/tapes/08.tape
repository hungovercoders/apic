# Lesson 8: apic in CI. The terminal parts of the episode (steps 2 to 4);
# the workflow file and the Actions run are prerecorded.
# Record it with `task tapes` (scripts/tapes), which serves the demo API on
# port 8089, writes apic-demo and the files this lesson's runnable blocks
# create into a scratch directory, and runs vhs on every tape there;
# `task tapes:check` validates the syntax. Just this one:
#   go run ./scripts/tapes -only 08
Output docs/learn/tapes/08.gif
Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 720
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms

Type "apic validate -C apic-demo --format github" Enter Sleep 3s
Type "printf '### dup\n# @name health\n# @assert stauts == 200\nGET {{baseUrl}}/health\n' > apic-demo/broken.http" Enter
Type "apic validate -C apic-demo --format github; echo exit $?" Enter Sleep 5s
Type "rm apic-demo/broken.http" Enter
Type "clear" Enter
Type "APIC_VAR_password=s3cret apic run login whoami -C apic-demo --json --redact" Enter Sleep 5s
Type "clear" Enter
Type "APIC_VAR_password=wrong apic run login -C apic-demo --json --redact || echo exit $?" Enter Sleep 4s
Type "clear" Enter
Type "apic test -C apic-demo --format junit --output report.xml --redact && head -3 report.xml" Enter Sleep 5s
