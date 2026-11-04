package database

import (
	"database/sql"
	"fmt"
	"net/url"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"base-admin/server/internal/platform/config"
)

// OpenExtra opens an extra database of the business modules, such as a game
// database (config.ExtraDatabaseConfig). It does not connect: callers ping
// it and decide whether an unreachable database stops the start.
//
// Unlike OpenSQL it applies no session settings, so the connection keeps
// the server's defaults (sql_mode, time zone, isolation level) and the
// shared-SQL helpers of this package (InsertID, OnConflictUpdate, ...) are
// not meant for it: write the SQL of that database. A MySQL URL gets
// parseTime=true unless it sets parseTime; its other parameters (loc,
// charset, tls, ...) go to the driver as they are.
func OpenExtra(name string, cfg config.ExtraDatabaseConfig, appName string) (*sql.DB, error) {
	dialect, err := ParseDialect(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("database.extra.%s: %w", name, err)
	}
	var db *sql.DB
	switch dialect {
	case MySQL:
		u, err := url.Parse(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("database.extra.%s: malformed mysql url", name)
		}
		mc, err := parseMySQLURL(u)
		if err != nil {
			return nil, fmt.Errorf("database.extra.%s: %w", name, err)
		}
		if !u.Query().Has("parseTime") {
			mc.ParseTime = true
		}
		mc.Timeout = cfg.ConnectTimeout
		mc.ConnectionAttributes = "program_name:" + appName
		connector, err := mysql.NewConnector(mc)
		if err != nil {
			return nil, fmt.Errorf("database.extra.%s: %w", name, err)
		}
		db = sql.OpenDB(connector)
	default:
		pgCfg, err := pgx.ParseConfig(cfg.URL)
		if err != nil {
			// pgx redacts the password in parse errors.
			return nil, fmt.Errorf("database.extra.%s: %w", name, err)
		}
		pgCfg.ConnectTimeout = cfg.ConnectTimeout
		if _, set := pgCfg.RuntimeParams["application_name"]; !set {
			pgCfg.RuntimeParams["application_name"] = appName
		}
		db = stdlib.OpenDB(*pgCfg)
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	return db, nil
}
