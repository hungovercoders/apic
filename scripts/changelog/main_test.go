package main

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
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
	// A release pull request that dates Unreleased instead of renaming it,
	// a second Unreleased, and a range written backwards are all refused,
	// so Latest never answers "Unreleased" and Section finds every version
	// a range covers.
	for heading, want := range map[string]string{
		"## [Unreleased] - 2026-10-28":                          "Unreleased has a date",
		"## [Unreleased]\n\n- x\n\n## [Unreleased]":             "only the first section is Unreleased",
		"## [Unreleased]\n\n## [0.1.3] to [0.1.1] - 2026-10-10": "runs backwards",
	} {
		changelog := heading + "\n\nA.\n\n[Unreleased]: u\n[0.1.1]: u\n[0.1.3]: u\n"
		if p := strings.Join(Check(changelog), "\n"); !strings.Contains(p, want) {
			t.Errorf("%q: want %q in:\n%s", heading, want, p)
		}
		if v, err := Latest(changelog); err == nil {
			t.Errorf("%q: Latest = %q, want an error", heading, v)
		}
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
	stale, err := Stale(fromDisk(root), string(data))
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
	rc := strings.Replace(sample, "## [0.2.0] - 2026-10-27", "## [0.3.0-rc.1] - 2026-10-28\n\n- Candidate.\n\n## [0.2.0] - 2026-10-27", 1) + "[0.3.0-rc.1]: u\n"
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
	_, after, err := pinned(fromDisk(root), "a.md", "0.2.0")
	if err != nil || after != "version: v0.2.0\nversion: v0.2.0 # x\nminor (`v0.2`)\n" {
		t.Fatalf("%q %v", after, err)
	}
	// A prerelease's suffix is part of the version the pin matches, so a
	// second run changes nothing rather than adding the suffix again.
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte(after), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		_, after, err = pinned(fromDisk(root), "a.md", "0.3.0-rc.1")
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
	if _, _, err := pinned(fromDisk(root), "a.md", "0.2.0"); err == nil || !strings.Contains(err.Error(), "no match for the pin") {
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
	if notes, err := Release(fromDisk(root), string(data), latest); err != nil || notes == "" {
		t.Fatalf("the current release: %v", err)
	}
	if _, err := Release(fromDisk(root), string(data), "0.1.0"); err == nil || !strings.Contains(err.Error(), "newest dated section is "+latest) {
		t.Fatalf("an older tag: %v", err)
	}
}

// A link reference inside a section is part of its body, and a fenced
// example may hold lines that look like headings: neither ends a section,
// leaks Unreleased onto the page, or fails Check. Only the block of
// references that ends the file is the file's link references.
func TestReferencesAndFencesInsideSections(t *testing.T) {
	body := "### Added\n\n- Errors follow [RFC 9457][rfc].\n\n[rfc]: https://example.com/rfc9457\n\n```markdown\n## Notes\n[x]: y\n```\n\n- After the example."
	withBody := strings.Replace(sample, "### Added\n\n- A thing.", body, 1)
	if p := Check(withBody); len(p) != 0 {
		t.Fatalf("well formed: %v", p)
	}
	if got, err := Section(withBody, "0.2.0"); err != nil || got != body {
		t.Fatalf("the whole section, reference and example included:\n%s\n%v", got, err)
	}
	unrel := strings.Replace(sample, "## [Unreleased]\n", "## [Unreleased]\n\n- Errors follow [RFC 9457][rfc].\n\n[rfc]: https://example.com\n\n- Still unreleased.\n", 1)
	if page := SitePage(unrel); page != SitePage(sample) {
		t.Fatalf("Unreleased leaked onto the page:\n%s", page)
	}
}

// A changelog that fails Check has no newest release to trust: a section
// dated below an older one would hide it and the tag job would skip it.
func TestLatestRefusesAMisorderedChangelog(t *testing.T) {
	misordered := strings.Replace(sample, "## [0.1.1] to [0.1.3] - 2026-10-10\n\nPipeline only.\n\n", "", 1)
	misordered = strings.Replace(misordered, "## [0.1.0] - 2026-10-10\n\nThe first release.\n", "## [0.1.0] - 2026-10-10\n\nThe first release.\n\n## [0.3.0] - 2026-10-28\n\nNew.\n", 1) + "[0.3.0]: u\n"
	if _, err := Latest(misordered); err == nil || !strings.Contains(err.Error(), "newest first") {
		t.Fatalf("a misordered changelog: %v", err)
	}
}

// The commit to tag is the first on main, after the section was dated,
// at which the release check passes: not the dating commit while the docs
// still lag, and not a later push that only added a line under Unreleased.
func TestReleaseCommitIsWhereTheReleaseWasCut(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	saved := Pins
	t.Cleanup(func() { Pins = saved })
	Pins = []Pin{pin("docs.md", "version: v{v}")}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(msg string) string {
		git("add", "-A")
		git("commit", "-q", "-m", msg)
		return git("rev-parse", "HEAD")
	}
	v1 := "# Changelog\n\n## [Unreleased]\n\n## [0.1.0] - 2026-10-01\n\nFirst.\n\n[Unreleased]: u\n[0.1.0]: u\n"
	v2 := "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] - 2026-10-02\n\nSecond.\n\n## [0.1.0] - 2026-10-01\n\nFirst.\n\n[Unreleased]: u\n[0.2.0]: u\n[0.1.0]: u\n"
	git("init", "-q")
	write("CHANGELOG.md", v1)
	write("docs.md", "version: v0.1.0\n")
	if err := Write(root, v1); err != nil {
		t.Fatal(err)
	}
	commit("0.1.0")
	write("CHANGELOG.md", v2)
	dated := commit("date 0.2.0")
	if err := Write(root, v2); err != nil {
		t.Fatal(err)
	}
	caughtUp := commit("task docs:changelog")
	// A later merge breaks the file and the next mends it: the walk skips
	// the broken commit rather than starting the release after it.
	write("CHANGELOG.md", strings.Replace(v2, "## [Unreleased]\n", "## [Unreleased]\n\n## Notes\n", 1))
	commit("a malformed heading")
	write("CHANGELOG.md", v2)
	commit("mend it")
	write("CHANGELOG.md", strings.Replace(v2, "## [Unreleased]\n", "## [Unreleased]\n\n- Later.\n", 1))
	later := commit("a line under Unreleased")

	got, err := ReleaseCommit(root, "CHANGELOG.md", later, "0.2.0")
	if err != nil || got != caughtUp {
		t.Fatalf("got %s %v; want %s (dated %s, later %s)", got, err, caughtUp, dated, later)
	}
	if _, err := ReleaseCommit(root, "CHANGELOG.md", dated, "0.2.0"); err == nil || !strings.Contains(err.Error(), "passes the release check") {
		t.Fatalf("docs never caught up: %v", err)
	}
	if _, err := ReleaseCommit(root, "CHANGELOG.md", later, "0.3.0"); err == nil || !strings.Contains(err.Error(), "not the newest dated section") {
		t.Fatalf("an undated version: %v", err)
	}
}

// Reading through git tells a file a commit does not have from git
// failing, and NewerTags finds the version tags above a release.
func TestGitReaderAndNewerTags(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "a.md"), []byte("sub\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "a")
	read := fromGit(root, "HEAD")
	if got, err := read("a.md"); err != nil || got != "a\n" {
		t.Fatalf("a.md: %q %v", got, err)
	}
	// From a subdirectory, paths are relative to it, as on disk.
	if got, err := fromGit(filepath.Join(root, "sub"), "HEAD")("a.md"); err != nil || got != "sub\n" {
		t.Fatalf("sub/a.md: %q %v", got, err)
	}
	if _, err := read("missing.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a missing file is fs.ErrNotExist: %v", err)
	}
	var ge *gitError
	if _, err := fromGit(root, "no-such-commit")("a.md"); !errors.As(err, &ge) {
		t.Fatalf("a bad commit is a gitError: %v", err)
	}
	if _, err := ReleaseCommit(root, "CHANGELOG.md", "no-such-commit", "0.1.0"); !errors.As(err, &ge) {
		t.Fatalf("ReleaseCommit passes a git failure on: %v", err)
	}
	for _, tag := range []string{"v0", "v0.1.0", "v0.2.0-rc.1", "v0.2.0", "vscode-v0.9.0"} {
		run("tag", tag)
	}
	for tag, want := range map[string]string{"0.1.0": "v0.2.0-rc.1 v0.2.0", "0.2.0-rc.1": "v0.2.0", "0.2.0": ""} {
		newer, err := NewerTags(root, tag)
		if err != nil || strings.Join(newer, " ") != want {
			t.Errorf("above %s: %v %v, want %q", tag, newer, err, want)
		}
	}
}

