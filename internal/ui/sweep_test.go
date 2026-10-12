package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/hungovercoders/apic/internal/runner"
)

// sweepSizes are the terminals the sweep draws at: the minimum, one more
// than it in each direction, the common ones, and a big one. Small sizes
// are where a pane overflows; big ones are where padding goes wrong.
var sweepSizes = [][2]int{{minWidth, minHeight}, {minWidth + 1, minHeight + 1}, {80, 24}, {100, 30}, {140, 45}, {220, 70}}

// TestEveryPaneFitsAtEverySize visits every request, every tab with and
// without headers, every code snippet language, the help overlay and the
// filter, at each size in sweepSizes, before any run and after running the
// whole project, and fails on a frame with the wrong number of lines or a
// line wider than the terminal. A scenario test checks one screen; this
// checks that no screen has quietly stopped fitting.
func TestEveryPaneFitsAtEverySize(t *testing.T) {
	f := newFixture(t, runner.Options{})
	frames := 0
	check := func(what string, w, h int) {
		t.Helper()
		frames++
		v := f.view()
		lines := strings.Split(v, "\n")
		if len(lines) != h {
			t.Errorf("%dx%d %s: %d lines, want %d\n%s", w, h, what, len(lines), h, v)
			return
		}
		for i, l := range lines {
			if lw := ansi.StringWidth(l); lw > w {
				t.Errorf("%dx%d %s: line %d is %d wide\n%s", w, h, what, i+1, lw, l)
			}
		}
		if !strings.Contains(lines[len(lines)-1], "? help") {
			t.Errorf("%dx%d %s: the status bar lost its key hint:\n%s", w, h, what, lines[len(lines)-1])
		}
		if f.m.selected != f.m.selectedReq() {
			t.Errorf("%dx%d %s: the pane shows %v while the cursor is on %v", w, h, what, f.m.selected, f.m.selectedReq())
		}
	}
	requests := len(f.m.allRequests())
	if requests < 10 {
		t.Fatalf("the demo project has %d requests; the sweep expects more", requests)
	}
	for _, phase := range []string{"before any run", "after running everything"} {
		if phase == "after running everything" {
			f.drain(f.press("a"))
		}
		for _, sz := range sweepSizes {
			w, h := sz[0], sz[1]
			f.m.Resize(w, h)
			f.press("g")
			for i := 0; i < requests; i++ {
				id := f.m.selected.ID()
				for _, tab := range []string{"1", "2", "3", "4", "5"} {
					f.press(tab)
					check(phase+" "+id+" tab "+tab, w, h)
					f.press("H")
					check(phase+" "+id+" tab "+tab+" with headers", w, h)
					f.press("H")
				}
				f.press("1")
				for k := 0; k < 8; k++ {
					f.press("c")
					check(phase+" "+id+" code snippet", w, h)
				}
				f.press("j")
			}
			f.press("?")
			check(phase+" help", w, h)
			f.press("esc")
			f.press("/")
			f.press("t")
			check(phase+" filter", w, h)
			f.press("esc")
			f.press("esc")
		}
	}
	if frames < 1000 {
		t.Errorf("only %d frames checked; the sweep has lost its coverage", frames)
	}
	f.m.Resize(minWidth-1, minHeight)
	if !strings.Contains(f.view(), "terminal too small") {
		t.Errorf("one column under the minimum should refuse to draw:\n%s", f.view())
	}
}
