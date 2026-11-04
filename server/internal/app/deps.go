// Package app wires configuration, infrastructure and HTTP servers together
// and implements the admin binary's commands.
package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/redisx"
)

// deps holds shared infrastructure clients.
type deps struct {
	sqlDB *sql.DB
	db    *gorm.DB
	redis *redis.Client
	// extra are the databases of the business modules (extra_db.go);
	// missingExtra the names modules asked for that are not configured.
	extra        map[string]*extraDB
	missingExtra []string
}

func openDeps(ctx context.Context, cfg config.Config, logger *zap.Logger) (*deps, error) {
	sqlDB, err := database.OpenSQL(ctx, cfg.Database, cfg.App.Name)
	if err != nil {
		return nil, err
	}
	gdb, err := database.NewGorm(sqlDB, cfg.Database, logger)
	if err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	rdb, err := redisx.Open(ctx, cfg.Redis, logger)
	if err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	extra, err := openExtraDBs(ctx, cfg, logger)
	if err != nil {
		_ = rdb.Close()
		_ = sqlDB.Close()
		return nil, err
	}
	return &deps{sqlDB: sqlDB, db: gdb, redis: rdb, extra: extra}, nil
}

func (d *deps) close() error {
	var errs []error
	if err := d.redis.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close redis: %w", err))
	}
	if err := d.sqlDB.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close database: %w", err))
	}
	errs = append(errs, closeExtraDBs(d.extra))
	return errors.Join(errs...)
}
