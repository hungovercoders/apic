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
