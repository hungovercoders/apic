package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
)

// Msg is an event delivered to the model.
type Msg any

// Cmd is work run off the event loop; its Msg comes back to the model.
type Cmd func() Msg

// sizeMsg reports the terminal size.
type sizeMsg struct{ width, height int }

// tickMsg advances the spinner while a run is in flight.
type tickMsg struct{}

// execMsg asks the program to suspend and run a command (the editor).
type execMsg struct {
	cmd  *exec.Cmd
	done func(error) Msg
}

// quitMsg ends the program.
type quitMsg struct{}

// Quit is the command that ends the UI.
func Quit() Msg { return quitMsg{} }

const (
	altScreenOn  = "\x1b[?1049h"
	altScreenOff = "\x1b[?1049l"
	hideCursor   = "\x1b[?25l"
	showCursor   = "\x1b[?25h"
	clearScreen  = "\x1b[2J\x1b[H"
	cursorHome   = "\x1b[H"
	eraseLine    = "\x1b[K"
	eraseBelow   = "\x1b[J"
)

// Run drives the model on a real terminal: raw mode, the alternate screen,
// a size poller and a spinner ticker. It restores the terminal on the way
// out, including when the model panics.
func Run(ctx context.Context, m *Model, in *os.File, out io.Writer) error {
	if !term.IsTerminal(in.Fd()) {
		return errors.New("stdin is not a terminal")
	}
	state, err := term.MakeRaw(in.Fd())
	if err != nil {
		return fmt.Errorf("raw mode: %w (on Windows use Windows Terminal, or winpty in Git Bash)", err)
	}
	src, err := newKeySource(in)
	if err != nil {
		_ = term.Restore(in.Fd(), state)
		return fmt.Errorf("input: %w", err)
	}
	p := &program{m: m, in: in, out: out, src: src, state: state, msgs: make(chan Msg, 64), done: make(chan struct{})}
	defer p.restore()
	err = p.loop(ctx)
	p.stopKeys()
	return err
}

type program struct {
	m     *Model
	in    *os.File
	out   io.Writer
	src   keySource
	state *term.State
	msgs  chan Msg
	done  chan struct{}

	keys     chan Key
	stopped  chan struct{} // closed when the key reader has returned; nil while it is not running
	lastSize sizeMsg
	frame    string
	restored bool
}

func (p *program) write(s string) { _, _ = io.WriteString(p.out, s) }

func (p *program) restore() {
	if p.restored {
		return
	}
	p.restored = true
	p.write(showCursor + altScreenOff)
	p.restoreTerminal()
}

// restoreTerminal puts the terminal modes back; a program built without
// them (tests) has nothing to restore.
func (p *program) restoreTerminal() {
	if p.state != nil {
		_ = term.Restore(p.in.Fd(), p.state)
	}
}

func (p *program) loop(ctx context.Context) error {
	p.write(altScreenOn + hideCursor + clearScreen)
	p.keys = make(chan Key, 16)
	p.startKeys()
	defer close(p.done)

	size := time.NewTicker(250 * time.Millisecond)
	defer size.Stop()
	tick := time.NewTicker(110 * time.Millisecond)
	defer tick.Stop()

	p.resize()
	p.render()
	for {
		select {
		case <-ctx.Done():
			return nil
		case k := <-p.keys:
			if p.dispatch(p.m.Update(k)) {
				return nil
			}
		case msg := <-p.msgs:
			if pm, ok := msg.(panicMsg); ok {
				// Raised here rather than on the worker so Run's deferred
				// restore puts the terminal back before the crash prints.
				panic(pm)
			}
			if p.dispatch(p.m.Update(msg)) {
				return nil
			}
		case <-size.C:
			if p.resize() {
				p.render()
			}
			continue
		case <-tick.C:
			if !p.m.Running() {
				continue
			}
			if p.dispatch(p.m.Update(tickMsg{})) {
				return nil
			}
		}
		p.render()
	}
}

// dispatch runs a command off the loop and reports whether to quit.
func (p *program) dispatch(cmd Cmd) bool {
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case nil:
		return false
	case quitMsg:
		return true
	case execMsg:
		p.exec(msg)
		return false
	case Cmd:
		return p.dispatch(msg)
	case batchMsg:
		for _, c := range msg {
			if p.dispatch(c) {
				return true
			}
		}
		return false
	case asyncMsg:
		go func() {
			out := guard(msg.run)
			select {
			case p.msgs <- out:
			case <-p.done:
			}
		}()
		return false
	case delayMsg:
		go func() {
			t := time.NewTimer(msg.d)
			defer t.Stop()
			select {
			case <-t.C:
			case <-p.done:
				return
			}
			select {
			case p.msgs <- msg.msg:
			case <-p.done:
			}
		}()
		return false
	default:
		return p.dispatch(func() Msg { return msg })
	}
}

