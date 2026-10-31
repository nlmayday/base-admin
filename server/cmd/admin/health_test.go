package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/app"
)

func TestProbeHealth(t *testing.T) {
	ready := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health/live":
		case r.URL.Path == "/health/ready" && ready:
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(srv.Close)
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
	ctx := context.Background()

	assert.NoError(t, probeHealth(ctx, ":"+port, nil), "an empty host means localhost")
	assert.NoError(t, probeHealth(ctx, "0.0.0.0:"+port, []string{"live"}))
	ready = false
	assert.ErrorContains(t, probeHealth(ctx, ":"+port, nil), "HTTP 503")
	assert.NoError(t, probeHealth(ctx, ":"+port, []string{"live"}))
	assert.ErrorIs(t, probeHealth(ctx, ":"+port, []string{"bogus"}), app.ErrUsage)
	srv.Close()
	assert.Error(t, probeHealth(ctx, ":"+port, nil), "nothing listening")
}
