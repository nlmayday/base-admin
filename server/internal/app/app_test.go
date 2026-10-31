package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/observability"
	"base-admin/server/internal/platform/requestid"
)

func init() { gin.SetMode(gin.TestMode) }

func testHTTPConfig() config.HTTPConfig {
	return config.HTTPConfig{
		Addr:              "127.0.0.1:0",
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       time.Second,
		RequestTimeout:    time.Second,
		MaxBodyBytes:      1 << 10,
		ShutdownTimeout:   5 * time.Second,
	}
}

func TestEngineRoutesAndMiddleware(t *testing.T) {
	health := observability.NewHealth(zap.NewNop(), observability.HealthOptions{Timeout: time.Second})
	engine, err := newEngine(testHTTPConfig(), zap.NewNop(), observability.NewMetrics(), health, routeLimits{})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Header().Get(requestid.Header))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))

	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	assert.Equal(t, http.StatusOK, rec.Code, "no checkers means ready")

	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/health/live", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code, "wrong method must be 404, not 405")
	assert.Contains(t, rec.Body.String(), `"code":10003`)
}

func TestEngineRejectsInvalidTrustedProxy(t *testing.T) {
	cfg := testHTTPConfig()
	cfg.TrustedProxies = []string{"not-an-ip"}
	_, err := newEngine(cfg, zap.NewNop(), observability.NewMetrics(), observability.NewHealth(zap.NewNop(), observability.HealthOptions{Timeout: time.Second}), routeLimits{})
	assert.Error(t, err)
}

func TestRunServersGracefulShutdown(t *testing.T) {
	started := make(chan struct{})
	slow := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(w, "done")
	})
	l, err := listen("api", "127.0.0.1:0", &http.Server{Handler: slow, ReadHeaderTimeout: time.Second})
	require.NoError(t, err)

	var drained atomic.Bool
	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() {
		runErr <- runServers(ctx, zap.NewNop(), shutdownPlan{
			onDrain:    func() { drained.Store(true) },
			drainDelay: 10 * time.Millisecond,
			timeout:    5 * time.Second,
		}, l)
	}()

	respCh := make(chan string, 1)
	go func() {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+l.ln.Addr().String(), nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			respCh <- "error: " + err.Error()
			return
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		respCh <- string(body)
	}()

	<-started
	cancel()

	assert.Equal(t, "done", <-respCh, "in-flight request must complete")
	require.NoError(t, <-runErr)
	assert.True(t, drained.Load())
}

func TestRunServersReportsServeFailure(t *testing.T) {
	l, err := listen("api", "127.0.0.1:0", &http.Server{ReadHeaderTimeout: time.Second})
	require.NoError(t, err)
	require.NoError(t, l.ln.Close(), "closing the socket makes Serve fail")

	err = runServers(context.Background(), zap.NewNop(), shutdownPlan{timeout: time.Second}, l)
	assert.ErrorContains(t, err, "api server")
}

func TestListenFailsOnBusyPort(t *testing.T) {
	first, err := listen("api", "127.0.0.1:0", &http.Server{ReadHeaderTimeout: time.Second})
	require.NoError(t, err)
	defer func() { _ = first.ln.Close() }()

	_, err = listen("api", first.ln.Addr().String(), &http.Server{ReadHeaderTimeout: time.Second})
	assert.Error(t, err)
}

func TestParseMigrateArgs(t *testing.T) {
	valid := []struct {
		args   []string
		action string
		n      int
	}{
		{[]string{"up"}, "up", 0},
		{[]string{"version"}, "version", 0},
		{[]string{"down", "2"}, "down", 2},
		{[]string{"force", "-1"}, "force", -1},
		{[]string{"force", "3"}, "force", 3},
	}
	for _, tc := range valid {
		action, n, err := parseMigrateArgs(tc.args)
		require.NoError(t, err, tc.args)
		assert.Equal(t, tc.action, action)
		assert.Equal(t, tc.n, n)
	}

	invalid := [][]string{
		nil, {"sideways"}, {"up", "1"}, {"version", "x"},
		{"down"}, {"down", "0"}, {"down", "-1"}, {"down", "x"}, {"force", "-2"}, {"force"},
	}
	for _, args := range invalid {
		_, _, err := parseMigrateArgs(args)
		assert.True(t, errors.Is(err, ErrUsage), "args %v: %v", args, err)
	}
}

