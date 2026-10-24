---
title: "Privacy"
description: "What apic.sh collects (nothing of its own), what Cloudflare keeps as the host, which other services the pages link to, and what the apic CLI sends."
---
apic handles API credentials, so it is worth being plain about what this site
does and does not know about you.

## What this site collects

Nothing of its own. There are no accounts, forms, cookies or analytics
scripts. The site is a set of static pages whose only scripts are its own:
the theme picker, the sidebar and the search, which runs entirely in your
browser from an index built with the site. The site's
[Content-Security-Policy](https://github.com/hungovercoders/apic/blob/main/website/public/_headers)
allows scripts from the site itself and a hash of each of those inline
scripts, and lets the page connect only to its own origin, so no third-party
script or beacon can run or phone home. Fonts are served from this site, not
from a font service.

## Hosting

The pages are served by [Cloudflare](https://www.cloudflare.com/). Like any
host, Cloudflare processes your IP address and browser details to deliver
each page. It keeps aggregate traffic statistics (how many requests, from
which countries, with which status codes) and short-lived per-request logs
that the maintainer uses to keep the site working. That is what "analytics"
means for this site: there is nothing else behind it. Cloudflare's handling
of that data is described in its
[privacy policy](https://www.cloudflare.com/privacypolicy/). Nothing from
it is combined with anything else or used to identify you.

## Other services the pages link to

The pages link out, but they do not load anything from anywhere else.

- **GitHub.** The repository, the "edit this page" links, the releases and
  the install script on `raw.githubusercontent.com` that the install
  instructions pipe to `sh`. GitHub has its
  [own privacy statement](https://docs.github.com/en/site-policy/privacy-policies/github-general-privacy-statement).
- **Editor marketplaces.** The [editors](editors.md) page links to the
  Visual Studio Marketplace and Open VSX for the extension.

## The apic CLI

`apic` sends requests only where your project tells it to: the APIs your
`.http` files name, and the token endpoints an [`@auth`](auth.md) directive
or the `auth:` section of `apic.yaml` names, which receive the credentials
that directive configures. It has no telemetry, no update check and no
crash reporting. Error messages, the
`--json` output and SARIF reports carry links to this site, but the binary
never fetches them; the one request a project can cause here is your editor
fetching the `apic.yaml` schema that `apic init` points it at, which
Cloudflare sees like any other page view. How the CLI stores sessions,
cookies and history on your own machine, and how it masks credentials in
output, is in the
[security policy](https://github.com/hungovercoders/apic/blob/main/SECURITY.md).

## Questions

Open an [issue](https://github.com/hungovercoders/apic/issues). This page
was last checked against the site's configuration on 24 October 2026.
