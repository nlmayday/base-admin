package ops

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// collectTimeout bounds one statistics collection.
const collectTimeout = 3 * time.Second

// cached memoizes an expensive collection for ttl. Concurrent callers wait
// for the one collection in progress instead of starting their own.
type cached[T any] struct {
	mu  sync.Mutex
	ttl time.Duration
	at  time.Time
	val T
}

func (c *cached[T]) get(ctx context.Context, collect func(context.Context) (T, error)) (T, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && time.Since(c.at) < c.ttl {
		return c.val, nil
	}
	ctx, cancel := context.WithTimeout(ctx, collectTimeout)
	defer cancel()
	v, err := collect(ctx)
	if err != nil {
		var zero T
		return zero, err
	}
	c.val, c.at = v, time.Now()
	return v, nil
}

type monitor struct {
	sqlDB     *sql.DB
	rdb       *redis.Client
	logger    *zap.Logger
	instance  string
	version   string
	startedAt time.Time

	cpuPrimed bool // guarded by server.mu
	server    cached[serverStats]
	app       cached[appStats]
}

func newMonitor(d Deps, ttl time.Duration) *monitor {
	return &monitor{
		sqlDB: d.SQLDB, rdb: d.Redis, logger: d.Logger.Named("ops.monitor"),
		instance: d.InstanceID, version: d.Version, startedAt: d.StartedAt,
		server: cached[serverStats]{ttl: ttl}, app: cached[appStats]{ttl: ttl},
	}
}

func (m *Module) serverStats(c *gin.Context) (any, error) {
	return m.monitor.server.get(c.Request.Context(), m.monitor.collectServer)
}

func (m *Module) appStats(c *gin.Context) (any, error) {
	return m.monitor.app.get(c.Request.Context(), m.monitor.collectApp)
}
