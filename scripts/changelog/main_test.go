package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `# Changelog

## [Unreleased]

## [0.2.0] - 2026-10-27

### Added

- A thing.

## [0.1.1] to [0.1.3] - 2026-10-10

Pipeline only.

## [0.1.0] - 2026-10-10

The first release.

[Unreleased]: https://example.com/compare/v0.2.0...HEAD
[0.2.0]: https://example.com/compare/v0.1.4...v0.2.0
[0.1.3]: https://example.com/compare/v0.1.2...v0.1.3
[0.1.2]: https://example.com/compare/v0.1.1...v0.1.2
[0.1.1]: https://example.com/compare/v0.1.0...v0.1.1
[0.1.0]: https://example.com/releases/tag/v0.1.0
`

func TestSectionFindsAVersionAndRefusesWhatIsNotThere(t *testing.T) {
	got, err := Section(sample, "0.2.0")
	if err != nil || got != "### Added\n\n- A thing." {
		t.Fatalf("0.2.0: %q %v", got, err)
	}
	// A range heading serves every version it names and those between.
	for _, v := range []string{"0.1.1", "0.1.2", "0.1.3"} {
		if got, err := Section(sample, v); err != nil || got != "Pipeline only." {
			t.Fatalf("%s: %q %v", v, got, err)
		}
	}
	if _, err := Section(sample, "0.1.4"); err == nil || !strings.Contains(err.Error(), "no section for 0.1.4") {
		t.Fatalf("0.1.4 is not in the range: %v", err)
	}
	if _, err := Section(sample, "Unreleased"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("an empty section is refused: %v", err)
	}
	if _, err := Section(sample, "9.9.9"); err == nil {
		t.Fatal("a missing version should be an error")
	}
	// The oldest section ends at the link references, not the file's end.
	if got, err := Section(sample, "0.1.0"); err != nil || got != "The first release." {
		t.Fatalf("0.1.0: %q %v", got, err)
	}
}

func TestCheckNamesEveryShapeProblem(t *testing.T) {
	if p := Check(sample); len(p) != 0 {
		t.Fatalf("the sample is well formed: %v", p)
	}
	bad := "# x\n\n## [0.2.0] - 2026-10-27\n\nA.\n\n## [0.3.0]\n\nB.\n\n## Notes\n\n[0.2.0]: u\n"
	p := strings.Join(Check(bad), "\n")
	for _, want := range []string{"first section must be Unreleased", "0.3.0 has no release date", "no link reference for [0.3.0]", "0.3.0 is not below 0.2.0", "is not a release heading"} {
		if !strings.Contains(p, want) {
			t.Errorf("missing %q in:\n%s", want, p)
		}
	}
	if compareVersions("0.10.0", "0.9.1") <= 0 || compareVersions("Unreleased", "1.0.0") <= 0 || compareVersions("1.0", "1.0.0") != 0 {
		t.Fatal("version order")
	}
	// A prerelease sits between the release before it and the one it leads to.
	if compareVersions("0.3.0-rc.1", "0.3.0") >= 0 || compareVersions("0.3.0-rc.1", "0.2.0") <= 0 || compareVersions("0.3.0-rc.2", "0.3.0-rc.1") <= 0 {
		t.Fatal("prerelease order")
	}
	// Semver's order for the suffix: words by their bytes, a number below a
	// word, a longer suffix above its own start.
	order := []string{"0.3.0-1", "0.3.0-alpha", "0.3.0-alpha.1", "0.3.0-alpha.2", "0.3.0-alpha.10", "0.3.0-beta.1", "0.3.0-beta.2", "0.3.0-rc.1", "0.3.0"}
	for i := 1; i < len(order); i++ {
		if compareVersions(order[i-1], order[i]) >= 0 {
			t.Errorf("%s should be below %s", order[i-1], order[i])
		}
	}
	ordered := "## [Unreleased]\n\n## [0.3.0-rc.1] - 2026-10-29\n\nC.\n\n## [0.3.0-beta.2] - 2026-10-28\n\nB.\n\n## [0.3.0-alpha.1] - 2026-10-27\n\nA.\n\n[Unreleased]: u\n[0.3.0-rc.1]: u\n[0.3.0-beta.2]: u\n[0.3.0-alpha.1]: u\n"
	if p := Check(ordered); len(p) != 0 {
		t.Fatalf("validly ordered prereleases: %v", p)
	}
	if p := strings.Join(Check("## [Unreleased]\n\n## [1] - 2026-10-27\n\nA.\n\n[Unreleased]: u\n[1]: u\n"), "\n"); !strings.Contains(p, `"1" is not a version`) {
		t.Fatalf("a malformed version: %s", p)
	}
}

// The repository's own changelog: well formed, with notes for every tag
// release.yml would attach, which is how a tag without them fails here
// before it fails on the runner.
func TestTheChangelogIsWellFormed(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	if p := Check(string(data)); len(p) != 0 {
		t.Fatalf("CHANGELOG.md:\n%s", strings.Join(p, "\n"))
	}
	for _, v := range []string{"0.2.0", "0.1.4", "0.1.3", "0.1.2", "0.1.1", "0.1.0"} {
		if _, err := Section(string(data), v); err != nil {
			t.Error(err)
		}
	}
}

