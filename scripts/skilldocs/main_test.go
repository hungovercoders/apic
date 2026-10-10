package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hungovercoders/apic/internal/cli"
)

const root = "../.."

func read(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	// A checkout with autocrlf (the Windows runner) is not staleness.
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// TestReferenceIsCurrent fails when the site's cheatsheet.md changed without
// `task skill`, which is how CI catches a stale copy in the skill.
func TestReferenceIsCurrent(t *testing.T) {
	if got, want := read(t, Target), Render(read(t, Source)); got != want {
		t.Fatalf("%s is stale; run `task skill`", Target)
	}
}

// The page's front matter is for the site; the skill gets a heading.
func TestRenderReplacesFrontMatter(t *testing.T) {
	got := Render("---\ntitle: \"Cheat sheet\"\n---\n\nBody with a [link](format.md#x).\n")
	want := "<!-- Generated from " + Source + " by scripts/skilldocs (`task skill`); edit that page, not this file. -->\n" +
		"# apic cheat sheet\nBody with a [link](" + Site + "format/#x).\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

// Every link in the reference must work from a project that only has the
// skill: absolute, or an anchor on the page itself.
func TestLinksAreAbsolute(t *testing.T) {
	ref := read(t, Target)
	for _, m := range regexp.MustCompile(`\]\(([^)]+)\)`).FindAllStringSubmatch(ref, -1) {
		if to := m[1]; !strings.HasPrefix(to, "https://") && !strings.HasPrefix(to, "#") {
			t.Errorf("relative link %q would not resolve inside the skill", to)
		}
	}
	if !strings.Contains(ref, "]("+Site+"format/#body-paths)") {
		t.Error("expected format.md#body-paths to become a site link")
	}
}

// The skill's frontmatter is what an agent sees before deciding to read the
// rest, and the body is read in full every time it does, so both are
// checked: a name and a description within the Agent Skills limits, and a
// body short enough to be worth loading.
func TestSkill(t *testing.T) {
	skill := read(t, "skills/apic/SKILL.md")
	parts := strings.SplitN(skill, "---\n", 3)
	if len(parts) != 3 || parts[0] != "" {
		t.Fatal("SKILL.md must start with a --- frontmatter block")
	}
	front, body := parts[1], parts[2]
	if !strings.Contains(front, "name: apic\n") {
		t.Error("frontmatter must name the skill apic")
	}
	desc := regexp.MustCompile(`(?m)^description: (.+)$`).FindStringSubmatch(front)
	if desc == nil || len(desc[1]) > 1024 {
		t.Error("frontmatter needs a one-line description of at most 1024 characters")
	}
	if n := strings.Count(body, "\n"); n > 200 {
		t.Errorf("SKILL.md body is %d lines; keep it under 200 and put detail in references/", n)
	}
	if !strings.Contains(body, "](references/cheatsheet.md)") {
		t.Error("SKILL.md should point at references/cheatsheet.md for the full format")
	}
	// Every `apic <command>` the skill mentions must be a real command, so
	// a rename or removal fails here rather than in an agent's shell.
	known := map[string]bool{}
	for _, c := range cli.New().Root.Commands() {
		known[c.Name()] = true
	}
	for _, m := range regexp.MustCompile(`\bapic ([a-z]+)\b`).FindAllStringSubmatch(body, -1) {
		if !known[m[1]] {
			t.Errorf("SKILL.md mentions `apic %s`, which is not a command", m[1])
		}
	}
}
