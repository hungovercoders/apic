---
template: home.html
title: apic is epic
hide:
  - navigation
  - toc
---

<!-- The landing page is overrides/home.html; this markdown renders inside
     its "30 seconds to epic" section, so the install tabs keep the site's
     tabbed code blocks. -->

=== "Homebrew"

    ```sh
    brew install hungovercoders/tap/apic
    apic ui --demo
    ```

=== "Go"

    ```sh
    go install github.com/hungovercoders/apic/cmd/apic@latest
    apic ui --demo
    ```

=== "Linux / macOS"

    ```sh
    curl -fsSL https://raw.githubusercontent.com/hungovercoders/apic/main/install.sh | sh
    apic ui --demo
    ```

=== "Windows"

    ```powershell
    irm https://raw.githubusercontent.com/hungovercoders/apic/main/install.ps1 | iex
    apic ui --demo
    ```

`--demo` serves a small fake API inside the same process and opens the
[terminal UI](tui.md) on an example project that targets it. Press
++enter++ to send the request under the cursor, ++f++ to run a whole file
as a flow, ++e++ to switch environment and ++question++ for every key.
There is nothing to sign up for or clone, and nothing is left behind when
you quit.

!!! tip "Prefer the plain CLI?"
    `apic demo` writes the same example project to `./apic-demo` and serves
    the API, so you can drive it by hand:
    `apic run login whoami -C apic-demo`.
