// Helpers the local servers share: `apic demo` and `apic mock`.

package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/hungovercoders/apic/internal/runner"
)

// listenLocal binds 127.0.0.1:port. It runs before anything is written or
// printed, so a port already in use fails first (E214 names the command).
func listenLocal(port int) (net.Listener, error) {
	if port < 1 || port > 65535 {
		return nil, runner.Usage(runner.CodeFlag, "--port must be between 1 and 65535")
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, runner.Usage(runner.CodeServer, err.Error())
	}
	return ln, nil
}

// serveLocal serves handler on ln until ctx is done (Ctrl-C), then shuts
// down cleanly. Timeouts rather than http.ListenAndServe: these servers are
// short-lived and local, but a server with no read timeout is still a
// server a single stalled connection can tie up.
func serveLocal(ctx context.Context, ln net.Listener, handler http.Handler) error {
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		// ctx is done by now; the shutdown gets its own deadline.
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return runner.Usage(runner.CodeServer, err.Error())
	}
	return nil
}
