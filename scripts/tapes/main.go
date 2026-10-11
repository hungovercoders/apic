// Command tapes records the course's VHS tapes the same way every time. It
// builds apic, serves the demo API on the port the lessons show, gives each
// tape a scratch directory with a fresh apic-demo project and the files its
// lesson's runnable blocks create (internal/lessons), and runs vhs there
// with a fixed HOME and time zone, so two recordings of a tape differ only
// in timings. Run it with `task tapes` (vhs, ttyd and ffmpeg on PATH);
// `task tapes:check` validates every tape's syntax with vhs alone, which
// CI does, so a tape that only a person has run cannot quietly break.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hungovercoders/apic/internal/demoapi"
	"github.com/hungovercoders/apic/internal/lessons"
)

// demoPort is the port the lessons show in their URLs; a tape's picture
// carries it, so it is not negotiable the way learncheck's is.
const demoPort = 8089

// Where the tapes and the course live, relative to the repository root.
const (
	TapesDir = "docs/learn/tapes"
	DemoTape = "docs/assets/demo.tape"
	LearnDir = "website/src/content/docs/learn"
)

// Tape is one VHS script: its name (01, 05, demo) and its path.
type Tape struct {
	Name string
	Path string
}

// step is one piece of a tape's setup: a shell snippet, or runnable blocks
// of a lesson page (1-based, in page order), optionally with a text
// substitution applied to them first.
type step struct {
	sh      string
	lesson  string // the page's number prefix, "09"
	blocks  []int
	replace [2]string
}

// setup is what each tape expects to find beyond the demo project: the
// state the lesson has built up by the time its terminal parts begin. A
// tape not listed needs only apic-demo and the running API.
var setup = map[string][]step{
	// the staging environment and the private env file
	"02": {{lesson: "02", blocks: []int{3, 7}}},
	// logged in, so the todos requests have a token
	"04": {{lesson: "04", blocks: []int{1}}},
	// logged in, and the apic-auth project with its exec and signed requests
	"06": {{lesson: "06", blocks: []int{1, 8, 10, 11, 12}}},
	// the feature files the tape runs, and a login for the ones that need
	// it; the lesson deletes the undefined feature after showing it, the
	// tape shows it
	"07": {{lesson: "07", blocks: []int{3, 4, 5, 6, 8}, replace: [2]string{"rm apic-demo/features/undefined.feature\n", ""}}},
	"09": {
		// the OpenAPI document and the Postman collection and environment
		{lesson: "09", blocks: []int{1, 11}},
		// the tidied files the tape copies in, written to tidy/ instead of
		// into the project the tape is about to import
		{sh: "mkdir -p tidy"},
		{lesson: "09", blocks: []int{8, 9}, replace: [2]string{"todos-api/", "tidy/"}},
		// block 11 imported the collection; the tape does that on camera
		{sh: "rm -rf todos-api postman-api"},
	},
}

func main() {
	check := flag.Bool("check", false, "validate every tape's syntax with vhs and record nothing")
	prepare := flag.Bool("prepare", false, "set each tape's scratch directory up and keep it, recording nothing")
	only := flag.String("only", "", "comma-separated tape names to record, e.g. 01,05,demo (default: all)")
	out := flag.String("out", "dist/tapes", "directory to write the GIFs into")
	keep := flag.Bool("keep", false, "keep each tape's scratch directory for inspection")
	flag.Parse()
	tapes, err := Find(".")
	if err == nil {
		tapes, err = pick(tapes, *only)
	}
	if err != nil {
		fail(err)
	}
	if *check {
		if failed := Validate(tapes, os.Stdout); failed > 0 {
			fail(fmt.Errorf("%d of %d tapes are invalid", failed, len(tapes)))
		}
		return
	}
	if err := record(".", tapes, *out, *keep || *prepare, *prepare); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "tapes:", err)
	os.Exit(1)
}

