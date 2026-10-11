package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
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
// this command, not the uncoded E200.
func TestDemoPortInUseIsAServerError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port
	code, _, errb := execute(t, "--json", "demo", "--out", t.TempDir(), "--port", fmt.Sprint(port))
	var got struct {
		Error runner.ErrorInfo `json:"error"`
	}
	if err := json.Unmarshal([]byte(errb), &got); err != nil {
		t.Fatalf("stderr is not one JSON error object (exit %d): %v\n%s", code, err, errb)
	}
	if code != 2 || got.Error.Code != runner.CodeServer {
		t.Errorf("code=%d %s: %s", code, got.Error.Code, got.Error.Message)
	}
}
