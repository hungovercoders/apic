# CSP exceptions

The Content-Security-Policy apic.sh sends is the `/*` rule in
`website/public/_headers`, applied to every path; `plugins/security-headers.mjs`
fills in the inline-script hashes at build time. A page that needs to relax
it (a third-party embed, say) gets its own path rule there and an entry
below, and `task ss:hygiene:csp-exceptions` fails when the two disagree. The
DAST gate reads this file too: a ZAP CSP finding on a path listed here is
reported but does not block.

`style-src 'unsafe-inline'` is site-wide, not an exception: Starlight sets
CSS custom properties through style attributes, which hashes cannot cover,
and `.zap/rules.tsv` accepts that one alert by reference.

## Exceptions

None. Every path sends the baseline policy.
