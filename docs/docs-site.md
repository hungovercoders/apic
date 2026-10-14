# The docs site, the course and the generated pages

`website/` is [apic.sh](https://apic.sh/), the user documentation. This page
is the layout, the link contract the binary depends on, the generators and
the prose checks. The site's content is not reference for working on apic;
`docs/` is.

## Layout

- Astro with Starlight, pages in `src/content/docs/` (Markdown;
  `getting-started.mdx` and `index.mdx` are MDX for the tab components), the
  sidebar in `src/sidebar.mjs` (which `astro.config.mjs`, `llms.txt` and
  `llms-full.txt` all read), apic's colours in `src/styles/custom.css`.
  Every page needs a `title:` and a one-line `description:` in its front
  matter, not an H1. Asides are
  `:::tip[Title]` … `:::`; collapsible blocks are `<details>` with a
  `<summary>`. A lesson stays Markdown (`<!-- learn -->` is an HTML comment,
  which MDX rejects and `scripts/learncheck` needs).
- The course is under `src/content/docs/learn/`, and the migration guides
  under `migrate/`, whose runnable blocks `task learn:check` runs too. A
  lesson is written from `learn/_template.md` in this directory, its episode
  from `learn/_episode-template.md`.
- Pages link to each other as `format.md#anchor`, the way GitHub renders
  them, and to files under `public/` by their path from the content root
  (`assets/apic-ui.svg`); `plugins/remark-doc-links.mjs` turns both into the
  URLs the site serves. `scripts/skilldocs` relies on the `.md` form, so keep
  writing links that way.
- The page paths and heading anchors are a public contract: the binary
  (`runner.DocsURL`, the SARIF `helpUri`, the LSP code description, the
  `apic init` schema modeline), the VS Code extension, the schema `$id`s and
  the Homebrew cask link to them. Renaming a page or a heading needs the same
  care as renaming a flag. `starlight-links-validator` fails the build on a
  broken internal link or anchor.
- The schemas under `public/schemas/` are generated (`task schemas`);
  `public/assets/` holds the generated screenshots (`task shots`);
  `src/assets/logo.svg` is the logo the README and the site share.
- `src/routeData.ts` adds the share tags Starlight does not write
  (`og:image`, the Twitter title, description and image) to every page;
  `public/og-image.png` is the 1200×630 image they point at.
  `public/_headers` is the template for the response headers Cloudflare
  sends (a Content-Security-Policy, frame and sniffing protection);
  `plugins/security-headers.mjs` writes the built copy with a hash of every
  inline script, so the policy allows Starlight's own scripts and nothing else. `robots.txt`,
  `llms.txt` and `llms-full.txt` (every page as one Markdown document) are
  generated under `src/pages/` from the site URL and the docs collection,
  so none can drift from the pages; every page's `description:` is its
  summary in `llms.txt` and its meta description, so a new page needs one. slopstopper's
  reliability checks assert all of this against the built site; see `ci.md`.

## Deploys

Cloudflare Workers Builds watches the repository and runs `npx wrangler
deploy` in `website/` (`wrangler.jsonc`, a static-assets Worker whose
`build.command` runs `npm run build` first); `main` is apic.sh and every pull
request gets a preview URL. `docs.yml` builds the site on each change to
prove it, and keeps GitHub Pages (`hungovercoders.github.io/apic/`, where the
docs lived before apic.sh) serving the schemas as real files and a redirect
page per route, because the 0.1.x binaries and the `apic.yaml` files they
wrote link there (`website/scripts/legacy.mjs`).

## Commands

- `task docs:install`, then `task docs` / `task docs:build`: install the
  site's npm packages (Node 22+), serve apic.sh locally, or build it into
  `website/dist`; the build fails on a broken internal link or anchor.
- `task docs:check`: codespell, Vale and lychee, as the docs workflow runs
  them (needs the three on PATH). Prose is British English: Vale's
  `Apic.British` rule fails on `color` or `behavior` outside code spans. A
  deliberate typo in an example goes in `.codespellrc`'s
  `ignore-words-list`; a name Vale should hold to one spelling goes in
  `docs/.vale/styles/config/vocabularies/Apic/accept.txt`.
- `task docs:cli`: regenerate the "Commands and flags" section at the end of
  the site's `cli.md` from the command tree (`scripts/clidocs`; a test fails
  when it is stale); `task man` writes the man pages goreleaser ships.
- `task docs:errors`: regenerate the site's `errors.md` from
  `runner.Catalogue` (`scripts/errdocs`; a test fails when it is stale).
- `task skill`: regenerate `skills/apic/references/cheatsheet.md` from the
  site's `cheatsheet.md` with absolute links (`scripts/skilldocs`; a test
  fails when it is stale). `SKILL.md` is the briefing an agent reads on
  demand: keep it under 200 lines, point at the reference for detail, and
  know that its test fails on an `apic <word>` that is not a command.
- `task agent:eval` (`scripts/agenteval`) runs the tasks in
  `skills/apic/evals/evals.json` through the Claude Code CLI against fresh
  `apic demo` projects, with and without the skill, scoring the transcript
  (task outcome by a `verify` command, files changed, secret files read,
  requests sent, selector errors hit). It makes real model calls (about 40
  cents a full run), so it is a monthly workflow with a manual trigger
  (`agent-eval.yml`, needs the `ANTHROPIC_API_KEY` secret) rather than part
  of `task check`. Run it when the briefing or an agent-facing output
  changes; the schedule is the backstop.
- `task shots`: regenerate `website/public/assets/apic-ui.svg`,
  `apic-run.svg` and `apic-demo.svg` from real output (needs port 8089 free).
- `task tapes`: record the course's VHS tapes (`learn/tapes/` and
  `assets/demo.tape` in this directory) into `dist/tapes` with
  `scripts/tapes`, which serves the demo API on 8089 and gives each tape a
  scratch directory holding the files its lesson's runnable blocks create
  (the `setup` table in `scripts/tapes/main.go` says which); needs vhs,
  ttyd, ffmpeg and jq, which the tapes pipe through. `task tapes:check`
  validates the tapes' syntax with vhs alone, as CI does; VHS strings take no
  backslash escapes, so a command that needs both quote kinds goes in
  backticks. A test fails when a numbered lesson has neither a tape nor "No
  tape for this lesson" in its episode script.
