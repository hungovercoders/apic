// Command episodes builds the course's YouTube episodes, bar the voice.
// Each lesson's shot list in docs/learn/episodes/NN.yaml names the cards,
// the terminal clips (a VHS tape each, recorded against the demo API from
// a scratch directory holding the files the lesson has created by then)
// and the push-ins, with the lines the voice says over each shot. Those
// lines set the timing: a shot runs as long as they take to say, or as
// long as its clip, whichever is longer, so a voice recorded to the
// captions fits the cut. For each episode it writes, under dist/episodes:
// the captioned MP4 with a silent track where the voice goes, a clean MP4
// without captions, the captions as SRT, the YouTube title, chapters and
// description (from docs/learn/_episode-template.md), the narration
// script and the thumbnail. Run it with `task episodes` (vhs, ttyd,
// ffmpeg and a chromium on PATH); `task episodes:check` validates every
// shot list and tape with vhs alone, recording nothing.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hungovercoders/apic/internal/demoapi"
	"github.com/hungovercoders/apic/internal/lessons"
	"github.com/hungovercoders/apic/scripts/internal/record"
)

func main() {
	check := flag.Bool("check", false, "validate every episode's shot list and tapes with vhs and build nothing")
	only := flag.String("only", "", "comma-separated episode names to build, e.g. 01,02 (default: all)")
	out := flag.String("out", "dist/episodes", "directory to write each episode into, under its name")
	keep := flag.Bool("keep", false, "keep each episode's scratch directory for inspection")
	version := flag.String("version", "dev", "version to stamp into the apic binary the episodes show")
	flag.Parse()
	all, err := LoadAll(".")
	if err != nil {
		fail(err)
	}
	eps, err := record.Pick(all, func(e *Episode) string { return e.Name }, *only)
	if err != nil {
		fail(err)
	}
	if *check {
		if n := Check(eps, os.Stdout); n > 0 {
			fail(fmt.Errorf("%d of %d episodes are invalid", n, len(eps)))
		}
		return
	}
	if err := Build(".", all, eps, *out, *version, *keep); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "episodes:", err)
	os.Exit(1)
}

// Check validates the shot lists (Load already did) and every clip's tape
// with vhs, and returns how many episodes failed.
func Check(eps []*Episode, w io.Writer) int {
	if _, err := exec.LookPath("vhs"); err != nil {
		fmt.Fprintln(w, vhsHint)
		return len(eps)
	}
	dir, err := os.MkdirTemp("", "apic-episodes-check-*")
	if err != nil {
		fmt.Fprintln(w, err)
		return len(eps)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	failed := 0
	for _, ep := range eps {
		ok := true
		for i, s := range ep.Shots {
			if s.Kind() != "clip" {
				continue
			}
			tape := filepath.Join(dir, fmt.Sprintf("clip-%02d-%s.tape", i, s.Clip))
			if err := writeTape(tape, s); err != nil {
				fmt.Fprintln(w, err)
				ok = false
				continue
			}
			cmd := exec.Command("vhs", "validate", tape) //nolint:gosec // vhs on a tape this process wrote
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Fprintf(w, "%s: clip %s: %v\n%s", ep.Name, s.Clip, err, out)
				ok = false
			}
		}
		if ok {
			fmt.Fprintf(w, "%s: ok (%d shots)\n", ep.Name, len(ep.Shots))
		} else {
			failed++
		}
	}
	return failed
}

// writeTape writes a clip's tape: the shared preamble, then its body.
func writeTape(path string, s Shot) error {
	w, h := termW, termH
	if s.Wide {
		w, h = wideW, wideH
	}
	mp4 := strings.TrimSuffix(filepath.Base(path), ".tape") + ".mp4"
	body := fmt.Sprintf(tapeHead, mp4, w, h) + strings.TrimSpace(s.Tape) + "\n"
	return os.WriteFile(path, []byte(body), 0o600)
}

const vhsHint = "vhs is not on PATH; checking needs vhs, building vhs, ttyd and ffmpeg (https://github.com/charmbracelet/vhs#installation) and a chromium"

// Build makes the picked episodes, out of all of them (the description
// links each to the one before): one apic binary, then each from its own
// scratch directory into out/NN.
func Build(root string, all, eps []*Episode, out, version string, keep bool) error {
	for _, tool := range []string{"vhs", "ttyd", "ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s is not on PATH; building needs vhs, ttyd and ffmpeg (https://github.com/charmbracelet/vhs#installation) and a chromium", tool)
		}
	}
	chromium, err := findChromium()
	if err != nil {
		return err
	}
	bin, err := os.MkdirTemp("", "apic-episodes-bin-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(bin) }()
	apic := filepath.Join(bin, "apic")
	ldflags := "-s -w -X github.com/hungovercoders/apic/internal/cli.Version=" + version
	build := exec.Command("go", "build", "-ldflags", ldflags, "-o", apic, "./cmd/apic") //nolint:gosec // the output path is the scratch directory this process just made
	build.Dir, build.Stderr = root, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("building apic: %w", err)
	}
	for _, ep := range eps {
		var prev *Episode
		for _, e := range all {
			if e.Lesson == ep.Lesson-1 {
				prev = e
			}
		}
		if err := buildOne(root, bin, chromium, ep, prev, filepath.Join(out, ep.Name), keep); err != nil {
			return fmt.Errorf("%s: %w", ep.Name, err)
		}
	}
	return nil
}

