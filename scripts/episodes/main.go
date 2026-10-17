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
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hungovercoders/apic/internal/demoapi"
)

// demoPort is the port the lessons show in their URLs.
const demoPort = 8089

func main() {
	check := flag.Bool("check", false, "validate every episode's shot list and tapes with vhs and build nothing")
	only := flag.String("only", "", "comma-separated episode names to build, e.g. 01,02 (default: all)")
	out := flag.String("out", "dist/episodes", "directory to write each episode into, under its name")
	keep := flag.Bool("keep", false, "keep each episode's scratch directory for inspection")
	version := flag.String("version", "dev", "version to stamp into the apic binary the episodes show")
	flag.Parse()
	eps, err := LoadAll(".")
	if err == nil {
		eps, err = pick(eps, *only)
	}
	if err != nil {
		fail(err)
	}
	if *check {
		if n := Check(eps, os.Stdout); n > 0 {
			fail(fmt.Errorf("%d of %d episodes are invalid", n, len(eps)))
		}
		return
	}
	if err := Build(".", eps, *out, *version, *keep); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "episodes:", err)
	os.Exit(1)
}

// pick keeps the episodes named in only, in the order found.
func pick(eps []*Episode, only string) ([]*Episode, error) {
	if only == "" {
		return eps, nil
	}
	want := map[string]bool{}
	for _, n := range strings.Split(only, ",") {
		want[strings.TrimSpace(n)] = true
	}
	var picked []*Episode
	for _, e := range eps {
		if want[e.Name] {
			picked = append(picked, e)
			delete(want, e.Name)
		}
	}
	for n := range want {
		return nil, fmt.Errorf("no episode %s in %s", n, EpisodesDir)
	}
	return picked, nil
}

// Check validates the shot lists (Load already did) and every clip's tape
// with vhs, and returns how many episodes failed.
func Check(eps []*Episode, w io.Writer) int {
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
	w := termW
	if s.Wide {
		w = wideW
	}
	mp4 := strings.TrimSuffix(filepath.Base(path), ".tape") + ".mp4"
	body := fmt.Sprintf(tapeHead, mp4, w, termH) + strings.TrimSpace(s.Tape) + "\n"
	return os.WriteFile(path, []byte(body), 0o600)
}

