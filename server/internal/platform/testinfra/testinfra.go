//go:build integration

// Package testinfra provides real PostgreSQL and Redis for integration
// tests. Tests built with the "integration" tag fail, rather than skip, when
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
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

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
// the test ends, and returns a config pointing at it.
func Database(t *testing.T) config.DatabaseConfig {
	t.Helper()
	adminURL := requireEnv(t, "ADMIN_TEST_DATABASE_URL")

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "test_" + hex.EncodeToString(b)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to test postgres: %v", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() { dropDatabase(t, adminURL, name) })

	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("ADMIN_TEST_DATABASE_URL must be a URL: %v", err)
	}
	u.Path = "/" + name
	return config.DatabaseConfig{
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

func dropDatabase(t *testing.T, adminURL, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Errorf("connect to drop test database: %v", err)
		return
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
		t.Errorf("drop test database: %v", err)
	}
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
