package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"base-admin/server/internal/core/jobs"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/modules/audit"
	"base-admin/server/internal/modules/auth"
	"base-admin/server/internal/modules/devtools"
	"base-admin/server/internal/modules/file"
	"base-admin/server/internal/modules/iam"
	"base-admin/server/internal/modules/notice"
	"base-admin/server/internal/modules/ops"
	"base-admin/server/internal/modules/system"
	"base-admin/server/internal/platform/buildinfo"
	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/dbmigrate"
	"base-admin/server/internal/platform/observability"
	"base-admin/server/internal/platform/password"
	"base-admin/server/internal/platform/storage"
)

// application is the assembled HTTP application.
type application struct {
	engine   *gin.Engine
	core     *auth.Core
	registry *module.Registry
	modules  []module.Module
	audit    *audit.Module
	ops      *ops.Module
	// closers release resources owned by modules (e.g. the local storage
	// directory handle); close runs them after the servers stop.
	closers []io.Closer
}

// close releases module resources; errors are joined.
func (a *application) close() error {
	var errs []error
	for _, c := range a.closers {
		errs = append(errs, c.Close())
	}
	return errors.Join(errs...)
}

// buildApplication wires modules onto the engine, validates them and syncs
// permissions, menus and seed data (design doc 4.3, 4.4). Business modules
// of downstream projects are appended to the module list here.
func buildApplication(ctx context.Context, cfg config.Config, logger *zap.Logger, d *deps,
	metrics *observability.Metrics, health *observability.Health, pw password.Params) (*application, error) {
	if err := dbmigrate.RequireCurrent(ctx, d.sqlDB, dbmigrate.TracksFor(cfg.Database)); err != nil {
		return nil, err
	}
	systemModule := system.New(system.Deps{DB: d.db, Redis: d.redis, Logger: logger})
	auditModule := audit.New(audit.Deps{DB: d.db, Logger: logger, RetentionDays: func(ctx context.Context) (int, error) {
		return systemModule.GetInt(ctx, system.KeyAuditRetentionDays)
	}})
	if err := metrics.Register(auditModule.Writer().Collectors()...); err != nil {
		return nil, fmt.Errorf("register audit metrics: %w", err)
	}
	core, err := auth.NewCore(auth.Options{
		DB: d.db, Redis: d.redis, Config: cfg.Auth, APIPrefix: APIPrefix, Logger: logger, Password: pw,
		LoginObserver: auditModule.Writer(),
	})
	if err != nil {
		return nil, err
	}
	registry := module.NewRegistry()
	iamModule := iam.New(d.db, core, registry, cfg.Bootstrap)
	// noticeModule also sends personal messages: give it to business modules
	// as a message.Sender (docs/module-guide.md).
	noticeModule := notice.New(notice.Deps{DB: d.db, Dict: systemModule})
	mods := []module.Module{
		auth.NewModule(core, iamModule.Me, auth.WithTargetGuard(iamModule.EnsureManageable)),
		iamModule,
		auditModule,
		systemModule,
		devtools.New(devtools.Deps{
			Registry: registry, DB: d.db, Env: cfg.App.Env, Logger: logger,
			APIPrefix: APIPrefix, SessionCookie: cfg.Auth.CookieName, Database: cfg.Database,
		}),
		noticeModule,
		// Business modules go here. Their configuration is cfg.Biz (biz in
		// the config file, secrets from ADMIN_BIZ_*), their extra databases
		// d.extraDB("<name>") (database.extra.<name>, URL from
		// ADMIN_DATABASE_EXTRA_<NAME>_URL); see docs/module-guide.md.
		// codegen:modules — 代码生成器在此行之上加入新模块
	}
	if err := d.missingExtraError(); err != nil {
		return nil, err
	}
	store, err := storage.New(cfg.File)
	if err != nil {
		return nil, fmt.Errorf("file storage: %w", err)
	}
	mods = append(mods, file.New(file.Deps{DB: d.db, Storage: store, Config: cfg.File, Logger: logger, Policy: systemModule}))
	// ops schedules the jobs every other module contributes, so it is built last.
	opsModule := ops.New(ops.Deps{
		DB: d.db, SQLDB: d.sqlDB, Redis: d.redis, Handlers: jobHandlers(mods), Logger: logger,
		InstanceID: buildinfo.InstanceID(), StartedAt: time.Now(), Version: buildinfo.Version,
	})
	mods = append(mods, opsModule)

	engine, err := newEngine(cfg.HTTP, logger, metrics, health, routeLimitsOf(registry, cfg.HTTP))
	if err != nil {
		return nil, err
	}
	// The audit middleware sits before CSRF so rejected attempts are recorded.
	api := engine.Group(APIPrefix, auditModule.Middleware(registry), core.Guard.CSRF())
	for _, m := range mods {
		m.Routes(module.NewRouter(api, m.Name(), core.Guard, registry))
	}

	report := module.Validate(mods, registry, engine.Routes(), APIPrefix)
	for _, w := range report.Warnings {
		logger.Info("module validation", zap.String("warning", w))
	}
	if err := report.Err(); err != nil {
		return nil, err
	}
	if err := module.Sync(ctx, d.db, mods); err != nil {
		return nil, fmt.Errorf("sync modules: %w", err)
	}
	if _, err := core.Versions.Reconcile(ctx); err != nil {
		return nil, err
	}
	return &application{engine: engine, core: core, registry: registry, modules: mods,
		audit: auditModule, ops: opsModule, closers: closersOf(store)}, nil
}

// closersOf returns the values that hold resources to release on shutdown.
func closersOf(values ...any) []io.Closer {
	var out []io.Closer
	for _, v := range values {
		if c, ok := v.(io.Closer); ok {
			out = append(out, c)
		}
	}
	return out
}

// jobHandlers collects the scheduled-job handlers of every module that
// implements jobs.Provider.
func jobHandlers(mods []module.Module) []jobs.Handler {
	var handlers []jobs.Handler
	for _, m := range mods {
		if p, ok := m.(jobs.Provider); ok {
			handlers = append(handlers, p.Jobs()...)
		}
	}
	return handlers
}

// routeLimitsOf resolves the global, pre-authentication limits: the default
// body limit (routes declaring MaxBody are limited later, after the guard)
// and the request context deadline (a route's Timeout, else the default).
func routeLimitsOf(reg *module.Registry, cfg config.HTTPConfig) routeLimits {
	return routeLimits{
		bodyBytes: func(c *gin.Context) int64 {
			if r, ok := reg.Find(c.Request.Method, c.FullPath()); ok && r.MaxBodyBytes > 0 {
				return 0 // limited by the module router after the access guard
			}
			return cfg.MaxBodyBytes
		},
		timeout: func(c *gin.Context) time.Duration {
			if r, ok := reg.Find(c.Request.Method, c.FullPath()); ok && r.Timeout > 0 {
				return r.Timeout
			}
			return cfg.RequestTimeout
		},
	}
}
