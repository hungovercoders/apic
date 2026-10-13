//go:build unix

package ui

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// terminalPipe stands in for the terminal: the read end is what the UI and
// the editor read, the write end is the keyboard.
func terminalPipe(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pr.Close(); _ = pw.Close() })
	return pr, pw
}

func newTestProgram(t *testing.T, pr *os.File) *program {
	t.Helper()
	src, err := newKeySource(pr)
	if err != nil {
		t.Fatal(err)
	}
	p := &program{in: pr, out: &bytes.Buffer{}, src: src, keys: make(chan Key, 16), msgs: make(chan Msg, 64), done: make(chan struct{})}
	// stopKeys first: it waits for the reader to let go of the terminal,
	// which closing done alone would not.
	t.Cleanup(func() { p.stopKeys(); close(p.done) })
	return p
}

func wantKey(t *testing.T, keys <-chan Key, want string) {
	t.Helper()
	select {
	case k := <-keys:
		if k.String() != want {
			t.Fatalf("got key %q, want %q", k.String(), want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no key arrived, want %q", want)
	}
}

// The poll source returns from a pending Read on Cancel without touching
// the terminal, and reads again after Resume.
func TestPollSourceCancelLeavesInputUnread(t *testing.T) {
	pr, pw := terminalPipe(t)
	src, err := newKeySource(pr)
	if err != nil {
		t.Fatal(err)
	}
	pending := readAsync(src)
	src.Cancel()
	if r := waitRead(t, pending); !errors.Is(r.err, errCancelled) {
		t.Fatalf("Cancel should end the pending Read with errCancelled, got %q, %v", r.s, r.err)
	}
	src.Cancel() // a second Cancel must not poison the next Read
	if _, err := pw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	// Nothing is reading for the UI now, so the bytes are still in the
	// terminal for whoever reads it next.
	buf := make([]byte, 8)
	if n, err := pr.Read(buf); err != nil || string(buf[:n]) != "hello" {
		t.Fatalf("the input should still be there: %q, %v", buf[:n], err)
	}
	src.Resume()
	after := readAsync(src)
	if _, err := pw.Write([]byte("k")); err != nil {
		t.Fatal(err)
	}
	if r := waitRead(t, after); r.err != nil || r.s != "k" {
		t.Fatalf("after Resume: %q, %v", r.s, r.err)
	}
}

// stopKeys leaves the terminal with no read outstanding, and startKeys
// picks keys up again.
func TestStopKeysLeavesTheTerminalAlone(t *testing.T) {
	pr, pw := terminalPipe(t)
	p := newTestProgram(t, pr)
	p.startKeys()
	if _, err := pw.Write([]byte("j")); err != nil {
		t.Fatal(err)
	}
	wantKey(t, p.keys, "j")
	p.stopKeys()
	if _, err := pw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	got := readAsync(pr)
	if r := waitRead(t, got); r.err != nil || r.s != "hello" {
		t.Fatalf("typed after stopKeys, the bytes should still be in the terminal: %q, %v", r.s, r.err)
	}
	p.startKeys()
	if _, err := pw.Write([]byte("k")); err != nil {
		t.Fatal(err)
	}
	wantKey(t, p.keys, "k")
}

// readyWriter is the program's output: when the editor says it is ready,
// it types a line at the terminal, so the line can only arrive once exec
// has stopped the key reader.
type readyWriter struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	pw   *os.File
	once sync.Once
}

func (w *readyWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(b)
	if strings.Contains(w.buf.String(), "ready") {
		w.once.Do(func() { _, _ = w.pw.Write([]byte("hello\n")) })
	}
	return n, err
}

func (w *readyWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// exec hands the terminal to the editor: every byte typed while it runs
// reaches the editor, none reaches the UI, and keys work again afterwards.
func TestExecLeavesTheTerminalToTheEditor(t *testing.T) {
	pr, pw := terminalPipe(t)
	p := newTestProgram(t, pr)
	out := &readyWriter{pw: pw}
	p.out = out
	p.startKeys()
	if _, err := pw.Write([]byte("j")); err != nil {
		t.Fatal(err)
	}
	wantKey(t, p.keys, "j")

	editor := exec.Command("sh", "-c", `printf ready; IFS= read -r line; printf 'editor got %s' "$line"`)
	var done Msg
	p.exec(execMsg{cmd: editor, done: func(err error) Msg { return editorDoneMsg{err: err} }})
	select {
	case done = <-p.msgs:
	case <-time.After(2 * time.Second):
		t.Fatal("exec should report the editor finishing")
	}
	if m, ok := done.(editorDoneMsg); !ok || m.err != nil {
		t.Fatalf("editor done: %#v", done)
	}
	if !strings.Contains(out.String(), "editor got hello") {
		t.Fatalf("the editor should have read the line typed while it ran:\n%s", out.String())
	}
	select {
	case k := <-p.keys:
		t.Fatalf("the UI should not have seen a key while the editor ran, got %q", k.String())
	default:
	}
	if _, err := pw.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	wantKey(t, p.keys, "q")
	if !strings.HasSuffix(out.String(), altScreenOn+hideCursor+clearScreen) {
		t.Fatalf("exec should put the UI back on the alternate screen:\n%q", out.String())
	}
}
