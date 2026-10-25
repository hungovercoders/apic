package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hungovercoders/apic/internal/lessons"
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
		blocks, err := lessons.Blocks(root, ep.Name)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range ep.Shots {
			for _, st := range s.Setup {
				code, err := st.step(ep.Name).Code(blocks)
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

// Validate catches an episode that does not open on its title card, a
// shot that is no kind or two, a zoom before any clip or after a wide one,
// a thumbnail naming no clip or a wide one, a clip name that is not a file
// name, and a caption the renderer would misread.
func TestValidate(t *testing.T) {
	good := func() *Episode {
		return &Episode{Lesson: 1, Name: "01", Summary: "s", Thumbnail: Thumbnail{Words: "w", Clip: "a"}, Shots: []Shot{
			{Chapter: "c", Say: []string{"hi"}, Card: &Card{Series: "s", Title: "t"}},
			{Chapter: "c", Say: []string{"hi"}, Clip: "a", Tape: `Type "x" Enter`},
			{Chapter: "c", Say: []string{"hi"}, Zoom: &Zoom{Col: 1, Row: 1}},
		}}
	}
	if err := good().Validate(); err != nil {
		t.Fatalf("a good episode fails: %v", err)
	}
	cases := map[string]func(*Episode){
		"no title first":   func(e *Episode) { e.Shots = e.Shots[1:] },
		"heading first":    func(e *Episode) { e.Shots[0].Card = &Card{Heading: "h"} },
		"no kind":          func(e *Episode) { e.Shots[1].Clip = "" },
		"two kinds":        func(e *Episode) { e.Shots[1].Card = &Card{Heading: "h"} },
		"zoom first":       func(e *Episode) { e.Shots = append(e.Shots[:1], e.Shots[2:]...) },
		"zoom off window":  func(e *Episode) { e.Shots[2].Zoom.Col = 500 },
		"thumbnail clip":   func(e *Episode) { e.Thumbnail.Clip = "nope" },
		"brace in caption": func(e *Episode) { e.Shots[1].Say = []string{"{{user}}"} },
		"name mismatch":    func(e *Episode) { e.Lesson = 2 },
		"empty tape":       func(e *Episode) { e.Shots[1].Tape = " " },
		"zoom with a tape": func(e *Episode) { e.Shots[2].Tape = `Type "x"` },
		"zoom after wide": func(e *Episode) {
			e.Shots[1].Wide = true
			e.Thumbnail.Clip = "b"
			e.Shots = append(e.Shots, Shot{Chapter: "c", Say: []string{"hi"}, Clip: "b", Tape: "Enter"})
		},
		"wide thumbnail": func(e *Episode) {
			e.Shots[1].Wide = true
			e.Shots[2] = Shot{Chapter: "c", Say: []string{"hi"}, Card: &Card{Heading: "h"}}
		},
		"clip name": func(e *Episode) { e.Shots[1].Clip = "a/b"; e.Thumbnail.Clip = "a/b" },
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
	lines := []string{"one two three four five", "six"}
	cues := spread(10, 20, lines, sayTimes(lines))
	if len(cues) != 2 || cues[0].Start != 10 || cues[1].End < 29.99 || cues[1].End > 30.01 {
		t.Errorf("spread did not fill the shot: %+v", cues)
	}
	// Recorded times: a 1s and a 3s clip over an 8s shot keep their ratio,
	// and each cue is at least as long as its clip.
	cues = spread(0, 8, lines, []float64{1 + voicePad, 3 + voicePad})
	if cues[0].End < 1 || cues[1].Start != cues[0].End || cues[1].End-cues[1].Start < 3 || cues[1].End < 7.99 || cues[1].End > 8.01 {
		t.Errorf("spread over recorded times: %+v", cues)
	}
}

// A voice directory is read by cue number, whatever the clip format, and a
// missing, duplicated or surplus clip is named with its line; files not
// named by a number are left alone, and no directory is no voice.
func TestLoadVoice(t *testing.T) {
	ep := &Episode{Name: "04", Shots: []Shot{
		{Say: []string{"first line", "second line"}},
		{Say: []string{"third line"}},
	}}
	dir := t.TempDir()
	probe = func(string) (float64, error) { return 1.5, nil }
	defer func() { probe = duration }()
	if v, err := LoadVoice(dir, ep); v != nil || err != nil {
		t.Fatalf("no directory should be no voice, got %+v, %v", v, err)
	}
	if v, err := LoadVoice("", ep); v != nil || err != nil {
		t.Fatalf("no -voice should be no voice, got %+v, %v", v, err)
	}
	epDir := filepath.Join(dir, "04")
	if err := os.MkdirAll(epDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(epDir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("001.wav")
	write("002.flac")
	write("notes.txt")
	_, err := LoadVoice(dir, ep)
	if err == nil || !strings.Contains(err.Error(), "no clip 003") || !strings.Contains(err.Error(), "third line") {
		t.Fatalf("a missing clip should name its cue and line, got %v", err)
	}
	write("003.aiff")
	v, err := LoadVoice(dir, ep)
	if err != nil {
		t.Fatal(err)
	}
	if total := v.Total(); len(v.Clips) != 3 || filepath.Base(v.Clips[1]) != "002.flac" || filepath.Base(v.Clips[2]) != "003.aiff" || v.Secs[2] != 1.5 || total < 6.59 || total > 6.61 {
		t.Errorf("voice: %+v (total %v)", v, total)
	}
	write("004.m4a")
	if _, err := LoadVoice(dir, ep); err == nil || !strings.Contains(err.Error(), "past the last line") {
		t.Errorf("a surplus clip should fail, got %v", err)
	}
	if err := os.Remove(filepath.Join(epDir, "004.m4a")); err != nil {
		t.Fatal(err)
	}
	write("2.wav")
	if _, err := LoadVoice(dir, ep); err == nil || !strings.Contains(err.Error(), "two clips for cue 002") {
		t.Errorf("two clips for one cue should fail, got %v", err)
	}
}

// The narration script numbers each line as the SRT does.
func TestNarrationNumbersCues(t *testing.T) {
	ep := &Episode{Lesson: 4, Name: "04", Title: "T"}
	cues := []Cue{{Start: 0, End: 2, Text: "one"}, {Start: 2, End: 4, Text: "two"}}
	got := ep.Narration(cues, []Chapter{{0, "Why"}}, 4)
	for _, want := range []string{"## 0:00 Why", "- **0:00** 001 · one", "- **0:02** 002 · two"} {
		if !strings.Contains(got, want) {
			t.Errorf("narration lacks %q:\n%s", want, got)
		}
	}
}

// With ffmpeg on hand, recorded lines land on a cut's audio track at
// their cues and the track runs the length of the picture; a one-take
// narration muxes the same way.
func TestVoiceTracks(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not on PATH", tool)
		}
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "cut.mp4")
	if err := ffmpeg("-f", "lavfi", "-i", "color=c=black:s=64x64:r=10", "-t", "3", "-pix_fmt", "yuv420p", video); err != nil {
		t.Fatal(err)
	}
	clip := func(name string, secs float64) string {
		p := filepath.Join(dir, name)
		if err := ffmpeg("-f", "lavfi", "-i", "sine=frequency=440:sample_rate=22050", "-t", fmt.Sprint(secs), p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cues := []Cue{{Start: 0.2, Text: "a", Audio: clip("001.wav", 0.5)}, {Start: 1.5, Text: "b", Audio: clip("002.wav", 0.5)}}
	voiced := filepath.Join(dir, "voiced.mp4")
	if err := withVoice(video, cues, voiced); err != nil {
		t.Fatal(err)
	}
	if secs, err := duration(voiced); err != nil || secs < 2.9 || secs > 3.2 {
		t.Errorf("the voiced cut runs %v (%v), want the picture's 3s", secs, err)
	}
	if n := audioStreams(t, voiced); n != 1 {
		t.Errorf("the voiced cut has %d audio streams, want 1", n)
	}
	muxed := filepath.Join(dir, "muxed.mp4")
	if err := mux(video, clip("take.wav", 1), muxed); err != nil {
		t.Fatal(err)
	}
	if secs, err := duration(muxed); err != nil || secs < 2.9 || secs > 3.2 {
		t.Errorf("the muxed cut runs %v (%v), want the picture's 3s, not the take's 1s", secs, err)
	}
}

func audioStreams(t *testing.T, path string) int {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "a", "-show_entries", "stream=index", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	return len(strings.Fields(string(out)))
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