// Build makes every episode: one apic binary, then each from its own
// scratch directory into out/NN.
func Build(root string, eps []*Episode, out, version string, keep bool) error {
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
	all, err := LoadAll(root)
	if err != nil {
		return err
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

// buildOne records, renders and cuts one episode.
func buildOne(root, bin, chromium string, ep, prev *Episode, out string, keep bool) error {
	work, err := os.MkdirTemp("", "apic-episodes-"+ep.Name+"-*")
	if err != nil {
		return err
	}
	if !keep {
		defer func() { _ = os.RemoveAll(work) }()
	}
	for _, d := range []string{"home", "cards"} {
		if err := os.MkdirAll(filepath.Join(work, d), 0o755); err != nil { //nolint:gosec // scratch directory
			return err
		}
	}
	if err := os.MkdirAll(out, 0o755); err != nil { //nolint:gosec // the output directory, under dist
		return err
	}
	var stop func()
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	if ep.Project {
		if _, _, err := demoapi.WriteProject(filepath.Join(work, "apic-demo"), demoPort, true); err != nil {
			return err
		}
		if stop, err = serve(); err != nil {
			return err
		}
	}
	env := environment(bin, work)

	for i, s := range ep.Shots {
		if s.Kind() != "card" {
			continue
		}
		html := filepath.Join(work, "cards", fmt.Sprintf("%02d.html", i))
		if err := renderCard(s.Card, html); err != nil {
			return err
		}
		if err := screenshot(chromium, html, filepath.Join(work, "cards", fmt.Sprintf("%02d.png", i)), canvasW, canvasH); err != nil {
			return err
		}
	}
	fmt.Printf("%s: cards rendered\n", ep.Name)

	var (
		segments []string
		cues     []Cue
		chapters []Chapter
		t        float64
		lastClip string
		clipSeg  = map[string]string{}
	)
	for i, s := range ep.Shots {
		seg := filepath.Join(work, fmt.Sprintf("seg-%02d.mp4", i))
		var want float64
		for _, l := range s.Say {
			want += sayTime(l)
		}
		switch s.Kind() {
		case "clip":
			for _, st := range s.Setup {
				code, err := st.code(root, ep.Name)
				if err != nil {
					return err
				}
				cmd := exec.Command("sh", "-e") //nolint:gosec // the course's own commands, as learncheck runs them
				cmd.Dir, cmd.Env, cmd.Stdin = work, append(env, "NO_COLOR=1"), strings.NewReader(code)
				if o, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("clip %s: setup failed: %w\n%s", s.Clip, err, o)
				}
			}
			// An episode that does not start with the project has a clip
			// that runs `apic demo` on camera; vhs ends that server with the
			// shell, so from the next clip on this process serves the API.
			if stop == nil {
				if _, err := os.Stat(filepath.Join(work, "apic-demo")); err == nil {
					if stop, err = serve(); err != nil {
						return err
					}
				}
			}
			tape := filepath.Join(work, fmt.Sprintf("clip-%02d-%s.tape", i, s.Clip))
			if err := writeTape(tape, s); err != nil {
				return err
			}
			fmt.Printf("%s: recording %s\n", ep.Name, filepath.Base(tape))
			vhs := exec.Command("vhs", filepath.Base(tape)) //nolint:gosec // vhs on a tape this process wrote
			vhs.Dir, vhs.Env, vhs.Stderr = work, env, os.Stderr
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
			lastClip, clipSeg[s.Clip] = seg, seg
		case "card":
			if err := composeCard(filepath.Join(work, "cards", fmt.Sprintf("%02d.png", i)), seg, want); err != nil {
				return err
			}
		case "zoom":
			frame := filepath.Join(work, fmt.Sprintf("frame-%02d.png", i))
			if err := lastFrame(lastClip, frame); err != nil {
				return err
			}
			if err := composeZoom(frame, seg, want); err != nil {
				return err
			}
		}
		secs, err := duration(seg)
		if err != nil {
			return err
		}
		cues = append(cues, spread(t, secs, s.Say)...)
		if len(chapters) == 0 || chapters[len(chapters)-1].Title != s.Chapter {
			chapters = append(chapters, Chapter{Start: t, Title: s.Chapter})
		}
		segments = append(segments, seg)
		t += secs
		fmt.Printf("%s: %02d %-5s %6.1fs -> %s\n", ep.Name, i, s.Kind(), secs, ytTime(t))
	}

	clean := filepath.Join(out, ep.Name+"-clean.mp4")
	if err := concat(segments, filepath.Join(work, "segments.txt"), filepath.Join(work, "all.mp4")); err != nil {
		return err
	}
	if err := withSilence(filepath.Join(work, "all.mp4"), clean); err != nil {
		return err
	}
	ass := filepath.Join(work, "captions.ass")
	if err := os.WriteFile(ass, []byte(ASS(cues)), 0o600); err != nil {
		return err
	}
	if err := burn(clean, ass, filepath.Join(out, ep.Name+".mp4")); err != nil {
		return err
	}
	shot := filepath.Join(work, "shot.png")
	if err := cropTerm(clipSeg[ep.Thumbnail.Clip], shot); err != nil {
		return err
	}
	if err := renderThumb(ep, shot, filepath.Join(work, "thumb.html")); err != nil {
		return err
	}
	if err := screenshot(chromium, filepath.Join(work, "thumb.html"), filepath.Join(out, ep.Name+"-thumbnail.png"), 1280, 720); err != nil {
		return err
	}
	description, err := ep.Description(root, chapters, prev)
	if err != nil {
		return err
	}
	var ch strings.Builder
	for _, c := range chapters {
		fmt.Fprintf(&ch, "%s %s\n", ytTime(c.Start), c.Title)
	}
	for name, text := range map[string]string{
		".srt":            SRT(cues),
		"-title.txt":      ep.VideoTitle() + "\n",
		"-chapters.txt":   ch.String(),
		"-description.md": description,
		"-narration.md":   ep.Narration(cues, chapters, t),
	} {
		if err := os.WriteFile(filepath.Join(out, ep.Name+name), []byte(text), 0o644); err != nil { //nolint:gosec // outputs under dist, meant to be read
			return err
		}
	}
	fmt.Printf("%s: wrote %s (%s)\n", ep.Name, out, ytTime(t))
	if keep {
		fmt.Printf("%s: scratch directory kept at %s\n", ep.Name, work)
	}
	return nil
}

// environment is what the setup steps and vhs run under: the fresh apic
// first on PATH, a HOME of their own, UTC and a UTF-8 locale, and no
// NO_COLOR from the recording machine, since the picture should have
// colour.
func environment(bin, work string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "PATH", "HOME", "TZ", "LANG", "LC_ALL", "NO_COLOR", "APIC_DEMO_URL":
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+filepath.Join(work, "home"),
		"TZ=UTC",
		"LANG=C.UTF-8",
		"APIC_DEMO_URL=http://localhost:"+strconv.Itoa(demoPort),
	)
}

// serve starts the demo API on demoPort, waiting a few seconds for the
// port when a clip's own `apic demo` is still letting go of it.
func serve() (stop func(), err error) {
	var ln net.Listener
	for deadline := time.Now().Add(5 * time.Second); ; {
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", demoPort))
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("the lessons show port %d, which is in use: %w", demoPort, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	srv := &http.Server{Handler: demoapi.New(), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}, nil
}
