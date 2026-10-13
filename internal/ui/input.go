package ui

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
)

// errCancelled is what a keySource's Read returns once Cancel has been
// called on it.
var errCancelled = errors.New("read cancelled")

// keySource is the terminal readKeys reads from. Its pending Read can be
// interrupted, which is what lets the editor have the terminal to itself:
// the program cancels the read before it suspends the UI and resumes it
// afterwards, so a byte typed into the editor is never taken by apic
// instead and replayed as a key when the editor closes.
type keySource interface {
	io.Reader
	// Cancel makes the pending Read, or the next one, return errCancelled.
	// A second Cancel before Resume does nothing.
	Cancel()
	// Resume lets Read work again. Input that arrived in between is dropped.
	Resume()
}

// bgSource is the keySource for platforms without poll(2): one goroutine
// reads the file for the life of the program and Read waits on it or on
// Cancel. The read that is pending when Cancel is called still completes
// in the background, so on these platforms the first bytes typed into the
// editor can be lost to it. Each chunk records whether its read returned
// while paused, and Read drops those, so what the editor was meant to
// get never reaches the model as keys, however late the chunk arrives.
type bgSource struct {
	chunks chan chunk
	errc   chan error
	paused atomic.Bool

	mu     sync.Mutex
	cancel chan struct{} // closed by Cancel, replaced by Resume
	err    error         // the reader's final error, once it has one
	rest   []byte        // the tail of a chunk a short Read left over
}

// chunk is one read's bytes. stale marks a read that returned while the
// source was paused: its bytes were typed at the editor, not the UI.
type chunk struct {
	b     []byte
	stale bool
}

func newBgSource(r io.Reader) *bgSource {
	s := &bgSource{chunks: make(chan chunk, 8), errc: make(chan error, 1), cancel: make(chan struct{})}
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				s.chunks <- chunk{b: append([]byte(nil), buf[:n]...), stale: s.paused.Load()}
			}
			if err != nil {
				s.errc <- err
				return
			}
		}
	}()
	return s
}

func (s *bgSource) Read(p []byte) (int, error) {
	s.mu.Lock()
	cancel, err, rest := s.cancel, s.err, s.rest
	s.mu.Unlock()
	if len(rest) > 0 {
		n := copy(p, rest)
		s.setRest(rest[n:])
		return n, nil
	}
	if err != nil {
		return 0, err
	}
	for {
		select {
		case <-cancel:
			return 0, errCancelled
		case c := <-s.chunks:
			if c.stale {
				continue
			}
			n := copy(p, c.b)
			s.setRest(c.b[n:])
			return n, nil
		case err := <-s.errc:
			s.mu.Lock()
			s.err = err
			s.mu.Unlock()
			return 0, err
		}
	}
}

func (s *bgSource) setRest(b []byte) {
	s.mu.Lock()
	s.rest = b
	s.mu.Unlock()
}

func (s *bgSource) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused.Store(true)
	select {
	case <-s.cancel:
	default:
		close(s.cancel)
	}
}

func (s *bgSource) Resume() {
	s.mu.Lock()
	s.cancel = make(chan struct{})
	s.rest = nil
	s.paused.Store(false)
	s.mu.Unlock()
}
