# Licensing

- apic is MIT (`LICENSE`, root). Keep it there: CI's `licences` job fails
  without it, and goreleaser ships it in every archive.
- `task notices` (`scripts/notices.sh`) regenerates the third-party notices
  file, which is generated rather than committed (it is in `.gitignore`);
  goreleaser runs it before packaging.
- The generator unions the module set across every released GOOS/GOARCH,
  not just the host: cobra pulls in `mousetrap` on Windows only. It also
  reproduces each module's `NOTICE` (required by Apache-2.0 4(d)) and
  `PATENTS` files, and exits non-zero if a module has no licence file at all.
- The VS Code extension bundles its runtime npm dependencies (today
  `vscode-languageclient` and what it pulls in) into `dist/extension.js`;
  `npm run package` runs `editors/vscode/notices.mjs` first, which writes the
  extension's own notices file (generated, gitignored) and fails on a package
  with no licence file. The same permissive-only rule applies there.
- Keep new dependencies permissive (MIT, BSD, Apache-2.0; the MPL-2.0
  modules godog pulls in are the existing exception). Anything reciprocal —
  GPL or LGPL — would change apic's own terms, so it is off the table for a
  statically linked binary.
- The rule is about what ships: the binary, the archives, the image and the
  extension. Tooling that only runs in CI, such as the npm devDependencies
  slopstopper's browser checks use (`axe-core` is MPL-2.0), is not
  distributed and is outside it.
- Write the AWS signer and the OpenAPI reader style of code from the spec,
  not by copying from another project; the tree carries no third-party
  source files and should stay that way.
