package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/observability"
)

// extraDB is an open extra database of the business modules
// (config.ExtraDatabaseConfig).
type extraDB struct {
	sqlDB    *sql.DB
	db       *gorm.DB
	required bool
}

// openExtraDBs opens every database.extra.<name>. A required database must
// answer a ping at startup; the others only log a warning and are used
// once they come up (the pool reconnects).
func openExtraDBs(ctx context.Context, cfg config.Config, logger *zap.Logger) (map[string]*extraDB, error) {
	names := make([]string, 0, len(cfg.Database.Extra))
	for name := range cfg.Database.Extra {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make(map[string]*extraDB, len(names))
	fail := func(err error) (map[string]*extraDB, error) {
		_ = closeExtraDBs(out)
		return nil, err
	}
	for _, name := range names {
		ec := cfg.Database.Extra[name]
		sqlDB, err := database.OpenExtra(name, ec, cfg.App.Name)
		if err != nil {
			return fail(err)
		}
		gdb, err := database.NewGorm(sqlDB, config.DatabaseConfig{SlowQueryThreshold: cfg.Database.SlowQueryThreshold},
			logger.Named("extra."+name))
		if err != nil {
			_ = sqlDB.Close()
			return fail(err)
		}
		out[name] = &extraDB{sqlDB: sqlDB, db: gdb, required: ec.Required}
		pingCtx, cancel := context.WithTimeout(ctx, ec.ConnectTimeout)
		err = sqlDB.PingContext(pingCtx)
		cancel()
		switch {
		case err != nil && ec.Required:
			return fail(fmt.Errorf("ping database.extra.%s: %w", name, err))
		case err != nil:
			logger.Warn("extra database unreachable; starting without it", zap.String("database", name), zap.Error(err))
		}
	}
	return out, nil
}

func closeExtraDBs(dbs map[string]*extraDB) error {
	var errs []error
	for name, x := range dbs {
		if err := x.sqlDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close database.extra.%s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// extraMetrics returns the pool metrics of the extra databases, labelled
// db_name="extra_<name>".
func (d *deps) extraMetrics() []prometheus.Collector {
	var out []prometheus.Collector
	for _, name := range d.extraNames() {
		out = append(out, collectors.NewDBStatsCollector(d.extra[name].sqlDB, "extra_"+name))
	}
	return out
}

// extraChecks returns the readiness checks of the required extra databases.
func (d *deps) extraChecks() []observability.Checker {
	var out []observability.Checker
	for _, name := range d.extraNames() {
		if x := d.extra[name]; x.required {
			out = append(out, observability.NewCheck("extra_"+name, x.sqlDB.PingContext))
		}
	}
	return out
}

func (d *deps) extraNames() []string {
	names := make([]string, 0, len(d.extra))
	for name := range d.extra {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// extraDB returns the extra database name for a business module's Deps, as
// in server/internal/app/modules.go:
//
//	game.New(game.Deps{DB: d.db, GameDB: d.extraDB("game"), Biz: cfg.Biz})
//
// An undeclared name is reported by buildApplication (set
// ADMIN_DATABASE_EXTRA_<NAME>_URL), so it never reaches the module as nil
// unnoticed.
func (d *deps) extraDB(name string) *gorm.DB {
	if x, ok := d.extra[name]; ok {
		return x.db
	}
	if !slices.Contains(d.missingExtra, name) {
		d.missingExtra = append(d.missingExtra, name)
	}
	return nil
}

// missingExtraError reports extra databases that modules asked for but the
// configuration does not declare.
func (d *deps) missingExtraError() error {
	var errs []error
	for _, name := range d.missingExtra {
		errs = append(errs, fmt.Errorf("database.extra.%s is used by a module but not configured (set %s)",
			name, config.ExtraURLEnv(name)))
	}
	return errors.Join(errs...)
}
