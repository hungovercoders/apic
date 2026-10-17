package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const root = "../.."

// Every episode loads and validates against the lesson page it is for;
// Load runs Validate, so this is the check CI gets without vhs.
func TestEpisodesLoad(t *testing.T) {
	eps, err := LoadAll(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) == 0 {
		t.Fatalf("no episodes in %s", EpisodesDir)
	}
	for _, ep := range eps {
		if ep.Title == "" || ep.Slug == "" {
			t.Errorf("%s: no title or slug from the lesson page", ep.Name)
		}
	}
}

// Every setup step's lesson blocks exist, and its substitution names text
// that is in them.
func TestSetupNamesRealBlocks(t *testing.T) {
	eps, err := LoadAll(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, ep := range eps {
		for _, s := range ep.Shots {
			for _, st := range s.Setup {
				code, err := st.code(root, ep.Name)
				if err != nil {
					t.Errorf("%s: clip %s: %v", ep.Name, s.Clip, err)
					continue
				}
				if len(st.Replace) == 2 && strings.Contains(code, st.Replace[0]) {
					t.Errorf("%s: clip %s: the substitution of %q left some behind", ep.Name, s.Clip, st.Replace[0])
				}
			}
		}
	}
}

// Validate catches a shot that is no kind or two, a zoom before any clip,
// a thumbnail naming no clip, and a caption the renderer would misread.
func TestValidate(t *testing.T) {
	good := func() *Episode {
		return &Episode{Lesson: 1, Name: "01", Summary: "s", Thumbnail: Thumbnail{Words: "w", Clip: "a"}, Shots: []Shot{
			{Chapter: "c", Say: []string{"hi"}, Clip: "a", Tape: `Type "x" Enter`},
			{Chapter: "c", Say: []string{"hi"}, Zoom: &Zoom{Col: 1, Row: 1}},
		}}
	}
	if err := good().Validate(); err != nil {
		t.Fatalf("a good episode fails: %v", err)
	}
	cases := map[string]func(*Episode){
		"no kind":          func(e *Episode) { e.Shots[0].Clip = "" },
		"two kinds":        func(e *Episode) { e.Shots[0].Card = &Card{Heading: "h"} },
		"zoom first":       func(e *Episode) { e.Shots = e.Shots[1:] },
		"zoom off window":  func(e *Episode) { e.Shots[1].Zoom.Col = 500 },
		"thumbnail clip":   func(e *Episode) { e.Thumbnail.Clip = "nope" },
		"brace in caption": func(e *Episode) { e.Shots[0].Say = []string{"{{user}}"} },
		"name mismatch":    func(e *Episode) { e.Lesson = 2 },
		"empty tape":       func(e *Episode) { e.Shots[0].Tape = " " },
	}
	for name, breakIt := range cases {
		e := good()
		breakIt(e)
		if err := e.Validate(); err == nil {
			t.Errorf("%s: Validate passed", name)
		}
	}
}

// The timing formats, and that a shot never runs shorter than its lines.
func TestTimes(t *testing.T) {
	for _, c := range []struct {
		t        float64
		srt, ass string
		yt       string
	}{
		{0, "00:00:00,000", "0:00:00.00", "0:00"},
		{61.5, "00:01:01,500", "0:01:01.50", "1:01"},
		{3599.999, "00:59:59,999", "1:00:00.00", "59:59"},
	} {
		if got := srtTime(c.t); got != c.srt {
			t.Errorf("srtTime(%v) = %q, want %q", c.t, got, c.srt)
		}
		if got := assTime(c.t); got != c.ass {
			t.Errorf("assTime(%v) = %q, want %q", c.t, got, c.ass)
		}
		if got := ytTime(c.t); got != c.yt {
			t.Errorf("ytTime(%v) = %q, want %q", c.t, got, c.yt)
		}
	}
	if sayTime("Now send it.") != 2.4 {
		t.Errorf("a short line should take the minimum, got %v", sayTime("Now send it."))
	}
	cues := spread(10, 20, []string{"one two three four five", "six"})
	if len(cues) != 2 || cues[0].Start != 10 || cues[1].End < 29.99 || cues[1].End > 30.01 {
		t.Errorf("spread did not fill the shot: %+v", cues)
	}
}

// The description fills every placeholder of the template, and the
// template still has the block the generator fills.
func TestDescription(t *testing.T) {
	eps, err := LoadAll(root)
	if err != nil {
		t.Fatal(err)
	}
	var prev *Episode
	for _, ep := range eps {
		d, err := ep.Description(root, []Chapter{{0, "Why"}, {30.2, "Install"}}, prev)
		if err != nil {
			t.Fatalf("%s: %v", ep.Name, err)
		}
		for _, want := range []string{ep.Summary, "https://apic.sh/learn/" + ep.Slug + "/", "0:00 Why\n0:30 Install\n", "lesson " + strings.TrimLeft(ep.Name, "0") + " of"} {
			if !strings.Contains(d, want) {
				t.Errorf("%s: description lacks %q:\n%s", ep.Name, want, d)
			}
		}
		prev = ep
	}
}

// A card renders to HTML with its text escaped and its backticks turned
// into highlights, and nothing else.
func TestRenderCard(t *testing.T) {
	html := filepath.Join(t.TempDir(), "c.html")
	if err := renderCard(&Card{Heading: "h <b>", Text: "$ `apic run` <x>", Foot: "see `apic.sh`"}, html); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(html)
	for _, want := range []string{"h &lt;b&gt;", `<span class="dim">$</span> <code class="hl">apic run</code> &lt;x&gt;`, `<code class="hl">apic.sh</code>`} {
		if !strings.Contains(string(got), want) {
			t.Errorf("rendered card lacks %q", want)
		}
	}
}
