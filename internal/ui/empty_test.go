package ui

import (
	"strings"
	"testing"

	"github.com/hungovercoders/apic/internal/output"
	"github.com/hungovercoders/apic/internal/project"
	"github.com/hungovercoders/apic/internal/runner"
	"github.com/hungovercoders/apic/internal/session"
)

// A directory with no request files opens, and the list says what would
// give it some rather than only that nothing matches.
func TestEmptyProjectPointsAtInitAndDemo(t *testing.T) {
	dir := t.TempDir()
	p, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	r, err := runner.New(p, runner.Options{Session: session.NewMemory()})
	if err != nil {
		t.Fatal(err)
	}
	m := New(Config{Root: dir, Runner: r, Theme: output.Default()})
	m.Update(sizeMsg{width: 120, height: 40})
	v := m.View()
	for _, want := range []string{"no .http files here", "apic init scaffolds a project", "apic ui --demo shows a working one"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q in:\n%s", want, v)
		}
	}
	if strings.Contains(v, "no requests match") {
		t.Errorf("an empty project is not a filter with no matches:\n%s", v)
	}
}