// The site's changelog page and every version pin in the docs follow the
// newest dated section, so a release pull request that dates one fails
// here until `task docs:changelog` has caught the docs up.
func TestTheDocsNameTheCurrentRelease(t *testing.T) {
	root := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	stale, err := Stale(root, string(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) > 0 {
		t.Fatalf("%s are stale; run task docs:changelog", strings.Join(stale, ", "))
	}
}

func TestLatestAndTheSitePage(t *testing.T) {
	if v, err := Latest(sample); err != nil || v != "0.2.0" {
		t.Fatalf("latest = %q %v", v, err)
	}
	ranged := strings.Replace(sample, "## [0.2.0] - 2026-10-27\n\n### Added\n\n- A thing.\n\n", "", 1)
	if v, _ := Latest(ranged); v != "0.1.3" {
		t.Fatalf("a range's latest is its upper end, got %q", v)
	}
	page := SitePage(sample)
	if !strings.HasPrefix(page, "---\ntitle: Changelog\n") || strings.Contains(page, "# Changelog") {
		t.Fatalf("front matter in place of the H1:\n%s", page)
	}
	if strings.Contains(page, "[Unreleased]") || !strings.Contains(page, "## [0.2.0] - 2026-10-27") || !strings.Contains(page, "[0.2.0]: ") {
		t.Fatalf("Unreleased goes, the releases stay:\n%s", page)
	}
	// A line under Unreleased, which every user-facing pull request adds,
	// leaves the page as it was; so does a heading with trailing spaces.
	for _, variant := range []string{
		strings.Replace(sample, "## [Unreleased]\n", "## [Unreleased]\n\n### Added\n\n- Coming.\n", 1),
		strings.Replace(sample, "## [Unreleased]\n", "## [Unreleased]  \n", 1),
		// A line that opens with a link is not a link reference.
		strings.Replace(sample, "## [Unreleased]\n", "## [Unreleased]\n\n[apic mock](cli.md#apic-mock) answers HEAD.\n\n- More.\n", 1),
	} {
		if p := SitePage(variant); p != page {
			t.Fatalf("Unreleased changed the page:\n%s", p)
		}
	}
	// Latest is any release, the docs pin the newest stable one, and a
	// heading that is not a version is an error, not a panic.
	rc := strings.Replace(sample, "## [0.2.0] - 2026-10-27", "## [0.3.0-rc.1] - 2026-10-28\n\n- Candidate.\n\n## [0.2.0] - 2026-10-27", 1)
	if v, _ := Latest(rc); v != "0.3.0-rc.1" {
		t.Fatalf("latest = %q", v)
	}
	if v, _ := Stable(rc); v != "0.2.0" {
		t.Fatalf("stable = %q", v)
	}
	if _, err := Latest("## [1] - 2026-10-27\n"); err == nil || !strings.Contains(err.Error(), "not a version") {
		t.Fatalf("a bad version: %v", err)
	}
}

// A pin rewrites the version wherever its phrase appears, and a phrase
// that is gone fails rather than leaving the old version in place.
func TestPinsRewriteAndRefuseAMissingPhrase(t *testing.T) {
	root := t.TempDir()
	saved := Pins
	t.Cleanup(func() { Pins = saved })
	Pins = []Pin{
		pin("a.md", "version: v{v}"),
		pin("a.md", "minor (`v{minor}`)"),
	}
	// Everything but the placeholders is literal: a dot in a host is a dot.
	host := pin("a.md", "ghcr.io/x:{v} ")
	if !host.re.MatchString("ghcr.io/x:0.2.0 ") || host.re.MatchString("ghcrXio/x:0.2.0 ") {
		t.Fatalf("pin pattern %s", host.re)
	}
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("version: v0.1.4\nversion: v0.1.4 # x\nminor (`v0.1`)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, after, err := pinned(root, "a.md", "0.2.0")
	if err != nil || after != "version: v0.2.0\nversion: v0.2.0 # x\nminor (`v0.2`)\n" {
		t.Fatalf("%q %v", after, err)
	}
	// A prerelease's suffix is part of the version the pin matches, so a
	// second run changes nothing rather than adding the suffix again.
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte(after), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		_, after, err = pinned(root, "a.md", "0.3.0-rc.1")
		if err != nil || after != "version: v0.3.0-rc.1\nversion: v0.3.0-rc.1 # x\nminor (`v0.3`)\n" {
			t.Fatalf("run %d: %q %v", i+1, after, err)
		}
		if err := os.WriteFile(filepath.Join(root, "a.md"), []byte(after), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("reworded\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := pinned(root, "a.md", "0.2.0"); err == nil || !strings.Contains(err.Error(), "no match for the pin") {
		t.Fatalf("a missing phrase: %v", err)
	}
}

// release.yml's check: the tag must be the newest dated section, and the
// docs must already name it.
func TestReleaseChecksTheTagAndTheDocs(t *testing.T) {
	root := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	latest, err := Latest(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if notes, err := Release(root, string(data), latest); err != nil || notes == "" {
		t.Fatalf("the current release: %v", err)
	}
	if _, err := Release(root, string(data), "0.1.0"); err == nil || !strings.Contains(err.Error(), "newest dated section is "+latest) {
		t.Fatalf("an older tag: %v", err)
	}
}
