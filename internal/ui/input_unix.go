//go:build unix

package ui

import (
	"errors"
	"os"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

// pollSource reads the terminal through poll(2), waiting on it and on a
// pipe of its own at once. Cancel writes to the pipe, so a pending Read
// returns without having taken a byte from the terminal: whatever is typed
// next goes to the editor, untouched.
type pollSource struct {
	f         *os.File
	fd        int32 // f's descriptor, for poll
	pipe      [2]int
	cancelled atomic.Bool
}

func newKeySource(f *os.File) (keySource, error) {
	var p [2]int
	if err := unix.Pipe(p[:]); err != nil {
		return nil, err
	}
	for _, fd := range p {
		unix.CloseOnExec(fd)
		if err := unix.SetNonblock(fd, true); err != nil {
			_ = unix.Close(p[0])
			_ = unix.Close(p[1])
			return nil, err
		}
	}
	return &pollSource{f: f, fd: int32(f.Fd()), pipe: p}, nil //nolint:gosec // a file descriptor fits in an int32
}

func (s *pollSource) Read(b []byte) (int, error) {
	for {
		fds := []unix.PollFd{
			{Fd: s.fd, Events: unix.POLLIN},
			{Fd: int32(s.pipe[0]), Events: unix.POLLIN}, //nolint:gosec // a file descriptor fits in an int32
		}
		if _, err := unix.Poll(fds, -1); err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return 0, err
		}
		if fds[1].Revents != 0 {
			s.drain()
			return 0, errCancelled
		}
		if fds[0].Revents != 0 {
			return s.f.Read(b)
		}
	}
}

func (s *pollSource) Cancel() {
	if s.cancelled.CompareAndSwap(false, true) {
		_, _ = unix.Write(s.pipe[1], []byte{0})
	}
}

func (s *pollSource) Resume() {
	s.drain()
	s.cancelled.Store(false)
}

// drain empties the cancel pipe, which is non-blocking.
func (s *pollSource) drain() {
	var b [16]byte
	for {
		if n, err := unix.Read(s.pipe[0], b[:]); n <= 0 || err != nil {
			return
		}
	}
}
