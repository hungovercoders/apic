---
title: "Licence"
description: "apic is MIT licensed; what that lets you do, and the licences of the modules it ships with."
---
apic is released under the **MIT Licence**. The full text lives in
[`LICENSE`](https://github.com/hungovercoders/apic/blob/main/LICENSE) in the
repository, and a copy ships inside every release archive and package and in
the container image.

In short: use it, change it, ship it inside your own products, commercial or
not. Keep the copyright notice with any substantial copy of the source, and
understand that it comes with no warranty.

## Third-party code

apic is a single static binary, so the Go standard library and runtime and the
open source modules it is built from travel inside it. Each keeps its own
licence. All of them are permissive except three MPL-2.0 modules, whose
copyleft is file-level:

| Licence | What |
|---|---|
| MIT | most modules, including godog, gjson, lipgloss and termenv |
| BSD-3-Clause | Go itself, `golang.org/x/...`, `github.com/google/uuid`, `spf13/pflag`, `yosida95/uritemplate` |
| Apache-2.0 | `spf13/cobra` and `inconshreveable/mousetrap` (cobra's, in the Windows builds) |
| MIT and Apache-2.0 | `gopkg.in/yaml.v3` (the parts ported from libyaml are MIT, the rest Apache-2.0) and `modelcontextprotocol/go-sdk` (moving from MIT to Apache-2.0, so its code is under one or the other) |
| MPL-2.0 | the HashiCorp modules godog pulls in (`go-memdb`, `go-immutable-radix`, `golang-lru`) |

Nothing here is copyleft in a way that reaches your use of apic or your own
code: MPL-2.0 applies only to those modules' own files, which apic does not
modify. It does ask that anyone who ships them in a binary says where their
source is, which the notices file does for every module.

`THIRD_PARTY_NOTICES.md` reproduces each licence in full, along with the
`NOTICE` and `PATENTS` files some modules ship, and links each module's exact
source on the Go module proxy. Every release archive and package contains it,
and the container image has it in `/usr/share/doc/apic` beside `LICENSE` and
`CA-CERTIFICATES.md`. The image's CA bundle is Mozilla's certificate store
(MPL-2.0) as Alpine packages it, and that file names the package and its
source. Regenerate the notices from a checkout at any time:

```sh
task notices
```

The generator walks the module set for **every** released platform, so the
notices are complete whichever archive you downloaded.

The VS Code extension is MIT too. It bundles a handful of npm packages (the
language client and what it pulls in, under MIT, ISC and BlueOak-1.0.0), and
every `.vsix` carries their notices.
