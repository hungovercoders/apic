# Releasing

- `CHANGELOG.md` is where a release is written down, and the rest follows
  from it. A change a user would notice gets a line under Unreleased in
  the pull request that makes it, in
  [Keep a Changelog](https://keepachangelog.com/) form. Cutting a release
  is one pull request that touches only `CHANGELOG.md`: turn Unreleased
  into `## [x.y.z] - YYYY-MM-DD`, add its compare link at the end of the
  file, and start a new empty Unreleased above it.
- The site follows the newest *published* release, recorded in
  `website/released-version`, not the newest dated section, so it never
  names a version whose archives do not exist yet. `task docs:changelog`
  renders the site's changelog page with the sections up to that release
  and rewrites every place the docs pin a release (the install commands,
  `setup-apic`'s `version:`, the image tags and SBOM file in the cookbook
  and on the verifying page, the course's sample output) to the newest
  stable release at or below it; a test fails while either is stale. A
  dated section above the published release is not on the page, so the
  release pull request changes nothing there; once the release is out,
  `release.yml`'s `docs` job runs `scripts/changelog -publish <tag>`,
  which records the release and regenerates, and commits the result to
  `main` with `GITHUB_TOKEN`, retrying if `main` moved meanwhile. That
  push starts no workflow; the site deploys from it as from any push. It
  needs `main` to accept a push from the workflow's token: if a ruleset
  ever forbids it, the job fails and says to run `-publish` on `main` by
  hand.
- Merging the release pull request is the release. `release.yml` runs on
  every push to `main` that changes `CHANGELOG.md` or anything else the
  release check reads (the docs its pins govern, `website/released-version`,
  `scripts/changelog`), so the push that mends whatever held a dated
  release back also tags it, and on a manual run on `main`. Its `tag` job takes
  the newest dated section (`scripts/changelog -latest`) and stops when
  that version is tagged already, which is every push that only added
  lines under Unreleased. Otherwise it tags the commit the release pull
  request left on `main` (`scripts/changelog -release-commit`): walking
  `main`'s first-parent history back to where the version was dated, the
  first commit at which the release checks below pass. A later push that
  runs the job first, or a retry on a later push, still tags that commit,
  so changes merged since stay under Unreleased and out of this release.
  It then dispatches `release.yml` on the
  new tag, where the `goreleaser` job releases it as it would a tag pushed
  by hand. The release always runs on a tag ref, so the keyless
  certificate names `release.yml@refs/tags/<tag>`, the identity the site's
  verifying page checks. The tag is pushed with `GITHUB_TOKEN`, which
  starts no run of its own; `workflow_dispatch` is the one event it may
  start, hence the `actions: write`. A manual run on any branch but
  `main` is refused, so nothing unmerged is released. Re-running the tag
  job, or a manual run on `main`, starts the release of a version that is
  tagged but has neither a release nor a release run (its dispatch failed
  after the push); one that has either is left to that run. An ordinary
  push never does that, so two pushes racing cannot release a tag twice.
  To release a tag again, re-run its run or `gh workflow run release.yml
  --ref <tag>`. GitHub reads the workflow at the tag's commit, so dispatch
  works only for tags cut after this workflow gained `workflow_dispatch`;
  for v0.2.0 and earlier, re-run the tag's own run from the Actions page.
  Only the newest version tag may be released again: releasing an older
  one would move `latest`, the cask and `v0` back to it, so the release
  check refuses a tag with a higher version above it. goreleaser keeps an
  existing release and replaces what it
  uploads again, so a re-run after a later step failed (the cask, the smoke
  test) completes the same release, and the `mcp-registry` job skips a
  version the registry lists already. A failed release leaves the site on
  the previous one, since the `docs` job runs only after goreleaser has
  published.
- Before tagging, and again before building, the workflow runs
  `scripts/changelog -release <tag>`. It fails the run with nothing built
  or tagged unless the tag is the newest dated section, no higher version
  is tagged and the docs are current for the release published before it,
  then passes the section to goreleaser as `--release-notes` as it is
  written; `task notes -- 0.2.0` prints what it would attach. Each
  paragraph and bullet in `CHANGELOG.md` is one line (let the editor wrap
  it): GitHub shows a release body's single newlines as breaks, so a
  hard-wrapped bullet would break mid-sentence there, and the changelog's
  check refuses one.
  A prerelease tag (`v0.3.0-rc.1`) needs its own dated section like any
  other. It is published as a GitHub prerelease (`prerelease: auto`), so
  it never becomes `releases/latest`. Publishing it puts its notes on the
  site's changelog page, but it leaves the docs' pins, the
  `v0` tag, the Homebrew cask, the image's `latest` and the MCP registry
  listing on the last stable release.
  The generator's tests also check the file's shape (Unreleased first, a
  date on every release, newest first, a link reference for each
  version). A new place in the docs that names the current release goes
  into `Pins` in `scripts/changelog/site.go`, not into the page by hand;
  a pin whose phrase is reworded fails the test rather than keeping the old
  version. goreleaser's own commit-list changelog is left configured but
  never used: it listed every commit since the repository began.
- Releases are signed with cosign, keyless: the certificate is bound to the
  release workflow's OIDC identity, so `release.yml` needs `id-token: write`
  on the job. There is no private key.
- Only `checksums.txt` is signed. It names every archive with its SHA-256,
  so one signature covers the release; verifying is a two-step chain,
  documented on the site's `verifying.md`.
- Each archive gets an SPDX 2.3 SBOM from syft, per archive rather than per
  release because the module set differs by platform (cobra pulls in
  `mousetrap` on Windows only).
- `cosign` and `syft` are installed by `release.yml`; neither ships with the
  runner or with goreleaser-action. goreleaser tries to sign even on a
  snapshot and fails hard without cosign, so `task snapshot` passes
  `--skip=sign,sbom` — a local snapshot is a build sanity check, not a
  release.
- Before merging a release pull request: the changelog section above,
  `task check`, then `task snapshot` to prove archive names, contents and
  version injection.
  `apic version` from an extracted archive must report the version, not
  `dev` — a typo in the ldflags path fails silently.
- The image `ghcr.io/hungovercoders/apic` is `Dockerfile` built by
  goreleaser's `dockers_v2` for linux/amd64 and linux/arm64: the binary on
  `scratch` with Alpine's CA bundle and a `/tmp`, laid out by an Alpine
  stage that runs on the build platform, so no QEMU. `release.yml` needs
  `packages: write`, logs in to GHCR and smoke-runs the pushed tag;
  `docker_signs` signs it keylessly. `task snapshot` builds it locally (needs
  Docker) without pushing.
- The image is also what the MCP registry
  (registry.modelcontextprotocol.io) lists for `apic mcp`, as
  `io.github.hungovercoders/apic`. `server.json` at the root is the listing
  with its version at `0.0.0`; the `mcp-registry` job in `release.yml`
  runs after the release, sets the tag's version and the image tag with
  `jq`, logs in with `mcp-publisher login github-oidc` (so `id-token:
  write`; the namespace comes from the repository's owner, who must own the
  organisation) and publishes. The registry fetches the image and checks
  its `io.modelcontextprotocol.server.name` label, set in
  `.goreleaser.yaml`, before it will list it, and refuses a version it has
  seen. `mcp-publisher` is pinned by tag and verified against the
  registry's own release signature; bump `MCP_PUBLISHER_VERSION`
  deliberately. The job is separate from `goreleaser` so a registry
  failure fails the listing, not the release: re-run the job alone.
- `nfpms:` builds `.deb`, `.rpm` and `.apk` packages (amd64 and arm64) with
  the binary in `/usr/bin`, the man pages, shell completions (generated by a
  `before` hook into `completions/`, per-packager paths for zsh) and the
  licence files; they are in `checksums.txt`, so the signature covers them.
  CI's `packages` job builds them with a snapshot and installs each on its
  distribution. There is no package repository.
- `homebrew_casks:` writes `Casks/apic.rb` to `hungovercoders/homebrew-tap`
  on each release (`brew install hungovercoders/tap/apic`, macOS and Linux).
  It is a cask, not a formula: goreleaser deprecated `brews:` for prebuilt
  binaries. The cask installs the binary, the archive's `man/` pages (listed
  one by one in `.goreleaser.yaml`; `scripts/clidocs` has a test that fails
  when a command is added without its line) and `completions/`, which the
  archive carries for that reason. `release.yml` needs the
  `HOMEBREW_TAP_GITHUB_TOKEN` secret, a fine-grained PAT with contents write
  on the tap only, and fails before building when it is missing. A snapshot
  renders the cask into `dist/homebrew/` without pushing; CI's `packages`
  job checks it.
- `install.sh`, `install.ps1` and `setup-apic/action.yml` build their URLs
  from the tag with the leading `v` stripped, which is what goreleaser's
  `.Version` gives; changing `archives.name_template` breaks all three.
  `install.ps1` runs in a script block and throws rather than exits, because
  `irm | iex` runs it in the caller's session; `action-test.yml` runs it on
  Windows against the latest release (5.1 and 7) and against a local mirror,
  including a tampered checksum.
- After each release, `release.yml` force-moves the major tag (`v0` today,
  `v1` later) to the released commit so `setup-apic@v0` follows the newest
  release. The `goreleaser` job releases only tags containing a dot, so the
  moving tag never starts a release of its own.
- The VS Code extension is released separately, on `vscode-v<version>`
  tags: `vscode-release.yml` checks the tag against
  `editors/vscode/package.json`, runs the extension's tests, publishes to the
  Marketplace and Open VSX (secrets `VSCE_PAT` and `OVSX_PAT`; without them
  the publish steps are skipped) and attaches the `.vsix` to a GitHub
  release with the matching `CHANGELOG.md` section as notes. A binary tag
  never touches the extension and an extension tag never touches the
  binary; `release.yml` only takes tags matching `v[0-9]*`, which `vscode-v*` does
  not.
  The extension's `MIN_VERSION` in `src/apic.ts` names the oldest binary it
  drives; raise it when the extension starts to depend on a newer `--json`
  key.
