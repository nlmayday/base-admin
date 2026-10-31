package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// listener pairs an HTTP server with an already bound socket, so bind
// errors surface before the service reports itself as started.
type listener struct {
	name string
	srv  *http.Server
	ln   net.Listener
}

func listen(name, addr string, srv *http.Server) (listener, error) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
	if err != nil {
		return listener{}, fmt.Errorf("listen %s on %s: %w", name, addr, err)
	}
	return listener{name: name, srv: srv, ln: ln}, nil
}

// shutdownPlan controls graceful shutdown.
type shutdownPlan struct {
	// onDrain runs first, e.g. to make readiness fail.
	onDrain func()
	// drainDelay gives load balancers time to stop routing traffic.
	drainDelay time.Duration
	// timeout bounds waiting for in-flight requests.
	timeout time.Duration
}

// runServers serves until ctx is cancelled or a server fails, then shuts
// every server down gracefully.
func runServers(ctx context.Context, logger *zap.Logger, plan shutdownPlan, listeners ...listener) error {
	errCh := make(chan error, len(listeners))
	for _, l := range listeners {
		logger.Info("server started", zap.String("server", l.name), zap.String("addr", l.ln.Addr().String()))
		go func() {
			if err := l.srv.Serve(l.ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("%s server: %w", l.name, err)
			}
		}()
	}

	var serveErr error
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case serveErr = <-errCh:
		logger.Error("server failed, shutting down", zap.Error(serveErr))
	}

	return errors.Join(serveErr, shutdown(logger, plan, listeners))
}

func shutdown(logger *zap.Logger, plan shutdownPlan, listeners []listener) error {
	if plan.onDrain != nil {
		plan.onDrain()
	}
	if plan.drainDelay > 0 {
		logger.Info("draining before shutdown", zap.Duration("delay", plan.drainDelay))
		time.Sleep(plan.drainDelay)
	}

	ctx, cancel := context.WithTimeout(context.Background(), plan.timeout)
	defer cancel()

	// All servers share one deadline, so they are stopped concurrently.
	errs := make([]error, len(listeners))
	var wg sync.WaitGroup
	for i, l := range listeners {
		wg.Go(func() { errs[i] = stopServer(ctx, l) })
	}
	wg.Wait()
	logger.Info("servers stopped")
	return errors.Join(errs...)
}

// stopServer waits for in-flight requests until ctx expires, then closes the
// remaining connections. Closing cancels their request contexts, so handlers
// that honour ctx stop before the shared dependencies are closed.
func stopServer(ctx context.Context, l listener) error {
	err := l.srv.Shutdown(ctx)
	if err == nil {
		return nil
	}
	return errors.Join(
		fmt.Errorf("shutdown %s server: %w", l.name, err),
		l.srv.Close(),
	)
}
