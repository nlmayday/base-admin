// Package database opens the PostgreSQL connection pool and the GORM handle
// built on top of it. Schema changes are managed by migrations only; GORM
// AutoMigrate must not be used.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"base-admin/server/internal/platform/config"
)

// OpenSQL opens a pgx-backed *sql.DB, applies pool and session settings and
// verifies connectivity. The caller owns the returned pool.
func OpenSQL(ctx context.Context, cfg config.DatabaseConfig, appName string) (*sql.DB, error) {
	pgCfg, err := pgx.ParseConfig(cfg.URL)
	if err != nil {
		// pgx redacts the password in parse errors.
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	pgCfg.ConnectTimeout = cfg.ConnectTimeout
	pgCfg.RuntimeParams["application_name"] = appName
	if cfg.StatementTimeout > 0 {
		pgCfg.RuntimeParams["statement_timeout"] = strconv.FormatInt(cfg.StatementTimeout.Milliseconds(), 10)
	}

	db := stdlib.OpenDB(*pgCfg)
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return db, nil
}

// NewGorm wraps an open pool with GORM. Query parameters are never logged
// so that sensitive values cannot reach the log.
func NewGorm(db *sql.DB, cfg config.DatabaseConfig, logger *zap.Logger) (*gorm.DB, error) {
	stdLog, err := zap.NewStdLogAt(logger.Named("gorm"), zap.WarnLevel)
	if err != nil {
		return nil, fmt.Errorf("create gorm logger: %w", err)
	}
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{
		Logger: gormlogger.New(stdLog, gormlogger.Config{
			SlowThreshold:             cfg.SlowQueryThreshold,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
		}),
		NowFunc:        func() time.Time { return time.Now().UTC() },
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open gorm: %w", err)
	}
	return gdb, nil
}
