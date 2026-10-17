// Command tapes records the course's VHS tapes the same way every time. It
// builds apic, serves the demo API on the port the lessons show, gives each
// tape a scratch directory with a fresh apic-demo project and the files its
// lesson's runnable blocks create (internal/lessons), and runs vhs there
// with a fixed HOME and time zone, so two recordings of a tape differ only
// in timings. Run it with `task tapes` (vhs, ttyd, ffmpeg and jq on PATH);
// `task tapes:check` validates every tape's syntax with vhs alone, which
// CI does, so a tape that only a person has run cannot quietly break.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/hungovercoders/apic/internal/demoapi"
	"github.com/hungovercoders/apic/scripts/internal/record"
)

// Where the tapes and the course live, relative to the repository root.
const (
	TapesDir = "docs/learn/tapes"
	DemoTape = "docs/assets/demo.tape"
)

// Tape is one VHS script: its name (01, 05, demo) and its path.
type Tape struct {
	Name string
	Path string
}

// setup is what each tape expects to find beyond the demo project: the
// state the lesson has built up by the time its terminal parts begin, as
// record.Steps run in its scratch directory. A tape not listed needs only
// apic-demo and the running API.
var setup = map[string][]record.Step{
	// the staging environment and the private env file
	"02": {{Lesson: "02", Blocks: []int{3, 7}}},
	// logged in, so the todos requests have a token
	"04": {{Lesson: "04", Blocks: []int{1}}},
	// logged in, and the apic-auth project with its exec and signed requests
	"06": {{Lesson: "06", Blocks: []int{1, 8, 10, 11, 12}}},
	// the feature files the tape runs, and a login for the ones that need
	// it; the lesson deletes the undefined feature after showing it, the
	// tape shows it
	"07": {{Lesson: "07", Blocks: []int{3, 4, 5, 6, 8}, Replace: [2]string{"rm apic-demo/features/undefined.feature\n", ""}}},
	"09": {
		// the OpenAPI document and the Postman collection and environment
		{Lesson: "09", Blocks: []int{1, 11}},
		// the tidied files the tape copies in, written to tidy/ instead of
		// into the project the tape is about to import
		{Sh: "mkdir -p tidy"},
		{Lesson: "09", Blocks: []int{8, 9}, Replace: [2]string{"todos-api/", "tidy/"}},
		// block 11 imported the collection; the tape does that on camera
		{Sh: "rm -rf todos-api postman-api"},
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
		tapes, err = record.Pick(tapes, func(t Tape) string { return t.Name }, *only)
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
	if err := recordAll(".", tapes, *out, *keep || *prepare, *prepare); err != nil {
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

func recordAll(root string, tapes []Tape, out string, keep, prepareOnly bool) error {
	// vhs records a shell, so a missing tool would not fail the recording:
	// it would be filmed as "command not found". Check up front for vhs
	// and what it drives, and for jq, which most tapes pipe through.
	for _, tool := range []string{"vhs", "ttyd", "ffmpeg", "jq"} {
		if _, err := exec.LookPath(tool); err != nil && !prepareOnly {
			return fmt.Errorf("%s is not on PATH; recording needs vhs, ttyd and ffmpeg (https://github.com/charmbracelet/vhs#installation) and jq, which the tapes pipe through", tool)
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
	if _, _, err := demoapi.WriteProject(filepath.Join(work, "apic-demo"), record.Port, true); err != nil {
		return err
	}
	stop, err := record.Serve()
	if err != nil {
		return err
	}
	defer stop()
	env := record.Environment(bin, work)
	for _, s := range setup[t.Name] {
		if err := s.Run(root, work, env); err != nil {
			return err
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
		if err := moveFile(g, dst); err != nil {
			return err
		}
		fmt.Printf("%s: wrote %s\n", t.Name, dst)
	}
	if keep {
		fmt.Printf("%s: scratch directory kept at %s\n", t.Name, work)
	}
	return nil
}

// moveFile renames src to dst, copying when the two are on different
// filesystems (the scratch directory is under the system temp directory,
// the output under the repository), where a rename fails.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	data, err := os.ReadFile(src) //nolint:gosec // the GIF vhs just wrote in the scratch directory
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil { //nolint:gosec // a GIF for the docs, meant to be world-readable
		return err
	}
	return os.Remove(src)
}
