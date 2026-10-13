package ui

import (
	"errors"
	"io"
	"testing"
	"time"
)

// readResult is one Read's outcome, carried over a channel so a test can
// wait on it with a timeout.
type readResult struct {
	s   string
	err error
}

func readAsync(r io.Reader) <-chan readResult {
	out := make(chan readResult, 1)
	go func() {
		buf := make([]byte, 64)
		n, err := r.Read(buf)
		out <- readResult{string(buf[:n]), err}
	}()
	return out
}

func waitRead(t *testing.T, c <-chan readResult) readResult {
	t.Helper()
	select {
	case r := <-c:
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("Read did not return")
		return readResult{}
	}
}

// The fallback source, used where poll(2) is not available: Cancel
// unblocks a pending Read, Resume drops what arrived in between, and the
// next Read sees only what came after.
func TestBgSourceCancelAndResume(t *testing.T) {
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()
	s := newBgSource(pr)

	pending := readAsync(s)
	s.Cancel()
	if r := waitRead(t, pending); !errors.Is(r.err, errCancelled) {
		t.Fatalf("Cancel should end the pending Read with errCancelled, got %q, %v", r.s, r.err)
	}
	if r := waitRead(t, readAsync(s)); !errors.Is(r.err, errCancelled) {
		t.Fatalf("a Read after Cancel should fail at once, got %q, %v", r.s, r.err)
	}

	// Typed while paused: taken by the background read, which marks it
	// stale, so it is dropped however late it is consumed. Wait for it to
	// be queued so that Resume cannot run before the mark is taken.
	if _, err := pw.Write([]byte("zzz")); err != nil {
		t.Fatal(err)
	}
	for start := time.Now(); len(s.chunks) == 0; {
		if time.Since(start) > 2*time.Second {
			t.Fatal("the reader should have queued the chunk")
		}
		time.Sleep(time.Millisecond)
	}
	s.Resume()
	after := readAsync(s)
	if _, err := pw.Write([]byte("a")); err != nil {
		t.Fatal(err)
	}
	if r := waitRead(t, after); r.err != nil || r.s != "a" {
		t.Fatalf("after Resume the first Read should see only what was typed after it, got %q, %v", r.s, r.err)
	}

	// A Read shorter than the chunk keeps the rest for the next one.
	if _, err := pw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	one := make([]byte, 2)
	if n, err := s.Read(one); err != nil || string(one[:n]) != "he" {
		t.Fatalf("short read: %q, %v", one[:n], err)
	}
	rest := make([]byte, 8)
	if n, err := s.Read(rest); err != nil || string(rest[:n]) != "llo" {
		t.Fatalf("rest: %q, %v", rest[:n], err)
	}

	// The reader's own error ends every Read from then on.
	_ = pw.Close()
	for i := 0; i < 2; i++ {
		if r := waitRead(t, readAsync(s)); !errors.Is(r.err, io.EOF) {
			t.Fatalf("read %d after the pipe closed: %q, %v", i, r.s, r.err)
		}
	}
}
