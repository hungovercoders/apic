package ui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hungovercoders/apic/internal/output"
	"github.com/hungovercoders/apic/internal/project"
	"github.com/hungovercoders/apic/internal/runner"
	"github.com/hungovercoders/apic/internal/session"
)

// newLoopProgram builds a program over a pipe standing in for the
// terminal, with an empty project behind it, so loop can run without one.
func newLoopProgram(t *testing.T) (*program, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	p, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	r, err := runner.New(p, runner.Options{Session: session.NewMemory()})
	if err != nil {
		t.Fatal(err)
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pw.Close(); _ = pr.Close() })
	src, err := newKeySource(pr)
	if err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	prog := &program{m: New(Config{Root: dir, Runner: r, Theme: output.Default()}), in: pr, out: out, src: src, msgs: make(chan Msg, 64), done: make(chan struct{})}
	return prog, out
}

// A panic on the request goroutine reaches the loop as a message, which
// raises it again only once the terminal has been put back, so the crash
// prints on a usable terminal and carries the worker's stack.
func TestWorkerPanicRestoresTheTerminal(t *testing.T) {
	p, out := newLoopProgram(t)
	if p.dispatch(async(func() Msg { panic("boom") })) {
		t.Fatal("async work should not quit")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		defer p.restore() // what Run defers
		_ = p.loop(ctx)
	}()
	p.stopKeys()
	if recovered == nil {
		t.Fatal("the loop should re-raise the worker's panic")
	}
	text := fmt.Sprint(recovered)
	if !strings.Contains(text, "boom") || !strings.Contains(text, "goroutine") {
		t.Fatalf("the panic should carry the value and the worker's stack:\n%s", text)
	}
	if !strings.HasSuffix(out.String(), showCursor+altScreenOff) {
		t.Fatalf("the terminal should be restored before the panic:\n%q", out.String())
	}
}
