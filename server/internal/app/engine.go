package app

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/middleware"
	"base-admin/server/internal/platform/observability"
)

// routeLimits are per-route overrides of the global request limits; a nil
// field uses the configured default.
type routeLimits struct {
	bodyBytes func(*gin.Context) int64
	timeout   func(*gin.Context) time.Duration
}

// APIPrefix is the base path of every business API. Modules register their
// routes under it in step 2 (module registry).
const APIPrefix = "/api/v1"

// newEngine builds the Gin engine with the middleware chain from design doc
// section 3: RequestID → AccessLog → metrics → Recover → security headers →
// body limit → timeout. AccessLog and metrics sit outside Recover so that
// panics are still logged and counted as 500s.
// limits resolves per-route overrides; its zero value uses the configured
// defaults for every route.
func newEngine(cfg config.HTTPConfig, logger *zap.Logger, metrics *observability.Metrics, health *observability.Health,
	limits routeLimits) (*gin.Engine, error) {
	bodyLimit := limits.bodyBytes
	if bodyLimit == nil {
		bodyLimit = func(*gin.Context) int64 { return cfg.MaxBodyBytes }
	}
	if err := httpx.RegisterJSONFieldNames(); err != nil {
		return nil, err
	}

	engine := gin.New()
	if err := engine.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, fmt.Errorf("set trusted proxies: %w", err)
	}
	engine.ContextWithFallback = true
	engine.HandleMethodNotAllowed = false

	engine.Use(
		middleware.RequestID(),
		middleware.AccessLog(logger),
		metrics.Middleware(),
		middleware.Recover(logger),
		middleware.SecurityHeaders(),
		middleware.BodyLimitFunc(bodyLimit),
		middleware.TimeoutFunc(cfg.RequestTimeout, limits.timeout),
	)
	engine.NoRoute(httpx.NoRoute)

	engine.GET("/health/live", health.Live)
	engine.GET("/health/ready", health.Ready)
	return engine, nil
}