// The notes GitHub shows are unwrapped: one line per paragraph and list
// item, while headings, tables, references and fenced code keep theirs.
func TestUnwrapJoinsWrappedLines(t *testing.T) {
	in := "### Added\n\n- `apic mcp install` registers the server in the\n  project's `.mcp.json`, with `--to\n  .cursor/mcp.json` for Cursor.\n  - nested item that wraps\n    onto a second line\n- second item\n\nA paragraph that\nwraps.\n\n| a | b |\n|---|---|\n\n```sh\napic mcp install\n  --to x\n```\n\n[ref]: https://example.com"
	want := "### Added\n\n- `apic mcp install` registers the server in the project's `.mcp.json`, with `--to .cursor/mcp.json` for Cursor.\n  - nested item that wraps onto a second line\n- second item\n\nA paragraph that wraps.\n\n| a | b |\n|---|---|\n\n```sh\napic mcp install\n  --to x\n```\n\n[ref]: https://example.com"
	if got := Unwrap(in); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	// The notes release.yml attaches are unwrapped.
	root := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	latest, _ := Latest(string(data))
	notes, err := Release(fromDisk(root), string(data), latest)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(notes, "\n") {
		if strings.HasPrefix(line, "  ") && !listItem.MatchString(line) {
			t.Fatalf("a wrapped continuation line in the notes: %q", line)
		}
	}
}

// A fence closes only on a bare fence line of the same kind, at least as
// long: an inner "```http" in a ```` example is text, and so is the "## "
// line after it.
func TestNestedFences(t *testing.T) {
	body := "- Example:\n\n````markdown\n```http\nGET /\n```\n## Usage\n````\n\n- After."
	withBody := strings.Replace(sample, "### Added\n\n- A thing.", body, 1)
	if p := Check(withBody); len(p) != 0 {
		t.Fatalf("well formed: %v", p)
	}
	if got, err := Section(withBody, "0.2.0"); err != nil || got != body {
		t.Fatalf("the whole section:\n%s\n%v", got, err)
	}
}
