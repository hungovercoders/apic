# Lesson 6: authentication. The terminal parts of the episode.
# Record it with `task tapes` (scripts/tapes), which serves the demo API on
# port 8089, writes apic-demo and the files this lesson's runnable blocks
# create into a scratch directory, and runs vhs on every tape there;
# `task tapes:check` validates the syntax. Just this one:
#   go run ./scripts/tapes -only 06
# Only the demo-only parts: the real-provider auth is talked over slides.
Output docs/learn/tapes/06.gif
Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 720
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms

Type "apic run login -C apic-demo > /dev/null" Enter Sleep 1s
Type "apic run whoami -C apic-demo -v | head -3" Enter Sleep 3s
Type "clear" Enter
Type "apic run basic-auth -C apic-demo" Enter Sleep 4s
Type "apic curl basic-auth -C apic-demo" Enter Sleep 3s
Type "clear" Enter
Type "apic run api-key -C apic-demo -v | head -3" Enter Sleep 3s
Type "clear" Enter
Type "apic describe oauth2-token -C apic-demo" Enter Sleep 4s
Type "apic run oauth2-token -C apic-demo > /dev/null; apic session -C apic-demo" Enter Sleep 4s
Type "clear" Enter
Type "apic run me health -C apic-auth" Enter Sleep 4s
Type "apic describe me -C apic-auth | sed -n 5,6p" Enter Sleep 3s
Type "clear" Enter
Type "apic run me-exec -C apic-auth" Enter Sleep 4s
Type "clear" Enter
Type "AWS_ACCESS_KEY_ID=AKIAEXAMPLE AWS_SECRET_ACCESS_KEY=example apic curl signed -C apic-auth" Enter Sleep 5s
