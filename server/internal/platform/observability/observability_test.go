package observability

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"base-admin/server/internal/platform/config"
)

func init() { gin.SetMode(gin.TestMode) }

func TestNewLogger(t *testing.T) {
	for _, format := range []string{"json", "console"} {
		logger, err := NewLogger(config.LogConfig{Level: "warn", Format: format})
		require.NoError(t, err, format)
		assert.False(t, logger.Core().Enabled(zapcore.InfoLevel), format)
		assert.True(t, logger.Core().Enabled(zapcore.WarnLevel), format)
	}
	_, err := NewLogger(config.LogConfig{Level: "loud", Format: "json"})
	assert.Error(t, err)
}

func get(engine *gin.Engine, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func healthEngine(h *Health) *gin.Engine {
	engine := gin.New()
	engine.GET("/health/live", h.Live)
	engine.GET("/health/ready", h.Ready)
	return engine
}

func TestHealthReady(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	ok := NewCheck("postgres", func(context.Context) error { return nil })
	failing := NewCheck("redis", func(context.Context) error { return errors.New("dial tcp: refused") })

	t.Run("all checks pass", func(t *testing.T) {
		rec := get(healthEngine(NewHealth(zap.New(core), HealthOptions{Timeout: time.Second}, ok)), "/health/ready")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"status":"ok","checks":{"postgres":"ok"}}`, dataOf(t, rec))
	})

	t.Run("one check fails", func(t *testing.T) {
		rec := get(healthEngine(NewHealth(zap.New(core), HealthOptions{Timeout: time.Second}, ok, failing)), "/health/ready")
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.JSONEq(t, `{"checks":{"postgres":"ok","redis":"fail"}}`, dataOf(t, rec))
		assert.NotContains(t, rec.Body.String(), "refused", "error details must not leak")
		assert.Equal(t, 1, logs.FilterMessage("readiness check failed").Len())
	})

	t.Run("check respects timeout", func(t *testing.T) {
		slow := NewCheck("slow", func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		})
		start := time.Now()
		rec := get(healthEngine(NewHealth(zap.New(core), HealthOptions{Timeout: 20 * time.Millisecond}, slow)), "/health/ready")
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.Less(t, time.Since(start), time.Second)
	})

	t.Run("draining", func(t *testing.T) {
		h := NewHealth(zap.New(core), HealthOptions{Timeout: time.Second}, ok)
		h.SetDraining()
		engine := healthEngine(h)
		assert.Equal(t, http.StatusServiceUnavailable, get(engine, "/health/ready").Code)
		assert.Equal(t, http.StatusOK, get(engine, "/health/live").Code, "liveness is unaffected")
	})
}

func dataOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Data any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	out, err := json.Marshal(env.Data)
	require.NoError(t, err)
	return string(out)
}

func TestMetrics(t *testing.T) {
	m := NewMetrics()
	engine := gin.New()
	engine.Use(m.Middleware())
	engine.GET("/users/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	get(engine, "/users/1")
	get(engine, "/users/2")
	get(engine, "/nope/123")

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	text := string(body)
	assert.Contains(t, text, `http_requests_total{method="GET",route="/users/:id",status="200"} 2`)
	assert.Contains(t, text, `http_requests_total{method="GET",route="unmatched",status="404"} 1`)
	assert.NotContains(t, text, "/users/1", "raw paths must not become labels")
	assert.Contains(t, text, "go_goroutines")
}

func TestMetricsRegister(t *testing.T) {
	m := NewMetrics()
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: "extra_gauge", Help: "x"})
	require.NoError(t, m.Register(gauge))
	assert.Error(t, m.Register(gauge), "duplicate registration must fail")
}

func TestMetricsFoldsUnknownMethods(t *testing.T) {
	m := NewMetrics()
	engine := gin.New()
	engine.Use(m.Middleware())
	for _, method := range []string{"AAA", "BBB", "CCC"} {
		engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, "/x", nil))
	}
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	text := rec.Body.String()
	assert.Contains(t, text, `http_requests_total{method="OTHER",route="unmatched",status="404"} 3`)
	assert.NotContains(t, text, `method="AAA"`)
}

func TestHealthReadyCachesResults(t *testing.T) {
	var calls atomic.Int32
	counting := NewCheck("postgres", func(context.Context) error {
		calls.Add(1)
		return nil
	})
	engine := healthEngine(NewHealth(zap.NewNop(), HealthOptions{Timeout: time.Second, CacheTTL: time.Hour}, counting))

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { get(engine, "/health/ready") })
	}
	wg.Wait()
	assert.Equal(t, int32(1), calls.Load(), "concurrent and repeated probes must share one check")

	uncached := healthEngine(NewHealth(zap.NewNop(), HealthOptions{Timeout: time.Second}, counting))
	get(uncached, "/health/ready")
	get(uncached, "/health/ready")
	assert.Equal(t, int32(3), calls.Load(), "a zero TTL disables caching")
}
