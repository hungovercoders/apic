package ui

import (
	"errors"
	"io"
	"sync"
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
// editor can be lost to it; Resume drops them rather than letting them
// reach the model as keys.
type bgSource struct {
	chunks chan []byte
	errc   chan error

	mu     sync.Mutex
	cancel chan struct{} // closed by Cancel, replaced by Resume
	err    error         // the reader's final error, once it has one
	rest   []byte        // the tail of a chunk a short Read left over
}

func newBgSource(r io.Reader) *bgSource {
	s := &bgSource{chunks: make(chan []byte, 8), errc: make(chan error, 1), cancel: make(chan struct{})}
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				s.chunks <- append([]byte(nil), buf[:n]...)
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
	select {
	case <-cancel:
		return 0, errCancelled
	case b := <-s.chunks:
		n := copy(p, b)
		s.setRest(b[n:])
		return n, nil
	case err := <-s.errc:
		s.mu.Lock()
		s.err = err
		s.mu.Unlock()
		return 0, err
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
	s.mu.Unlock()
	for {
		select {
		case <-s.chunks:
		default:
			return
		}
	}
}