// build is one episode in progress: where it is made and written, the
// demo API once it is up, and the cut so far.
type build struct {
	root, bin, chromium, work, out string
	ep                             *Episode
	blocks                         []lessons.Block // the lesson's runnable blocks, for the setups
	env                            []string
	stop                           func()
	segments                       []string
	cues                           []Cue
	chapters                       []Chapter
	t                              float64
	lastClip                       string            // the newest clip's segment, for a zoom
	clipSeg                        map[string]string // each clip's segment, for the thumbnail
}

// buildOne records, renders and cuts one episode.
func buildOne(root, bin, chromium string, ep, prev *Episode, out string, keep bool) error {
	work, err := os.MkdirTemp("", "apic-episodes-"+ep.Name+"-*")
	if err != nil {
		return err
	}
	if !keep {
		defer func() { _ = os.RemoveAll(work) }()
	}
	for _, d := range []string{filepath.Join(work, "home"), filepath.Join(work, "cards"), out} {
		if err := os.MkdirAll(d, 0o755); err != nil { //nolint:gosec // the scratch directory and the output directory under dist
			return err
		}
	}
	blocks, err := lessons.Blocks(root, ep.Name)
	if err != nil {
		return err
	}
	b := &build{root: root, bin: bin, chromium: chromium, work: work, out: out, ep: ep, blocks: blocks, env: append(record.Environment(bin, work), goCaches()...), clipSeg: map[string]string{}}
	defer func() {
		if b.stop != nil {
			b.stop()
		}
	}()
	if ep.Project {
		if _, _, err := demoapi.WriteProject(filepath.Join(work, "apic-demo"), record.Port, true); err != nil {
			return err
		}
		if b.stop, err = record.Serve(); err != nil {
			return err
		}
	}
	if err := b.renderCards(); err != nil {
		return err
	}
	for i, s := range ep.Shots {
		if err := b.shot(i, s); err != nil {
			return err
		}
	}
	if err := b.write(prev); err != nil {
		return err
	}
	fmt.Printf("%s: wrote %s (%s)\n", ep.Name, out, ytTime(b.t))
	if keep {
		fmt.Printf("%s: scratch directory kept at %s\n", ep.Name, work)
	}
	return nil
}

// renderCards draws every card shot to a PNG under the scratch directory.
func (b *build) renderCards() error {
	for i, s := range b.ep.Shots {
		if s.Kind() != "card" {
			continue
		}
		html := filepath.Join(b.work, "cards", fmt.Sprintf("%02d.html", i))
		if err := renderCard(s.Card, html); err != nil {
			return err
		}
		if err := screenshot(b.chromium, html, filepath.Join(b.work, "cards", fmt.Sprintf("%02d.png", i)), canvasW, canvasH); err != nil {
			return err
		}
	}
	fmt.Printf("%s: cards rendered\n", b.ep.Name)
	return nil
}

// shot makes the segment for one shot and adds it to the cut: its
// captions spread over its length, and a chapter when the chapter changes.
func (b *build) shot(i int, s Shot) error {
	seg := filepath.Join(b.work, fmt.Sprintf("seg-%02d.mp4", i))
	var want float64
	for _, l := range s.Say {
		want += sayTime(l)
	}
	var err error
	switch s.Kind() {
	case "clip":
		err = b.recordClip(i, s, seg, want)
	case "card":
		err = composeCard(filepath.Join(b.work, "cards", fmt.Sprintf("%02d.png", i)), seg, want)
	case "zoom":
		frame := filepath.Join(b.work, fmt.Sprintf("frame-%02d.png", i))
		if err = lastFrame(b.lastClip, frame); err == nil {
			err = composeZoom(frame, seg, want)
		}
	}
	if err != nil {
		return err
	}
	secs, err := duration(seg)
	if err != nil {
		return err
	}
	b.cues = append(b.cues, spread(b.t, secs, s.Say)...)
	if len(b.chapters) == 0 || b.chapters[len(b.chapters)-1].Title != s.Chapter {
		b.chapters = append(b.chapters, Chapter{Start: b.t, Title: s.Chapter})
	}
	b.segments = append(b.segments, seg)
	b.t += secs
	fmt.Printf("%s: %02d %-5s %6.1fs -> %s\n", b.ep.Name, i, s.Kind(), secs, ytTime(b.t))
	return nil
}

