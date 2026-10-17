// Package record is what the course's recorders share. scripts/tapes and
// scripts/episodes both serve the demo API on the port the lessons show,
// run a lesson's own commands as setup in a scratch directory, and hand
// vhs an environment with nothing of the recording machine's in the
// picture; one copy of each keeps the two recordings alike.
package record

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hungovercoders/apic/internal/demoapi"
	"github.com/hungovercoders/apic/internal/lessons"
)

// Port is the port the lessons show in their URLs; a recording's picture
// carries it, so it is not negotiable the way learncheck's is.
const Port = 8089

// Step is one piece of a recording's setup: a shell snippet, or runnable
// blocks of a lesson page (1-based, in page order), optionally with a text
// substitution applied to them first.
type Step struct {
	Sh      string
	Lesson  string // the page's number prefix, "09"
	Blocks  []int
	Replace [2]string
}

// Code is the shell the step runs: its snippet, or the lesson blocks it
// names (from blocks, the lesson's runnable blocks in page order), with
// the substitution applied. A substitution that matches none of the
// blocks is an error, since the step would silently run the unchanged
// lesson commands.
func (s Step) Code(blocks []lessons.Block) (string, error) {
	if s.Sh != "" {
		return s.Sh + "\n", nil
	}
	var b strings.Builder
	for _, n := range s.Blocks {
		if n < 1 || n > len(blocks) {
			return "", fmt.Errorf("lesson %s has %d runnable blocks, no block %d", s.Lesson, len(blocks), n)
		}
		b.WriteString(blocks[n-1].Code)
	}
	code := b.String()
	if s.Replace[0] != "" {
		if !strings.Contains(code, s.Replace[0]) {
			return "", fmt.Errorf("nothing to replace: %q is not in blocks %v of lesson %s", s.Replace[0], s.Blocks, s.Lesson)
		}
		code = strings.ReplaceAll(code, s.Replace[0], s.Replace[1])
	}
	return code, nil
}

// Run reads the step's lesson, if it names one, and runs its code in work.
func (s Step) Run(root, work string, env []string) error {
	var blocks []lessons.Block
	if s.Sh == "" {
		var err error
		if blocks, err = lessons.Blocks(root, s.Lesson); err != nil {
			return err
		}
	}
	code, err := s.Code(blocks)
	if err != nil {
		return err
	}
	return Shell(work, env, code)
}

// Shell runs code with sh -e in work, as learncheck runs the lessons'
// blocks, with colour off since nothing is recording.
func Shell(work string, env []string, code string) error {
	cmd := exec.Command("sh", "-e") //nolint:gosec // the course's own commands, as learncheck runs them
	cmd.Dir, cmd.Env, cmd.Stdin = work, append(env, "NO_COLOR=1"), strings.NewReader(code)
	if o, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("setup failed: %w\n%s", err, o)
	}
	return nil
}

// Environment is what a recording's setup and vhs run under: the fresh
// apic in bin first on PATH, a HOME of its own under work, UTC and a UTF-8
// locale, and nothing of the recording machine's that would show in the
// picture: no NO_COLOR, no proxy (`apic env` reports the one it sees) and
// no APIC_ variable, since an exported APIC_VAR_baseUrl or APIC_VAR_user
// would override the demo project's values in every shot.
func Environment(bin, work string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch upper := strings.ToUpper(k); {
		case upper == "PATH", upper == "HOME", upper == "TZ", upper == "LANG", upper == "LC_ALL", upper == "NO_COLOR",
			upper == "HTTP_PROXY", upper == "HTTPS_PROXY", upper == "ALL_PROXY", upper == "NO_PROXY",
			strings.HasPrefix(upper, "APIC_"):
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+filepath.Join(work, "home"),
		"TZ=UTC",
		"LANG=C.UTF-8",
		"APIC_DEMO_URL=http://localhost:"+strconv.Itoa(Port),
	)
}

// Serve starts the demo API on Port, waiting a few seconds for the port
// when a recording's own `apic demo` is still letting go of it.
func Serve() (stop func(), err error) {
	var ln net.Listener
	for deadline := time.Now().Add(5 * time.Second); ; {
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", Port))
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("the lessons show port %d, which is in use: %w", Port, err)
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

// Pick keeps the items named in only, comma-separated, in the order
// given; an empty only keeps them all, and a name that matches nothing is
// an error, since a typo would otherwise record nothing and say so late.
func Pick[T any](items []T, name func(T) string, only string) ([]T, error) {
	if only == "" {
		return items, nil
	}
	var picked []T
	for _, n := range strings.Split(only, ",") {
		n = strings.TrimSpace(n)
		found := false
		for _, it := range items {
			if name(it) == n {
				picked = append(picked, it)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("no %q among %s", n, names(items, name))
		}
	}
	return picked, nil
}

func names[T any](items []T, name func(T) string) string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = name(it)
	}
	return strings.Join(out, ", ")
}