// startKeys begins decoding keys from the terminal on a goroutine of their
// own.
func (p *program) startKeys() {
	p.src.Resume()
	stopped := make(chan struct{})
	p.stopped = stopped
	go func() {
		defer close(stopped)
		readKeys(p.src, p.keys, p.done)
	}()
}

// stopKeys interrupts the key reader and waits for it to return, so that
// no read is outstanding on the terminal while another program has it.
// Keys still queued are dropped: they were typed before the editor
// opened, and acting on them once it closes would be a surprise.
func (p *program) stopKeys() {
	if p.stopped == nil {
		return
	}
	p.src.Cancel()
	for {
		select {
		case <-p.keys:
		case <-p.stopped:
			p.stopped = nil
			return
		}
	}
}

// exec suspends the UI, runs a command attached to the terminal, and
// restores the UI afterwards. The key reader is stopped first, so every
// byte typed while the command runs reaches it and not the UI.
func (p *program) exec(e execMsg) {
	p.stopKeys()
	p.write(showCursor + altScreenOff)
	p.restoreTerminal()
	e.cmd.Stdin, e.cmd.Stdout, e.cmd.Stderr = p.in, p.out, os.Stderr
	err := e.cmd.Run()
	if state, rerr := term.MakeRaw(p.in.Fd()); rerr == nil {
		p.state = state
	}
	p.write(altScreenOn + hideCursor + clearScreen)
	p.frame = ""
	p.startKeys()
	if e.done != nil {
		select {
		case p.msgs <- e.done(err):
		case <-p.done:
		}
	}
}

func (p *program) resize() bool {
	w, h, err := term.GetSize(p.in.Fd())
	if err != nil || w <= 0 || h <= 0 {
		w, h = 80, 24
	}
	if p.lastSize.width == w && p.lastSize.height == h {
		return false
	}
	p.lastSize = sizeMsg{w, h}
	p.m.Update(sizeMsg{w, h})
	p.frame = ""
	return true
}

// render paints the frame. Every line is erased to the end so a shorter
// line cannot leave debris from the previous frame.
func (p *program) render() {
	frame := p.m.View()
	if frame == p.frame {
		return
	}
	p.frame = frame
	var b strings.Builder
	b.WriteString(cursorHome)
	lines := strings.Split(frame, "\n")
	for i, line := range lines {
		b.WriteString(line)
		b.WriteString(eraseLine)
		if i < len(lines)-1 {
			b.WriteString("\r\n")
		}
	}
	b.WriteString(eraseBelow)
	p.write(b.String())
}

// batchMsg carries several commands to run in order.
type batchMsg []Cmd

// batch runs several commands, so one key can both start a run and set a
// status.
func batch(cmds ...Cmd) Cmd {
	return func() Msg { return batchMsg(cmds) }
}

// panicMsg is a panic caught on a worker goroutine, carried to the loop
// so that the terminal is restored before it is raised again. A panic that
// escaped the worker would end the process with the terminal still in raw
// mode on the alternate screen.
type panicMsg struct {
	value any
	stack []byte
}

func (pm panicMsg) String() string { return fmt.Sprintf("%v\n\n%s", pm.value, pm.stack) }

// guard runs fn, turning a panic into a panicMsg.
func guard(fn func() Msg) (out Msg) {
	defer func() {
		if v := recover(); v != nil {
			out = panicMsg{value: v, stack: debug.Stack()}
		}
	}()
	return fn()
}

// asyncMsg carries work that must not block the event loop.
type asyncMsg struct{ run func() Msg }

// async schedules work off the loop; its result comes back as a message.
func async(run func() Msg) Cmd {
	return func() Msg { return asyncMsg{run: run} }
}

// delayMsg asks the program to deliver msg once d has passed.
type delayMsg struct {
	d   time.Duration
	msg Msg
}

// after schedules a message once d has passed.
func after(d time.Duration, msg Msg) Cmd {
	return func() Msg { return delayMsg{d: d, msg: msg} }
}
