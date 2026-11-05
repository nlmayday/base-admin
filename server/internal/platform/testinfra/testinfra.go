//go:build integration

// Package testinfra provides a real database (PostgreSQL or MySQL) and
// Redis for integration tests. Tests built with the "integration" tag fail, rather than skip, when
// the infrastructure is not configured.
//
// Required environment:
//
//	ADMIN_TEST_DATABASE_URL  URL of a database the test user can CREATE DATABASE from
//	ADMIN_TEST_REDIS_ADDR    host:port of a Redis instance (see RedisDB for the databases it uses)
package testinfra

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"base-admin/server/internal/platform/config"
)

func requireEnv(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Fatalf("%s is not set; start dependencies with `make deps-up` and run `make test-integration`", key)
	}
	return value
}

// Database creates an empty, uniquely named database that is dropped when
// the test ends, and returns a config pointing at it. The dialect follows
// ADMIN_TEST_DATABASE_URL (postgres:// or mysql://).
func Database(t *testing.T) config.DatabaseConfig {
	t.Helper()
	return DatabaseAt(t, requireEnv(t, "ADMIN_TEST_DATABASE_URL"))
}

// DatabaseAt is Database on the server of adminURL, for tests that need a
// PostgreSQL and a MySQL database at once (schema parity).
func DatabaseAt(t *testing.T, adminURL string) config.DatabaseConfig {
	t.Helper()
	isMySQL := strings.HasPrefix(adminURL, "mysql://")

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "test_" + hex.EncodeToString(b)

	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal("the test database admin URL is malformed")
	}
	create, drop := postgresDDL(name)
	if isMySQL {
		create, drop = mysqlDDL(name)
	}
	adminExec(t, adminURL, create)
	t.Cleanup(func() { adminExec(t, adminURL, drop) })

	u.Path = "/" + name
	return config.DatabaseConfig{
		// The version tables follow the application settings when given,
		// so tests run with the names a project configures.
		MigrationsTable:    envOr("ADMIN_DATABASE_MIGRATIONS_TABLE", config.DefaultMigrationsTable),
		AppMigrationsTable: envOr("ADMIN_DATABASE_APP_MIGRATIONS_TABLE", config.DefaultAppMigrationsTable),
		URL:                u.String(),
		MaxOpenConns:       5,
		MaxIdleConns:       2,
		ConnMaxLifetime:    time.Minute,
		ConnMaxIdleTime:    time.Minute,
		ConnectTimeout:     5 * time.Second,
		StatementTimeout:   15 * time.Second,
		SlowQueryThreshold: time.Second,
	}
}

// IsMySQL reports whether integration tests run on MySQL.
func IsMySQL(t *testing.T) bool {
	t.Helper()
	return strings.HasPrefix(requireEnv(t, "ADMIN_TEST_DATABASE_URL"), "mysql://")
}

// SkipOnMySQL skips a test of a feature that supports PostgreSQL only.
func SkipOnMySQL(t *testing.T, why string) {
	t.Helper()
	if IsMySQL(t) {
		t.Skip("PostgreSQL only: " + why)
	}
}

// The test database name is generated, never input.
func postgresDDL(name string) (create, drop string) {
	id := pgx.Identifier{name}.Sanitize()
	return "CREATE DATABASE " + id, "DROP DATABASE IF EXISTS " + id + " WITH (FORCE)"
}

func mysqlDDL(name string) (create, drop string) {
	id := "`" + name + "`"
	return "CREATE DATABASE " + id + " CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_as_cs",
		"DROP DATABASE IF EXISTS " + id
}

// adminExec runs one statement on the database of the admin URL. Failures
// in cleanup are reported without stopping other cleanups. The package
// opens connections itself: package database's own tests use it.
func adminExec(t *testing.T, adminURL, stmt string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := openAdmin(adminURL)
	if err != nil {
		t.Errorf("connect to the test database server: %v", err)
		return
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		t.Errorf("%s: %v", stmt, err)
	}
}

func openAdmin(adminURL string) (*sql.DB, error) {
	if !strings.HasPrefix(adminURL, "mysql://") {
		cfg, err := pgx.ParseConfig(adminURL)
		if err != nil {
			return nil, err
		}
		return stdlib.OpenDB(*cfg), nil
	}
	u, err := url.Parse(adminURL)
	if err != nil {
		return nil, errors.New("malformed mysql url")
	}
	mc := mysql.NewConfig()
	mc.User = u.User.Username()
	mc.Passwd, _ = u.User.Password()
	mc.Net, mc.Addr, mc.DBName = "tcp", u.Host, strings.TrimPrefix(u.Path, "/")
	connector, err := mysql.NewConnector(mc)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(connector), nil
}

// Redis returns a config for the test Redis instance.
func Redis(t *testing.T) config.RedisConfig {
	t.Helper()
	return config.RedisConfig{
		Addr:                      requireEnv(t, "ADMIN_TEST_REDIS_ADDR"),
		PoolSize:                  5,
		DialTimeout:               3 * time.Second,
		ReadTimeout:               2 * time.Second,
		WriteTimeout:              2 * time.Second,
		RequireSafeEvictionPolicy: true,
	}
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
