package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hungovercoders/apic/internal/lessons"
)

const root = "../.."

// Every setup entry names a tape that exists, and lesson blocks the page
// really has; a substitution names text that appears in those blocks.
func TestSetupNamesRealTapesAndBlocks(t *testing.T) {
	for name, steps := range setup {
		if _, err := os.Stat(filepath.Join(root, TapesDir, name+".tape")); err != nil {
			t.Errorf("setup[%q]: no such tape: %v", name, err)
		}
		for _, s := range steps {
			if s.Sh != "" {
				continue
			}
			blocks, err := lessons.Blocks(root, s.Lesson)
			if err != nil {
				t.Errorf("setup[%q]: %v", name, err)
				continue
			}
			code, err := s.Code(blocks)
			if err != nil {
				t.Errorf("setup[%q]: %v", name, err)
				continue
			}
			if s.Replace[0] != "" && strings.Contains(code, s.Replace[0]) {
				t.Errorf("setup[%q]: the substitution of %q left some behind", name, s.Replace[0])
			}
		}
	}
}

// Every numbered lesson has a tape, or its episode script says there is
// none and why, so a lesson cannot be left without a recording by
// accident.
func TestEveryLessonHasATapeOrSaysWhyNot(t *testing.T) {
	pages, err := filepath.Glob(filepath.Join(root, lessons.Dir, "[0-9][0-9]-*.md"))
	if err != nil || len(pages) == 0 {
		t.Fatalf("no lesson pages: %v", err)
	}
	for _, p := range pages {
		n := filepath.Base(p)[:2]
		if _, err := os.Stat(filepath.Join(root, TapesDir, n+".tape")); err == nil {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "No tape for this lesson") {
			t.Errorf("%s has no tape and does not say why; add %s/%s.tape or \"No tape for this lesson\" to its episode script", filepath.Base(p), TapesDir, n)
		}
	}
}

// A tape's Output is where the recorder expects to pick the GIF up.
func TestEveryTapeWritesUnderDocs(t *testing.T) {
	tapes, err := Find(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(tapes) < 5 {
		t.Fatalf("found only %d tapes", len(tapes))
	}
	re := regexp.MustCompile(`(?m)^Output (docs/\S+\.gif)$`)
	for _, tp := range tapes {
		data, err := os.ReadFile(tp.Path)
		if err != nil {
			t.Fatal(err)
		}
		if !re.Match(data) {
			t.Errorf("%s: no `Output docs/….gif` line", tp.Path)
		}
	}
}

// With vhs on PATH, every tape parses; CI installs it for this. VHS strings
// take no backslash escapes, so a command that needs both quote kinds goes
// in backticks: this is the test that catches the alternative.
func TestEveryTapeIsValid(t *testing.T) {
	if _, err := exec.LookPath("vhs"); err != nil {
		t.Skip("vhs is not on PATH")
	}
	tapes, err := Find(root)
	if err != nil {
		t.Fatal(err)
	}
	if failed := Validate(tapes, os.Stdout); failed > 0 {
		t.Errorf("%d tapes are invalid", failed)
	}
}
