//go:build integration

package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/observability"
	"base-admin/server/internal/platform/testinfra"
)

func TestReadinessWithRealDependencies(t *testing.T) {
	cfg := config.Config{
		App:      config.AppConfig{Name: "base-admin-test", Env: config.EnvTest},
		HTTP:     testHTTPConfig(),
		Database: testinfra.Database(t),
		Redis:    testinfra.RedisDB(t),
	}
	ctx := context.Background()

	d, err := openDeps(ctx, cfg, zap.NewNop())
	require.NoError(t, err)

	health := observability.NewHealth(zap.NewNop(), observability.HealthOptions{Timeout: time.Second},
		observability.NewCheck("postgres", d.sqlDB.PingContext),
		observability.NewCheck("redis", func(ctx context.Context) error { return d.redis.Ping(ctx).Err() }),
	)
	engine, err := newEngine(cfg.HTTP, zap.NewNop(), observability.NewMetrics(), health, routeLimits{})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"checks":{"postgres":"ok","redis":"ok"}`)

	require.NoError(t, d.close())

	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "closed dependencies must fail readiness")
}
