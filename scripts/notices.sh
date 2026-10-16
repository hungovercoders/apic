#!/bin/sh
# Generate THIRD_PARTY_NOTICES.md: the licence of every module compiled into apic.
#
# The module set is the union across every released platform, not just the one
# running this script: cobra pulls in mousetrap on Windows only, so a notices
# file generated on Linux alone would ship in the Windows archive incomplete.
#
# Alongside each module's licence, any NOTICE file is reproduced too (Apache
# License 2.0 section 4(d) requires it) and any PATENTS file (the additional
# grant the golang.org/x modules ship next to their BSD licence). Each module
# also gets the URL of its exact source on the Go module proxy: MPL-2.0
# section 3.2(a) requires telling recipients of a binary where the source of
# its MPL files is, and the link does no harm for the rest. The Go standard
# library and runtime, compiled into every binary, come first.
#
# Usage: scripts/notices.sh [output-file]
# Exits non-zero if a module has no licence file, so a release cannot ship
# notices with a hole in them.
set -eu
out="${1:-THIRD_PARTY_NOTICES.md}"

platforms="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"
raw_mods=$(mktemp)
trap 'rm -f "$raw_mods"' EXIT
for p in $platforms; do
  GOOS=${p%/*} GOARCH=${p#*/} go list -deps \
    -f '{{if and .Module (not .Standard)}}{{.Module.Path}}{{end}}' ./cmd/apic >> "$raw_mods"
done
mods=$(awk 'NF && $0 !~ /^github.com\/hungovercoders\/apic/' "$raw_mods" | sort -u)

# proxy_path escapes a module path the way the module proxy does: each
# upper-case letter becomes '!' and its lower-case form.
proxy_path() {
  printf '%s\n' "$1" | awk '{
    out = ""
    for (i = 1; i <= length($0); i++) {
      c = substr($0, i, 1)
      if (c ~ /[A-Z]/) out = out "!" tolower(c); else out = out c
    }
    print out
  }'
}

missing=""
{
  echo "# Third-party notices"
  echo
  echo "apic is built with Go from the following open source modules. Each is used under its own licence, reproduced below."
  echo
  echo "apic itself is released under the MIT Licence; see LICENSE."
  echo
  goroot=$(go env GOROOT)
  echo "## Go standard library and runtime $(go env GOVERSION)"
  echo
  echo "Source: https://go.dev/dl/"
  echo
  echo '```'
  cat "$goroot/LICENSE"
  echo '```'
  if [ -f "$goroot/PATENTS" ]; then
    echo
    echo "### PATENTS"
    echo
    echo '```'
    cat "$goroot/PATENTS"
    echo '```'
  fi
  echo
  for mod in $mods; do
    dir=$(go list -m -f '{{.Dir}}' "$mod")
    ver=$(go list -m -f '{{.Version}}' "$mod")
    echo "## $mod $ver"
    echo
    echo "Source: https://proxy.golang.org/$(proxy_path "$mod")/@v/$(proxy_path "$ver").zip"
    echo
    found=0
    for f in "$dir"/LICENSE "$dir"/LICENSE.txt "$dir"/LICENSE.md "$dir"/LICENCE "$dir"/COPYING "$dir"/License; do
      if [ -f "$f" ]; then
        echo '```'
        cat "$f"
        echo '```'
        found=1
        break
      fi
    done
    if [ "$found" = 1 ]; then
      for extra in NOTICE NOTICE.txt PATENTS; do
        [ -f "$dir/$extra" ] || continue
        echo
        echo "### $extra"
        echo
        echo '```'
        cat "$dir/$extra"
        echo '```'
      done
    else
      echo "_No licence file found in module; see the module's repository._"
      missing="$missing $mod"
    fi
    echo
  done
} > "$out"

echo "wrote $out ($(grep -c "^Source: https://proxy.golang.org/" "$out") modules)"
if [ -n "$missing" ]; then
  echo "no licence file found for:$missing" >&2
  exit 1
fi
