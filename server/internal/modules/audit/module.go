// Package audit implements the login log, the operation log, log
// statistics and the log retention job (docs/p1-p3-plan.md, section 3).
//
// Logs are written asynchronously by Writer: the auth module reports login
// events through auth.LoginObserver and Middleware records API operations.
// Logs are read-only through the API; only the retention job deletes rows.
package audit

import (
	"context"
	"slices"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/jobs"
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/core/perm"
	"base-admin/server/internal/platform/httpx"
)

// Deps are the audit module's dependencies.
type Deps struct {
	DB *gorm.DB
	// Logger is optional; nil discards logs.
	Logger *zap.Logger
	// Writer tunes the asynchronous writer; zero values use defaults.
	Writer WriterConfig
	// Now is optional and only overridden by tests.
	Now func() time.Time
	// RetentionDays is optional; when set, the cleanup job uses it whenever
	// its own params do not specify days (typically the system setting).
	RetentionDays func(ctx context.Context) (int, error)
}

// Module is the audit module.
type Module struct {
	db     *gorm.DB
	logger *zap.Logger
	writer *Writer
	now    func() time.Time
	// retention supplies the default cleanup window; nil uses defaultRetainDays.
	retention func(ctx context.Context) (int, error)
}

var (
	_ module.Module = (*Module)(nil)
	_ jobs.Provider = (*Module)(nil)
)

// New creates the audit module. Start Writer().Run(ctx) to persist logs.
func New(d Deps) *Module {
	logger := d.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	now := d.Now
	if now == nil {
		now = time.Now
	}
	logger = logger.Named("audit")
	return &Module{
		db:        d.DB,
		logger:    logger,
		writer:    newWriter(d.Writer, dbSink{db: d.DB}, logger),
		now:       now,
		retention: d.RetentionDays,
	}
}

// Writer returns the asynchronous log writer; pass it to auth as the
// LoginObserver and run it with Run(ctx).
func (m *Module) Writer() *Writer { return m.writer }

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Permissions implements module.Module.
func (m *Module) Permissions() []perm.Def { return slices.Clone(permissionDefs) }

// Menus implements module.Module.
func (m *Module) Menus() []menu.Def { return slices.Clone(menuDefs) }

// Seed implements module.Module; the audit module has no seed data.
func (m *Module) Seed(context.Context, *gorm.DB) error { return nil }

// Routes implements module.Module.
func (m *Module) Routes(r *module.Router) {
	g := r.Group("/audit")
	g.GET("/login-logs", access.Perm(PermLoginLogView), handle(m.listLoginLogs)).
		Doc(module.Doc{Summary: "登录日志列表", Query: loginFilter{}, Response: httpx.Page[loginLogView]{}})
	g.GET("/login-logs/export", access.Perm(PermLoginLogExport), m.exportLoginLogs).
		Doc(module.Doc{Summary: "导出登录日志", Query: loginFilter{}, Sensitive: true})

	g.GET("/operation-logs", access.Perm(PermOperationLogView), handle(m.listOperationLogs)).
		Doc(module.Doc{Summary: "操作日志列表", Query: opFilter{}, Response: httpx.Page[opLogView]{}})
	g.GET("/operation-logs/modules", access.Perm(PermOperationLogView), handle(m.listModules)).
		Doc(module.Doc{Summary: "模块名称", Description: "各模块的中文名（取模块目录菜单的标题），用于显示与筛选", Response: []moduleName{}})
	g.GET("/operation-logs/export", access.Perm(PermOperationLogExport), m.exportOperationLogs).
		Doc(module.Doc{Summary: "导出操作日志", Query: opFilter{}, Sensitive: true})
	g.GET("/operation-logs/:id", access.Perm(PermOperationLogView), handle(m.getOperationLog)).
		Doc(module.Doc{Summary: "操作日志详情", Response: opLogDetail{}})

	g.GET("/stats", access.AnyPerm(PermLoginLogView, PermOperationLogView), handle(m.stats)).
		Doc(module.Doc{Summary: "日志统计", Response: statsView{}})
}

// Jobs implements jobs.Provider.
func (m *Module) Jobs() []jobs.Handler {
	return []jobs.Handler{{
		Key:            cleanupJobKey,
		Name:           "清理审计日志",
		Description:    `删除早于保留天数的登录日志和操作日志。参数留空时使用「基础设置 → 日志保留天数」，也可填 {"days": N} 覆盖`,
		DefaultTimeout: 30 * time.Minute,
		Run:            m.cleanup,
	}}
}
