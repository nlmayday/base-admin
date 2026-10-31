package observability

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/requestid"
)

const (
	checkOK   = "ok"
	checkFail = "fail"
)

// Checker probes one dependency.
type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

type funcChecker struct {
	name string
	fn   func(context.Context) error
}

func (f funcChecker) Name() string                    { return f.name }
func (f funcChecker) Check(ctx context.Context) error { return f.fn(ctx) }

// NewCheck adapts a function to a Checker.
func NewCheck(name string, fn func(context.Context) error) Checker {
	return funcChecker{name: name, fn: fn}
}

// HealthOptions tunes readiness checks.
type HealthOptions struct {
	// Timeout bounds all checks of one probe.
	Timeout time.Duration
	// CacheTTL reuses the last result so that probe floods (the endpoint
	// is unauthenticated) cannot pin database and Redis connections.
	CacheTTL time.Duration
}

type readiness struct {
	at      time.Time
	results map[string]string // shared read-only between responses
	healthy bool
}

// Health serves liveness and readiness endpoints.
type Health struct {
	logger   *zap.Logger
	opts     HealthOptions
	checkers []Checker
	draining atomic.Bool

	mu   sync.Mutex // serializes checks; waiters reuse the fresh result
	last readiness
}

// NewHealth creates a Health whose readiness depends on all checkers
// passing within opts.Timeout.
func NewHealth(logger *zap.Logger, opts HealthOptions, checkers ...Checker) *Health {
	return &Health{logger: logger, opts: opts, checkers: checkers}
}

// SetDraining makes readiness fail so load balancers stop routing traffic
// before shutdown. It cannot be undone.
func (h *Health) SetDraining() { h.draining.Store(true) }

// Live reports that the process is running.
func (h *Health) Live(c *gin.Context) {
	httpx.OK(c, gin.H{"status": checkOK})
}

// Ready reports whether the instance can serve traffic. Only check names
// and pass/fail are returned; failure details go to the log.
func (h *Health) Ready(c *gin.Context) {
	if h.draining.Load() {
		httpx.Fail(c, httpx.Unavailable("draining"))
		return
	}
	results, healthy := h.readiness(c.Request.Context())
	if !healthy {
		httpx.Fail(c, httpx.Unavailable("dependency check failed").WithDetails(gin.H{"checks": results}))
		return
	}
	httpx.OK(c, gin.H{"status": checkOK, "checks": results})
}

func (h *Health) readiness(ctx context.Context) (map[string]string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.last.at.IsZero() && time.Since(h.last.at) < h.opts.CacheTTL {
		return h.last.results, h.last.healthy
	}
	results, healthy := h.runChecks(ctx)
	h.last = readiness{at: time.Now(), results: results, healthy: healthy}
	return results, healthy
}

func (h *Health) runChecks(parent context.Context) (map[string]string, bool) {
	ctx, cancel := context.WithTimeout(parent, h.opts.Timeout)
	defer cancel()

	errs := make([]error, len(h.checkers))
	var wg sync.WaitGroup
	for i, checker := range h.checkers {
		wg.Go(func() { errs[i] = checker.Check(ctx) })
	}
	wg.Wait()

	results := make(map[string]string, len(h.checkers))
	healthy := true
	for i, checker := range h.checkers {
		if errs[i] == nil {
			results[checker.Name()] = checkOK
			continue
		}
		healthy = false
		results[checker.Name()] = checkFail
		h.logger.Warn("readiness check failed",
			zap.String("request_id", requestid.From(parent)),
			zap.String("check", checker.Name()),
			zap.Error(errs[i]),
		)
	}
	return results, healthy
}
