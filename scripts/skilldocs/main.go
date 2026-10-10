// Command skilldocs writes skills/apic/references/cheatsheet.md from the
// site's cheatsheet.md, the one page an agent needs when it writes a request.
// The skill is copied into other projects, where the docs' relative links
// mean nothing, so each becomes a link to the published site. A test fails
// when the copy is stale; `task skill` regenerates it.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Site is where the docs are published. A page links to another as
// `format.md#anchor`; the site serves it at `format/#anchor`
// (website/plugins/remark-doc-links.mjs does the same rewrite at build).
const Site = "https://apic.sh/"

// Source and Target are relative to the repository root.
const (
	Source = "website/src/content/docs/cheatsheet.md"
	Target = "skills/apic/references/cheatsheet.md"
)

var reDocLink = regexp.MustCompile(`\]\(([a-z0-9/-]+)\.md(#[^)]*)?\)`)

// Render turns the cheat sheet into the skill's reference: a notice that it
// is generated, and every relative link to another page made absolute.
func Render(cheatsheet string) string {
	body := reDocLink.ReplaceAllString(cheatsheet, "]("+Site+"$1/$2)")
	return "<!-- Generated from " + Source + " by scripts/skilldocs (`task skill`); edit that page, not this file. -->\n" +
		"# apic cheat sheet\n" + strings.TrimPrefix(body, "# Cheat sheet\n")
}

func run(root string) error {
	src, err := os.ReadFile(filepath.Join(root, Source)) //nolint:gosec // the repository's own cheat sheet, at a constant path
	if err != nil {
		return err
	}
	out := filepath.Join(root, Target)
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		return err
	}
	return os.WriteFile(out, []byte(Render(string(src))), 0o644) //nolint:gosec // a generated doc, not a secret
}

func main() {
	if err := run("."); err != nil {
		fmt.Fprintln(os.Stderr, "skilldocs:", err)
		os.Exit(1)
	}
}
