// Package ops implements scheduled jobs (a Redis-coordinated scheduler that
// runs handlers registered through jobs.Provider), their run records, and
// the server and application monitor (docs/p1-p3-plan.md, ops).
package ops

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/jobs"
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/core/perm"
	"base-admin/server/internal/platform/httpx"
)

// Deps are the module's dependencies.
type Deps struct {
	DB    *gorm.DB
	SQLDB *sql.DB
	Redis *redis.Client
	// Handlers are collected by the app from every module implementing
	// jobs.Provider. The module's own handlers are always registered, so
	// passing them again is harmless.
	Handlers   []jobs.Handler
	Logger     *zap.Logger
	InstanceID string
	StartedAt  time.Time
	Version    string
	// Tuning is optional; zero fields use the defaults.
	Tuning Tuning
}

// Tuning holds scheduler and monitor intervals. Tests shorten them.
type Tuning struct {
	// ResyncInterval is how often the job list is compared with the database
	// in case a pub/sub notification was missed (default 30s).
	ResyncInterval time.Duration
	// RetryBaseDelay and RetryMaxDelay bound the exponential backoff
	// between attempts (defaults 5s and 5m).
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
	// LockMargin is added to a fire's worst-case duration to get the run
	// record's expiry, after which a run left by a crashed instance is
	// marked failed (default 30s).
	LockMargin time.Duration
	// LockTTL is the job lock's lifetime; a running fire renews it every
	// LockTTL/3, so a crashed instance blocks the job for at most LockTTL
	// (default 60s).
	LockTTL time.Duration
	// MinEvery is the shortest @every interval accepted through the API
	// (default 1m), so a job cannot flood the run history.
	MinEvery time.Duration
	// ShutdownGrace is how long shutdown waits for running jobs before
	// cancelling their context (default 20s); keep it below the
	// orchestrator's termination grace period.
	ShutdownGrace time.Duration
	// MonitorCacheTTL bounds how often statistics are collected (default 2s).
	MonitorCacheTTL time.Duration
}

func (t Tuning) withDefaults() Tuning {
	def := func(v *time.Duration, d time.Duration) {
		if *v <= 0 {
			*v = d
		}
	}
	def(&t.ResyncInterval, 30*time.Second)
	def(&t.RetryBaseDelay, 5*time.Second)
	def(&t.RetryMaxDelay, 5*time.Minute)
	def(&t.LockMargin, 30*time.Second)
	def(&t.LockTTL, time.Minute)
	def(&t.ShutdownGrace, 20*time.Second)
	def(&t.MinEvery, time.Minute)
	def(&t.MonitorCacheTTL, 2*time.Second)
	return t
}

// Module is the ops module.
type Module struct {
	db       *gorm.DB
	handlers handlerSet
	sched    *scheduler
	monitor  *monitor
	builtin  []jobs.Handler
}

var (
	_ module.Module = (*Module)(nil)
	_ jobs.Provider = (*Module)(nil)
)

// New creates the ops module. It panics when a handler is invalid or
// registered twice, which is a programming error found at startup.
func New(d Deps) *Module {
	if d.Logger == nil {
		d.Logger = zap.NewNop()
	}
	if d.StartedAt.IsZero() {
		d.StartedAt = time.Now()
	}
	tuning := d.Tuning.withDefaults()
	m := &Module{db: d.DB}
	m.builtin = []jobs.Handler{m.cleanupHandler()}
	set, err := newHandlerSet(m.builtin, d.Handlers)
	if err != nil {
		panic(fmt.Sprintf("ops: invalid job handlers: %v", err))
	}
	m.handlers = set
	m.sched = newScheduler(d.DB, d.Redis, set, d.Logger.Named("ops.scheduler"), d.InstanceID, tuning)
	m.monitor = newMonitor(d, tuning.MonitorCacheTTL)
	return m
}

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Permissions implements module.Module.
func (m *Module) Permissions() []perm.Def { return slices.Clone(permissionDefs) }

// Menus implements module.Module.
func (m *Module) Menus() []menu.Def { return slices.Clone(menuDefs) }

// Jobs implements jobs.Provider.
func (m *Module) Jobs() []jobs.Handler { return slices.Clone(m.builtin) }

// Seed implements module.Module. The built-in cleanup job is inserted once
// by migration 000009 so that administrators may delete it.
func (m *Module) Seed(context.Context, *gorm.DB) error { return nil }

// Run schedules jobs until ctx is cancelled, then waits for in-flight runs
// (each bounded by its timeout) before returning.
func (m *Module) Run(ctx context.Context) { m.sched.run(ctx) }

// Routes implements module.Module.
func (m *Module) Routes(r *module.Router) {
	jobsGroup := r.Group("/jobs")
	jobsGroup.GET("", access.Perm(PermJobView), handle(m.listJobs)).Doc(module.Doc{Summary: "定时任务列表", Query: jobFilter{}, Response: httpx.Page[jobView]{}})
	jobsGroup.GET("/handlers", access.Perm(PermJobView), handle(m.listHandlers)).Doc(module.Doc{Summary: "任务处理器列表", Response: []handlerView{}})
	jobsGroup.POST("/cron/preview", access.Perm(PermJobView), handle(m.previewCron)).Doc(module.Doc{Summary: "预览 Cron 执行时间", Body: cronPreviewRequest{}, Response: cronPreviewView{}})
	jobsGroup.GET("/:id", access.Perm(PermJobView), handle(m.getJob)).Doc(module.Doc{Summary: "定时任务详情", Response: jobView{}})
	jobsGroup.POST("", access.Perm(PermJobCreate), handle(m.createJob)).Doc(module.Doc{Summary: "新增定时任务", Body: jobRequest{}, Response: jobView{}})
	jobsGroup.PUT("/:id", access.Perm(PermJobUpdate), handle(m.updateJob)).Doc(module.Doc{Summary: "编辑定时任务", Body: jobRequest{}, Response: jobView{}})
	jobsGroup.PUT("/:id/status", access.Perm(PermJobUpdate), handle(m.setJobEnabled)).Doc(module.Doc{Summary: "启用/停用定时任务", Body: jobEnabledRequest{}, Response: jobView{}})
	jobsGroup.DELETE("/:id", access.Perm(PermJobDelete), handle(m.deleteJob)).Doc(module.Doc{Summary: "删除定时任务"})
	jobsGroup.POST("/:id/run", access.Perm(PermJobExecute), handle(m.runJobNow)).Doc(module.Doc{Summary: "立即执行任务", Response: runNowView{}})

	runs := r.Group("/job-runs")
	runs.GET("", access.Perm(PermRunView), handle(m.listRuns)).Doc(module.Doc{Summary: "执行记录列表", Query: runFilter{}, Response: httpx.Page[runView]{}})
	runs.GET("/:id", access.Perm(PermRunView), handle(m.getRun)).Doc(module.Doc{Summary: "执行记录详情", Response: runView{}})

	mon := r.Group("/monitor")
	mon.GET("/server", access.Perm(PermMonitor), handle(m.serverStats)).Doc(module.Doc{Summary: "服务器监控", Response: serverStats{}})
	mon.GET("/app", access.Perm(PermMonitor), handle(m.appStats)).Doc(module.Doc{Summary: "应用监控", Response: appStats{}})
}