// Find lists the lesson tapes in order, then the README's demo tape.
func Find(root string) ([]Tape, error) {
	paths, err := filepath.Glob(filepath.Join(root, TapesDir, "*.tape"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	paths = append(paths, filepath.Join(root, DemoTape))
	var tapes []Tape
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		tapes = append(tapes, Tape{Name: strings.TrimSuffix(filepath.Base(p), ".tape"), Path: abs})
	}
	return tapes, nil
}

func pick(tapes []Tape, only string) ([]Tape, error) {
	if only == "" {
		return tapes, nil
	}
	want := map[string]bool{}
	for _, n := range strings.Split(only, ",") {
		want[strings.TrimSpace(n)] = true
	}
	var out []Tape
	for _, t := range tapes {
		if want[t.Name] {
			out = append(out, t)
			delete(want, t.Name)
		}
	}
	for n := range want {
		return nil, fmt.Errorf("no tape called %q under %s", n, TapesDir)
	}
	return out, nil
}

// Validate runs `vhs validate` on every tape and returns how many failed,
// reporting each on w.
func Validate(tapes []Tape, w io.Writer) int {
	failed := 0
	for _, t := range tapes {
		cmd := exec.Command("vhs", "validate", t.Path) //nolint:gosec // vhs on the tapes of this repository
		out, err := cmd.CombinedOutput()
		if err != nil {
			failed++
			fmt.Fprintf(w, "✗ %s\n%s\n", t.Path, strings.TrimRight(string(out), "\n"))
			continue
		}
		fmt.Fprintf(w, "✓ %s\n", t.Path)
	}
	return failed
}

func record(root string, tapes []Tape, out string, keep, prepareOnly bool) error {
	for _, tool := range []string{"vhs", "ttyd", "ffmpeg"} {
		if _, err := exec.LookPath(tool); err != nil && !prepareOnly {
			return fmt.Errorf("%s is not on PATH; recording needs vhs, ttyd and ffmpeg (https://github.com/charmbracelet/vhs#installation)", tool)
		}
	}
	if err := os.MkdirAll(out, 0o755); err != nil { //nolint:gosec // the GIF directory the caller named
		return err
	}
	bin, err := os.MkdirTemp("", "apic-tapes-bin-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(bin) }()
	apic := filepath.Join(bin, "apic")
	if runtime.GOOS == "windows" {
		apic += ".exe"
	}
	build := exec.Command("go", "build", "-o", apic, "./cmd/apic") //nolint:gosec // the output path is the scratch directory this process just made
	build.Dir, build.Stdout, build.Stderr = root, os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("building apic: %w", err)
	}
	for _, t := range tapes {
		if err := recordOne(root, bin, t, out, keep, prepareOnly); err != nil {
			return fmt.Errorf("%s: %w", t.Name, err)
		}
	}
	return nil
}

// recordOne gives the tape a scratch directory with the demo project, the
// state its setup describes and the demo API listening, then runs vhs
// there and moves what it wrote into out.
func recordOne(root, bin string, t Tape, out string, keep, prepareOnly bool) error {
	work, err := os.MkdirTemp("", "apic-tapes-"+t.Name+"-*")
	if err != nil {
		return err
	}
	if !keep {
		defer func() { _ = os.RemoveAll(work) }()
	}
	for _, d := range []string{"home", filepath.Dir(TapesDir), filepath.Dir(DemoTape)} {
		if err := os.MkdirAll(filepath.Join(work, d), 0o755); err != nil { //nolint:gosec // scratch directory
			return err
		}
	}
	if _, _, err := demoapi.WriteProject(filepath.Join(work, "apic-demo"), demoPort, true); err != nil {
		return err
	}
	stop, err := serve()
	if err != nil {
		return err
	}
	defer stop()
	env := environment(bin, work)
	for _, s := range setup[t.Name] {
		code, err := s.code(root)
		if err != nil {
			return err
		}
		cmd := exec.Command("sh", "-e") //nolint:gosec // the course's own commands, as learncheck runs them
		cmd.Dir, cmd.Env, cmd.Stdin = work, append(env, "NO_COLOR=1"), strings.NewReader(code)
		if o, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("setup failed: %w\n%s", err, o)
		}
	}
	if prepareOnly {
		fmt.Printf("%s: prepared in %s\n", t.Name, work)
		return nil
	}
	vhs := exec.Command("vhs", t.Path) //nolint:gosec // vhs on a tape of this repository
	vhs.Dir, vhs.Env, vhs.Stdout, vhs.Stderr = work, env, os.Stdout, os.Stderr
	if err := vhs.Run(); err != nil {
		return fmt.Errorf("vhs: %w", err)
	}
	// A tape's Output is a repository-relative path, written under the
	// scratch directory; the result belongs in out.
	gifs, _ := filepath.Glob(filepath.Join(work, "docs", "*", "*.gif"))
	more, _ := filepath.Glob(filepath.Join(work, "docs", "*", "*", "*.gif"))
	gifs = append(gifs, more...)
	if len(gifs) == 0 {
		return errors.New("vhs wrote no GIF under the scratch directory; does the tape's Output start with docs/?")
	}
	for _, g := range gifs {
		dst := filepath.Join(out, filepath.Base(g))
		if err := os.Rename(g, dst); err != nil {
			return err
		}
		fmt.Printf("%s: wrote %s\n", t.Name, dst)
	}
	if keep {
		fmt.Printf("%s: scratch directory kept at %s\n", t.Name, work)
	}
	return nil
}

// environment is what the setup and vhs run under: the fresh apic first on
// PATH, a HOME of their own, UTC and a UTF-8 locale, and no NO_COLOR from
// the recording machine, since the picture should have colour.
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

// code is the shell the step runs: its snippet, or the lesson blocks it
// names, with the substitution applied.
func (s step) code(root string) (string, error) {
	if s.sh != "" {
		return s.sh + "\n", nil
	}
	blocks, err := LessonBlocks(root, s.lesson)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, n := range s.blocks {
		if n < 1 || n > len(blocks) {
			return "", fmt.Errorf("lesson %s has %d runnable blocks, no block %d", s.lesson, len(blocks), n)
		}
		code := blocks[n-1].Code
		if s.replace[0] != "" {
			code = strings.ReplaceAll(code, s.replace[0], s.replace[1])
		}
		b.WriteString(code)
	}
	return b.String(), nil
}

// LessonBlocks returns the runnable blocks of the lesson with that number
// prefix, in page order.
func LessonBlocks(root, lesson string) ([]lessons.Block, error) {
	pages, err := filepath.Glob(filepath.Join(root, LearnDir, lesson+"-*.md"))
	if err != nil {
		return nil, err
	}
	if len(pages) != 1 {
		return nil, fmt.Errorf("want one lesson page %s-*.md, found %d", lesson, len(pages))
	}
	data, err := os.ReadFile(pages[0]) //nolint:gosec // a course page, by glob
	if err != nil {
		return nil, err
	}
	return lessons.Extract(pages[0], string(data)), nil
}

// serve starts the demo API on demoPort for one tape.
func serve() (stop func(), err error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", demoPort))
	if err != nil {
		return nil, fmt.Errorf("the lessons show port %d, which is in use: %w", demoPort, err)
	}
	srv := &http.Server{Handler: demoapi.New(), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}, nil
}
