package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hungovercoders/apic/internal/runner"
)

// An interrupt cancels the context the commands run under, instead of
// ending the process; after it, handling is back to the default.
func TestInterruptCancelsTheContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a process cannot send itself an interrupt on Windows")
	}
	ctx, stop := signalContext(context.Background())
	defer stop()
	self, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := self.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the interrupt did not cancel the context")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// The demo server stops when its context is cancelled (Ctrl-C), and
// exits 0: being told to stop is not an error.
func TestDemoStopsWhenCancelled(t *testing.T) {
	dir := t.TempDir()
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port)); err == nil {
				_ = c.Close()
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()
	a := New()
	var out, errb bytes.Buffer
	a.Stdout, a.Stderr = &out, &errb
	code := a.Execute(ctx, []string{"demo", "--out", dir, "--port", fmt.Sprint(port)})
	if code != 0 || !strings.Contains(out.String(), "demo api listening on") || errb.Len() != 0 {
		t.Fatalf("code=%d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errb.String())
	}
}

// A port already in use is E214, the code whose catalogue entry names
// this command, not the uncoded E200. It fails before anything is
// written or announced: no startup object or "listening" line on stdout,
// and no project on disk.
func TestDemoPortInUseIsAServerError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port
	dir := filepath.Join(t.TempDir(), "apic-demo")
	code, out, errb := execute(t, "--json", "demo", "--out", dir, "--port", fmt.Sprint(port))
	var got struct {
		Error runner.ErrorInfo `json:"error"`
	}
	if err := json.Unmarshal([]byte(errb), &got); err != nil {
		t.Fatalf("stderr is not one JSON error object (exit %d): %v\n%s", code, err, errb)
	}
	if code != 2 || got.Error.Code != runner.CodeServer {
		t.Errorf("code=%d %s: %s", code, got.Error.Code, got.Error.Message)
	}
	if out != "" {
		t.Errorf("stdout should be empty when the port is taken:\n%s", out)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the project should not be written when the port is taken: %v", err)
	}
	code, out, _ = execute(t, "demo", "--out", dir, "--port", fmt.Sprint(port))
	if code != 2 || strings.Contains(out, "listening") {
		t.Errorf("text mode: code=%d stdout:\n%s", code, out)
	}
}

// hangServer answers nothing until the test ends, so a request to it is
// still in flight when the context is cancelled.
func hangServer(t *testing.T) string {
	t.Helper()
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-done:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(done); srv.Close() })
	return srv.URL
}

// --keep-going carries a flow past a failed request, not past Ctrl-C: once
// the context is cancelled, the remaining requests and data rows are not
// attempted, so the output holds what completed plus the one interrupted,
// not a cancellation per remaining request.
func TestCancellationStopsAKeepGoingFlow(t *testing.T) {
	url := hangServer(t)
	dir := t.TempDir()
	files := map[string]string{
		"a.http":   "# @name one\nGET " + url + "/{{row}}\n\n###\n# @name two\nGET " + url + "/\n\n###\n# @name three\nGET " + url + "/\n",
		"rows.csv": "row\nx\ny\nz\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"run", "a.http", "--keep-going", "--json", "--var", "row=x"},
		{"run", "a.http", "--keep-going", "--json", "--data", filepath.Join(dir, "rows.csv")},
	} {
		t.Run(strings.Join(args[2:], " "), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			time.AfterFunc(100*time.Millisecond, cancel)
			a := New()
			var out, errb bytes.Buffer
			a.Stdout, a.Stderr = &out, &errb
			code := a.Execute(ctx, append([]string{"-C", dir, "--no-session"}, args...))
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			if code != 3 || len(lines) != 1 || !strings.Contains(lines[0], `"E305"`) {
				t.Fatalf("code=%d, %d result lines, want 3 and one E305 result:\n%s\n%s", code, len(lines), out.String(), errb.String())
			}
			if !strings.Contains(errb.String(), `"code":"E305"`) {
				t.Errorf("stderr should end with the E305 object:\n%s", errb.String())
			}
		})
	}
}
