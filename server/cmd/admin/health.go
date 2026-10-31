package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"base-admin/server/internal/app"
)

// healthTimeout bounds one probe; container healthchecks run it repeatedly.
const healthTimeout = 3 * time.Second

// probeHealth requests /health/<kind> ("live" or "ready") from the server
// listening on addr (http.addr, e.g. ":8080"), for container healthchecks:
// the distroless image has no shell or curl.
func probeHealth(ctx context.Context, addr string, args []string) error {
	kind := "ready"
	switch {
	case len(args) == 1 && (args[0] == "live" || args[0] == "ready"):
		kind = args[0]
	case len(args) > 0:
		return fmt.Errorf("%w: health takes live or ready", app.ErrUsage)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("http.addr %q: %w", addr, err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	url := "http://" + net.JoinHostPort(host, port) + "/health/" + kind
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("health %s: %w", kind, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health %s: HTTP %d", kind, resp.StatusCode)
	}
	return nil
}