// recordClip runs a clip's setup, records its tape with vhs and puts the
// recording on the canvas, held to the length of its narration.
func (b *build) recordClip(i int, s Shot, seg string, want float64) error {
	if err := b.setup(s); err != nil {
		return err
	}
	// An episode that does not start with the project has a clip that
	// runs `apic demo` on camera; vhs ends that server with the shell, so
	// from the next clip on this process serves the API.
	if _, err := os.Stat(filepath.Join(b.work, "apic-demo")); err == nil && b.stop == nil {
		if b.stop, err = record.Serve(); err != nil {
			return err
		}
	}
	tape := filepath.Join(b.work, fmt.Sprintf("clip-%02d-%s.tape", i, s.Clip))
	if err := writeTape(tape, s); err != nil {
		return err
	}
	fmt.Printf("%s: recording %s\n", b.ep.Name, filepath.Base(tape))
	vhs := exec.Command("vhs", filepath.Base(tape)) //nolint:gosec // vhs on a tape this process wrote
	vhs.Dir, vhs.Env, vhs.Stderr = b.work, b.env, os.Stderr
	if err := vhs.Run(); err != nil {
		return fmt.Errorf("clip %s: vhs: %w", s.Clip, err)
	}
	mp4 := strings.TrimSuffix(tape, ".tape") + ".mp4"
	clipLen, err := duration(mp4)
	if err != nil {
		return err
	}
	if err := composeClip(mp4, seg, max(0, want-clipLen), s.Wide); err != nil {
		return err
	}
	b.lastClip, b.clipSeg[s.Clip] = seg, seg
	return nil
}

// goCaches keeps the recording machine's Go module and build caches in
// force under the recording's own HOME, so a clip that builds or tests
// apic in a clone (lesson 13) neither downloads modules, which the
// recording's environment has no proxy for, nor compiles from cold.
func goCaches() []string {
	out, err := exec.Command("go", "env", "GOMODCACHE", "GOCACHE").Output()
	if err != nil {
		return nil
	}
	vals := strings.Fields(string(out))
	if len(vals) != 2 {
		return nil
	}
	return []string{"GOMODCACHE=" + vals[0], "GOCACHE=" + vals[1]}
}

// setup runs a clip's setup steps in the scratch directory, off camera.
// A step sees the repository root as EPISODE_REPO, for a lesson that
// works on the example projects rather than the demo; the clips do not.
func (b *build) setup(s Shot) error {
	root, err := filepath.Abs(b.root)
	if err != nil {
		return err
	}
	env := append(append([]string(nil), b.env...), "EPISODE_REPO="+root)
	for _, st := range s.Setup {
		code, err := st.step(b.ep.Name).Code(b.blocks)
		if err != nil {
			return err
		}
		if err := record.Shell(b.work, env, code); err != nil {
			return fmt.Errorf("clip %s: %w", s.Clip, err)
		}
	}
	return nil
}

// write joins the segments into the clean and captioned cuts and writes
// everything else the episode ships with.
func (b *build) write(prev *Episode) error {
	ep := b.ep
	clean := filepath.Join(b.out, ep.Name+"-clean.mp4")
	if err := concat(b.segments, filepath.Join(b.work, "segments.txt"), filepath.Join(b.work, "all.mp4")); err != nil {
		return err
	}
	if err := withSilence(filepath.Join(b.work, "all.mp4"), clean); err != nil {
		return err
	}
	ass := filepath.Join(b.work, "captions.ass")
	if err := os.WriteFile(ass, []byte(ASS(b.cues)), 0o600); err != nil {
		return err
	}
	if err := burn(clean, ass, filepath.Join(b.out, ep.Name+".mp4")); err != nil {
		return err
	}
	shot := filepath.Join(b.work, "shot.png")
	if err := cropTerm(b.clipSeg[ep.Thumbnail.Clip], shot); err != nil {
		return err
	}
	if err := renderThumb(ep, shot, filepath.Join(b.work, "thumb.html")); err != nil {
		return err
	}
	if err := screenshot(b.chromium, filepath.Join(b.work, "thumb.html"), filepath.Join(b.out, ep.Name+"-thumbnail.png"), 1280, 720); err != nil {
		return err
	}
	description, err := ep.Description(b.root, b.chapters, prev)
	if err != nil {
		return err
	}
	var ch strings.Builder
	for _, c := range b.chapters {
		fmt.Fprintf(&ch, "%s %s\n", ytTime(c.Start), c.Title)
	}
	for name, text := range map[string]string{
		".srt":            SRT(b.cues),
		"-title.txt":      ep.VideoTitle() + "\n",
		"-chapters.txt":   ch.String(),
		"-description.md": description,
		"-narration.md":   ep.Narration(b.cues, b.chapters, b.t),
	} {
		if err := os.WriteFile(filepath.Join(b.out, ep.Name+name), []byte(text), 0o644); err != nil { //nolint:gosec // outputs under dist, meant to be read
			return err
		}
	}
	return nil
}
