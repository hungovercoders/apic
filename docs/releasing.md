# Releasing

- `CHANGELOG.md` is where a release is written down, and the rest follows
  from it. A change a user would notice gets a line under Unreleased in
  the pull request that makes it, in
  [Keep a Changelog](https://keepachangelog.com/) form. Cutting a release
  is one pull request: turn Unreleased into `## [x.y.z] - YYYY-MM-DD`, add
  its compare link at the end of the file, start a new empty Unreleased
  above it, and run `task docs:changelog`, which renders the site's
  changelog page and rewrites every place the docs pin a release (the
  install commands, `setup-apic`'s `version:`, the image tags in the
  cookbook and on the verifying page, the course's sample output) to the
  newest dated section that is not a prerelease. A test fails until it
  has run, so a dated section cannot merge with docs naming the last
  release. The page leaves Unreleased out, so the pull requests that add
  lines there need not run it.
- Merging the release pull request is the release. `release.yml` runs on
  every push to `main` that changes `CHANGELOG.md`: its `plan` job takes
  the newest dated section (`scripts/changelog -latest`) and does nothing
  when that version already has a GitHub release, which is every push
  that only added lines under Unreleased. Otherwise the `goreleaser` job
  checks out the commit that brought the dated heading onto `main` (found
  with `git log -S`, so a manual run or a re-run never ships work merged
  since), runs the checks below, tags it, and releases it in the same run.
  The tag is pushed with `GITHUB_TOKEN`, which starts no workflow, so the
  run is the only release of it. The site, which deploys from `main`,
  names the new version for the minutes the release takes; if a release
  fails, its install commands name archives that do not exist until it is
  fixed and re-run, so treat a red release run as urgent. Pushing a
  version tag by hand still releases that tag, and the `release`
  concurrency group makes a hand-pushed tag that races the merge wait and
  then find the release done.
- Before tagging, the job runs `scripts/changelog -release <tag>`. It fails
  the run with nothing built or tagged unless the tag is the newest dated
  section and the docs already name it, then passes the section to
  goreleaser as `--release-notes`; `task notes -- 0.2.0` prints what it
  would attach.
  A prerelease tag (`v0.3.0-rc.1`) needs its own dated section like any
  other, and leaves the docs' pins on the last stable release.
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
  release. The `plan` job releases only tags containing a dot, so the
  moving tag never starts a release of its own.
- The VS Code extension is released separately, on `vscode-v<version>`
  tags: `vscode-release.yml` checks the tag against
  `editors/vscode/package.json`, runs the extension's tests, publishes to the
  Marketplace and Open VSX (secrets `VSCE_PAT` and `OVSX_PAT`; without them
  the publish steps are skipped) and attaches the `.vsix` to a GitHub
  release with the matching `CHANGELOG.md` section as notes. A binary tag
  never touches the extension and an extension tag never touches the
  binary; `release.yml` matches `v*`, which `vscode-v*` does not start with.
  The extension's `MIN_VERSION` in `src/apic.ts` names the oldest binary it
  drives; raise it when the extension starts to depend on a newer `--json`
  key.
