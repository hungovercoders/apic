// Command learncheck runs the commands in the course pages so a lesson
// cannot rot: it builds apic, starts `apic demo` in a scratch directory,
// then extracts every fenced block in the site's learn/*.md and migrate/*.md
// that is preceded by an `<!-- learn -->` comment (internal/lessons) and runs it there with
// `sh -e`, in page order (the course first, then the migration guides).
//
// Blocks that need credentials, an editor or a browser are simply not
// marked. Run it with `task learn:check`; CI runs it on every change to the
// course, the demo API or the CLI.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
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

	"github.com/hungovercoders/apic/internal/lessons"
)

// Learn and Migrate hold the course and the migration guides, relative to
// the repository root.
const (
	Learn   = "website/src/content/docs/learn"
	Migrate = "website/src/content/docs/migrate"
)

func main() {
	docs := flag.String("docs", Learn+","+Migrate, "comma-separated directories holding the pages, run in this order")
	keep := flag.Bool("keep", false, "keep the scratch directory for inspection")
	flag.Parse()
	if err := run(strings.Split(*docs, ","), *keep); err != nil {
		fmt.Fprintln(os.Stderr, "learncheck:", err)
		os.Exit(1)
	}
}

func run(dirs []string, keep bool) error {
	var pages []string
	for _, dir := range dirs {
		found, err := filepath.Glob(filepath.Join(dir, "*.md"))
		if err != nil {
			return err
		}
		sort.Strings(found)
		pages = append(pages, found...)
	}
	var blocks []lessons.Block
	for _, p := range pages {
		data, err := os.ReadFile(p) //nolint:gosec // the course pages, by glob
		if err != nil {
			return err
		}
		blocks = append(blocks, lessons.Extract(p, string(data))...)
	}
	if len(blocks) == 0 {
		return fmt.Errorf("no runnable blocks under %s (mark one with <!-- learn -->)", strings.Join(dirs, ", "))
	}

	work, err := os.MkdirTemp("", "learncheck-*")
	if err != nil {
		return err
	}
	if !keep {
		defer func() { _ = os.RemoveAll(work) }()
	}
	bin := filepath.Join(work, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil { //nolint:gosec // scratch directory
		return err
	}
	apic := filepath.Join(bin, "apic")
	if runtime.GOOS == "windows" {
		apic += ".exe"
	}
	build := exec.Command("go", "build", "-o", apic, "./cmd/apic") //nolint:gosec // the output path is the scratch directory this process just made
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("building apic: %w", err)
	}

	// The lessons say `apic demo` writes ./apic-demo and serves on 8089;
	// the harness does the same in the scratch directory, on the default
	// port when it is free so the pages' URLs match, else on a free one.
	port := 8089
	if !portFree(port) {
		port = freePort()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	demo := exec.CommandContext(ctx, apic, "demo", "--out", "apic-demo", "--port", strconv.Itoa(port), "--force") //nolint:gosec // the binary built a moment ago, in the scratch directory
	demo.Dir = work
	demo.Stdout, demo.Stderr = os.Stdout, os.Stderr
	if err := demo.Start(); err != nil {
		return fmt.Errorf("starting apic demo: %w", err)
	}
	if err := waitFor(fmt.Sprintf("http://127.0.0.1:%d/health", port), 10*time.Second); err != nil {
		return err
	}
	fmt.Printf("learncheck: apic demo on port %d, %d blocks from %d pages\n", port, len(blocks), len(pages))

	env := append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"APIC_DEMO_URL=http://localhost:"+strconv.Itoa(port),
		"NO_COLOR=1",
	)
	failed := 0
	for _, b := range blocks {
		where := fmt.Sprintf("%s:%d", b.File, b.Line)
		cmd := exec.Command("sh", "-e") //nolint:gosec // running the course's own commands is the job
		cmd.Dir = work
		cmd.Env = env
		cmd.Stdin = strings.NewReader(b.Code)
		out, err := cmd.CombinedOutput()
		if err != nil {
			failed++
			fmt.Printf("✗ %s\n%s\n", where, indent(strings.TrimRight(string(out), "\n")))
			continue
		}
		fmt.Printf("✓ %s\n", where)
	}
	if keep {
		fmt.Println("learncheck: scratch directory kept at", work)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d blocks failed", failed, len(blocks))
	}
	return nil
}

func portFree(port int) bool {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func freePort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

// waitFor polls url until it answers 200, or the deadline passes.
func waitFor(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		res, err := http.Get(url) //nolint:gosec // a loopback URL this process chose
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("apic demo did not come up in time")
}

func indent(s string) string {
	var b strings.Builder
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		b.WriteString("    " + sc.Text() + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
