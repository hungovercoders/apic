package skills

import (
	"os"
	"slices"
	"sort"
	"testing"
)

// The go:embed directive names each skill by hand, so a directory added
// under skills/ without a line there would be missing from the binary
// with nothing else to notice: the pack the binary carries must be the
// directories on disk that hold a SKILL.md.
func TestPackIsEveryDirectoryOnDisk(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var onDisk []string
	for _, e := range entries {
		if _, err := os.Stat(e.Name() + "/SKILL.md"); e.IsDir() && err == nil {
			onDisk = append(onDisk, e.Name())
		}
	}
	sort.Strings(onDisk)
	got := Names()
	sort.Strings(got)
	if !slices.Equal(got, onDisk) {
		t.Fatalf("the binary carries %v but skills/ holds %v: add the new directory to the go:embed line in embed.go", got, onDisk)
	}
	for _, name := range Names() {
		if paths := Paths(name); len(paths) < 2 || paths[0] != "SKILL.md" {
			t.Errorf("%s: %v; a skill is SKILL.md and at least one reference", name, paths)
		}
	}
}
