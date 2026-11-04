// Package database opens the connection pool and the GORM handle built on
// top of it, for PostgreSQL or MySQL (see Dialect). Schema changes are managed by migrations only; GORM
// AutoMigrate must not be used.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"base-admin/server/internal/platform/config"
)

// Option adjusts how OpenSQL opens a pool.
type Option func(*openOptions)

type openOptions struct{ migrations bool }

// ForMigrations opens a pool for the migration runner: MySQL then accepts
// several statements in one Exec, as migration files contain. Application
// pools never do, so that injected SQL cannot append statements.
func ForMigrations() Option { return func(o *openOptions) { o.migrations = true } }

// OpenSQL opens a *sql.DB for the database URL in cfg (postgres:// through
// pgx, mysql:// through go-sql-driver), applies pool and session settings
// and verifies connectivity. The caller owns the returned pool.
func OpenSQL(ctx context.Context, cfg config.DatabaseConfig, appName string, opts ...Option) (*sql.DB, error) {
	var o openOptions
	for _, opt := range opts {
		opt(&o)
	}
	dialect, err := ParseDialect(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	var db *sql.DB
	switch dialect {
	case MySQL:
		mc, err := mysqlConfig(cfg, appName, o.migrations)
		if err != nil {
			return nil, err
		}
		connector, err := mysql.NewConnector(mc)
		if err != nil {
			return nil, fmt.Errorf("open mysql: %w", err)
		}
		db = sql.OpenDB(connector)
	default:
		if db, err = openPostgres(cfg, appName); err != nil {
			return nil, err
		}
	}
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
	if dialect == MySQL {
		if err := checkMySQLVersion(pingCtx, db); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return db, nil
}

func openPostgres(cfg config.DatabaseConfig, appName string) (*sql.DB, error) {
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
	return stdlib.OpenDB(*pgCfg), nil
}

// NewGorm wraps a pool opened by OpenSQL with GORM, using the dialector of
// its database. Query parameters are never logged
// so that sensitive values cannot reach the log.
func NewGorm(db *sql.DB, cfg config.DatabaseConfig, logger *zap.Logger) (*gorm.DB, error) {
	stdLog, err := zap.NewStdLogAt(logger.Named("gorm"), zap.WarnLevel)
	if err != nil {
		return nil, fmt.Errorf("create gorm logger: %w", err)
	}
	gdb, err := gorm.Open(dialectorFor(db), &gorm.Config{
		Logger: gormlogger.New(stdLog, gormlogger.Config{
			SlowThreshold:             cfg.SlowQueryThreshold,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
		}),
		NowFunc: func() time.Time { return time.Now().UTC() },
		// OpenSQL already pinged the main database; an extra database that
		// is not required may be down at startup (app/extra_db.go).
		DisableAutomaticPing: true,
		TranslateError:       true,
	})
	if err != nil {
		return nil, fmt.Errorf("open gorm: %w", err)
	}
	return gdb, nil
}

func dialectorFor(db *sql.DB) gorm.Dialector {
	if SQLDialect(db) == MySQL {
		return mysqlDialector{gormmysql.Dialector{Config: &gormmysql.Config{
			Conn: db,
			// GORM must not query the server version or options at open:
			// the pool is already configured.
			SkipInitializeWithVersion: true,
			DefaultStringSize:         255,
		}}}
	}
	return postgres.New(postgres.Config{Conn: db})
}
