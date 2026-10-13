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

// newProgramOver builds a program around m with a pipe standing in for
// the terminal: the returned writer is its keyboard, the buffer its
// screen. loop, render, dispatch and resize all work without a terminal.
func newProgramOver(t *testing.T, m *Model) (*program, *os.File, *bytes.Buffer) {
	t.Helper()
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
	prog := &program{m: m, in: pr, out: out, src: src, msgs: make(chan Msg, 64), done: make(chan struct{})}
	return prog, pw, out
}

// newLoopProgram is newProgramOver with an empty project behind it.
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
	prog, _, out := newProgramOver(t, New(Config{Root: dir, Runner: r, Theme: output.Default()}))
	return prog, out
}

// render paints every line with an erase to its end and the rows below
// erased, so a shorter frame leaves no debris, and paints nothing at all
// when the frame has not changed.
func TestRenderErasesAndSkipsUnchangedFrames(t *testing.T) {
	f := newFixture(t, runner.Options{})
	p, _, out := newProgramOver(t, f.m)
	f.m.Update(sizeMsg{width: 80, height: 20})
	p.render()
	painted := out.String()
	if !strings.HasPrefix(painted, cursorHome) || !strings.HasSuffix(painted, eraseLine+eraseBelow) {
		t.Fatalf("a frame starts at the top and ends by erasing below:\n%q", painted)
	}
	rows := strings.Split(strings.TrimSuffix(strings.TrimPrefix(painted, cursorHome), eraseBelow), "\r\n")
	if len(rows) != 20 {
		t.Fatalf("a 20-row terminal gets 20 rows, got %d", len(rows))
	}
	for i, row := range rows {
		if !strings.HasSuffix(row, eraseLine) {
			t.Fatalf("row %d is not erased to its end: %q", i+1, row)
		}
	}
	out.Reset()
	p.render()
	if out.Len() != 0 {
		t.Fatalf("an unchanged frame should paint nothing, got %q", out.String())
	}
	f.m.Update(kp("j"))
	p.render()
	if out.Len() == 0 {
		t.Fatal("a moved cursor is a new frame")
	}
}

// resize falls back to 80x24 when the terminal cannot say its size, and
// reports a change only once.
func TestResizeFallsBackAndReportsChanges(t *testing.T) {
	f := newFixture(t, runner.Options{})
	p, _, _ := newProgramOver(t, f.m)
	if !p.resize() || f.m.width != 80 || f.m.height != 24 {
		t.Fatalf("a pipe has no size, so the first resize should report 80x24, got %dx%d", f.m.width, f.m.height)
	}
	if p.resize() {
		t.Fatal("the same size again is not a change")
	}
}

// dispatch routes each kind of command the model returns.
func TestDispatchRoutesCommands(t *testing.T) {
	f := newFixture(t, runner.Options{})
	p, _, _ := newProgramOver(t, f.m)
	defer close(p.done)
	if p.dispatch(nil) || p.dispatch(func() Msg { return nil }) {
		t.Fatal("nothing to do is not a quit")
	}
	if !p.dispatch(Quit) {
		t.Fatal("Quit ends the loop")
	}
	if !p.dispatch(func() Msg { return Cmd(Quit) }) {
		t.Fatal("a command that returns a command is followed")
	}
	if !p.dispatch(batch(func() Msg { return nil }, Quit)) {
		t.Fatal("a batch runs each command, and a quit inside it quits")
	}
	expect := func(what string, want Msg, within time.Duration) {
		t.Helper()
		select {
		case got := <-p.msgs:
			if got != want {
				t.Fatalf("%s: got %#v, want %#v", what, got, want)
			}
		case <-time.After(within):
			t.Fatalf("%s: nothing arrived", what)
		}
	}
	if p.dispatch(async(func() Msg { return clearStatusMsg{seq: 7} })) {
		t.Fatal("async work does not quit")
	}
	expect("async", clearStatusMsg{seq: 7}, 2*time.Second)
	if p.dispatch(after(10*time.Millisecond, clearStatusMsg{seq: 8})) {
		t.Fatal("a delay does not quit")
	}
	select {
	case got := <-p.msgs:
		t.Fatalf("a delayed message arrived at once: %#v", got)
	default:
	}
	expect("delay", clearStatusMsg{seq: 8}, 2*time.Second)
	// A command that returns a plain message hands it to the model.
	if p.dispatch(func() Msg { return sizeMsg{width: 90, height: 30} }) {
		t.Fatal("a plain message does not quit")
	}
	if f.m.width != 90 || f.m.height != 30 {
		t.Fatalf("the model should have seen the message, size is %dx%d", f.m.width, f.m.height)
	}
}

// A delayed message is dropped when the program ends before it is due.
func TestDelayedMessageDiesWithTheProgram(t *testing.T) {
	f := newFixture(t, runner.Options{})
	p, _, _ := newProgramOver(t, f.m)
	p.dispatch(after(20*time.Millisecond, clearStatusMsg{seq: 1}))
	close(p.done)
	select {
	case got := <-p.msgs:
		t.Fatalf("got %#v after the program ended", got)
	case <-time.After(100 * time.Millisecond):
	}
}

// loop enters the alternate screen, quits on q from the terminal, and
// returns when its context ends.
func TestLoopQuitsOnKeyAndOnContext(t *testing.T) {
	f := newFixture(t, runner.Options{})
	p, keyboard, out := newProgramOver(t, f.m)
	if _, err := keyboard.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- p.loop(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("loop: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("q should end the loop")
	}
	p.stopKeys()
	if !strings.HasPrefix(out.String(), altScreenOn+hideCursor+clearScreen) {
		t.Fatalf("loop should open the alternate screen first:\n%q", out.String())
	}
	if !strings.Contains(out.String(), "auth.http") {
		t.Fatal("loop should have painted the project")
	}

	f2 := newFixture(t, runner.Options{})
	p2, _, _ := newProgramOver(t, f2.m)
	ctx, cancel := context.WithCancel(context.Background())
	done2 := make(chan error, 1)
	go func() { done2 <- p2.loop(ctx) }()
	cancel()
	select {
	case err := <-done2:
		if err != nil {
			t.Fatalf("loop: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a cancelled context should end the loop")
	}
	p2.stopKeys()
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