func TestEngineCountsPanicsInMetrics(t *testing.T) {
	metrics := observability.NewMetrics()
	engine, err := newEngine(testHTTPConfig(), zap.NewNop(), metrics, observability.NewHealth(zap.NewNop(), observability.HealthOptions{Timeout: time.Second}), routeLimits{})
	require.NoError(t, err)
	engine.GET("/panic", func(*gin.Context) { panic("boom") })

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	scrape := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(scrape, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	assert.Contains(t, scrape.Body.String(), `http_requests_total{method="GET",route="/panic",status="500"} 1`)
}

func TestEngineClientIPAndTrustedProxies(t *testing.T) {
	cases := []struct {
		name    string
		proxies []string
		want    string
	}{
		{"no trusted proxy ignores X-Forwarded-For", nil, "203.0.113.9"},
		{"empty list ignores X-Forwarded-For", []string{}, "203.0.113.9"},
		{"untrusted peer ignores X-Forwarded-For", []string{"10.0.0.0/8"}, "203.0.113.9"},
		{"trusted peer honours X-Forwarded-For", []string{"203.0.113.0/24"}, "1.2.3.4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testHTTPConfig()
			cfg.TrustedProxies = tc.proxies
			engine, err := newEngine(cfg, zap.NewNop(), observability.NewMetrics(), observability.NewHealth(zap.NewNop(), observability.HealthOptions{Timeout: time.Second}), routeLimits{})
			require.NoError(t, err)
			var got string
			engine.GET("/ip", func(c *gin.Context) { got = c.ClientIP() })

			req := httptest.NewRequest(http.MethodGet, "/ip", nil)
			req.RemoteAddr = "203.0.113.9:4000"
			req.Header.Set("X-Forwarded-For", "1.2.3.4")
			engine.ServeHTTP(httptest.NewRecorder(), req)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDrainHappensBeforeListenerStops(t *testing.T) {
	var drained atomic.Bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if drained.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	l, err := listen("api", "127.0.0.1:0", &http.Server{Handler: handler, ReadHeaderTimeout: time.Second})
	require.NoError(t, err)
	url := "http://" + l.ln.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() {
		runErr <- runServers(ctx, zap.NewNop(), shutdownPlan{
			onDrain:    func() { drained.Store(true) },
			drainDelay: 500 * time.Millisecond,
			timeout:    time.Second,
		}, l)
	}()
	require.Eventually(t, func() bool { return statusCode(url) == http.StatusOK }, time.Second, 10*time.Millisecond)

	cancel()
	require.Eventually(t, drained.Load, time.Second, 5*time.Millisecond)
	assert.Equal(t, http.StatusServiceUnavailable, statusCode(url),
		"during the drain delay the listener must still accept and report draining")
	require.NoError(t, <-runErr)
	assert.Equal(t, 0, statusCode(url), "listener is closed after shutdown")
}

func TestShutdownTimeoutForceClosesConnections(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	started := make(chan struct{})
	stuck := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	l, err := listen("api", "127.0.0.1:0", &http.Server{Handler: stuck, ReadHeaderTimeout: time.Second})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() {
		runErr <- runServers(ctx, zap.NewNop(), shutdownPlan{timeout: 100 * time.Millisecond}, l)
	}()
	clientErr := make(chan error, 1)
	go func() {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+l.ln.Addr().String(), nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
		clientErr <- err
	}()

	<-started
	cancel()
	select {
	case err := <-runErr:
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(5 * time.Second):
		t.Fatal("runServers did not return after the shutdown timeout")
	}
	select {
	case err := <-clientErr:
		assert.Error(t, err, "the stuck connection must be force-closed")
	case <-time.After(2 * time.Second):
		t.Fatal("the stuck connection was not force-closed")
	}
}

func statusCode(url string) int {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return 0
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}
