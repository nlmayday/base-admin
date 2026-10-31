package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"go.uber.org/zap"

	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/observability"
	"base-admin/server/internal/platform/password"
)

// readinessOptions bounds readiness probes; see observability.HealthOptions.
var readinessOptions = observability.HealthOptions{Timeout: 2 * time.Second, CacheTTL: time.Second}

// Serve runs the API server (and the metrics server when enabled) until ctx
// is cancelled.
func Serve(ctx context.Context, cfg config.Config, logger *zap.Logger) (err error) {
	if cfg.App.Env != config.EnvDev {
		gin.SetMode(gin.ReleaseMode)
	}

	d, err := openDeps(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, d.close()) }()

	metrics := observability.NewMetrics()
	if err := metrics.Register(collectors.NewDBStatsCollector(d.sqlDB, "postgres")); err != nil {
		return fmt.Errorf("register database metrics: %w", err)
	}
	health := observability.NewHealth(logger, readinessOptions,
		observability.NewCheck("postgres", d.sqlDB.PingContext),
		observability.NewCheck("redis", func(ctx context.Context) error { return d.redis.Ping(ctx).Err() }),
	)

	application, err := buildApplication(ctx, cfg, logger, d, metrics, health, password.Default)
	if err != nil {
		return err
	}
	defer func() {
		if err := application.close(); err != nil {
			logger.Warn("release module resources", zap.Error(err))
		}
	}()
	reconcileCtx, stopReconciler := context.WithCancel(ctx)
	reconcilerDone := make(chan struct{})
	go func() {
		defer close(reconcilerDone)
		application.core.Versions.RunReconciler(reconcileCtx, cfg.Auth.ReconcileInterval)
	}()
	// Waited for, so it never queries after the database is closed.
	defer func() { stopReconciler(); <-reconcilerDone }()

	// The audit writer must outlive ctx: requests keep draining after the
	// stop signal. It is stopped (and flushed) after the servers, before the
	// deferred dependency close.
	stopBackground := startBackground(ctx, application)
	defer stopBackground()

	listeners, err := bindListeners(cfg, application.engine, metrics, logger)
	if err != nil {
		return err
	}
	return runServers(ctx, logger, shutdownPlan{
		onDrain:    health.SetDraining,
		drainDelay: cfg.HTTP.DrainDelay,
		timeout:    cfg.HTTP.ShutdownTimeout,
	}, listeners...)
}

func bindListeners(cfg config.Config, api http.Handler, metrics *observability.Metrics, logger *zap.Logger) ([]listener, error) {
	errorLog, err := zap.NewStdLogAt(logger.Named("http"), zap.WarnLevel)
	if err != nil {
		return nil, fmt.Errorf("create http error logger: %w", err)
	}
	apiLn, err := listen("api", cfg.HTTP.Addr, newAPIServer(cfg.HTTP, api, errorLog))
	if err != nil {
		return nil, err
	}
	if !cfg.Metrics.Enabled {
		return []listener{apiLn}, nil
	}
	metricsLn, err := listen("metrics", cfg.Metrics.Addr, newMetricsServer(metrics.Handler(), errorLog))
	if err != nil {
		_ = apiLn.ln.Close()
		return nil, err
	}
	return []listener{apiLn, metricsLn}, nil
}

func newAPIServer(cfg config.HTTPConfig, handler http.Handler, errorLog *log.Logger) *http.Server {
	return &http.Server{
		Handler:           handler,
		ErrorLog:          errorLog,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}
}

func newMetricsServer(handler http.Handler, errorLog *log.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", handler)
	return &http.Server{
		Handler:           mux,
		ErrorLog:          errorLog,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
}

// startBackground runs long-lived workers that must keep going while
// in-flight requests drain, and returns a function that stops them and
// waits for them to finish. It runs after the HTTP servers have drained and
// before the database and Redis close.
func startBackground(ctx context.Context, application *application) func() {
	bgCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	var wg sync.WaitGroup
	workers := []func(context.Context){
		application.audit.Writer().Run,
		// The scheduler waits for in-flight job runs, each bounded by its timeout.
		application.ops.Run,
	}
	for _, run := range workers {
		wg.Go(func() { run(bgCtx) })
	}
	return func() {
		cancel()
		wg.Wait()
	}
}
