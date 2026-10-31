//go:build integration

// Package migrated provides fully migrated test databases. It is separate
// from testinfra because the database and dbmigrate tests use testinfra.
package migrated

import (
	"context"
	"testing"

	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/dbmigrate"
	"base-admin/server/internal/platform/testinfra"
)

// Database creates a fresh database with all migrations applied.
func Database(t *testing.T) config.DatabaseConfig {
	t.Helper()
	cfg := testinfra.Database(t)
	db, err := database.OpenSQL(context.Background(), cfg, "testinfra-migrate")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	runner, err := dbmigrate.New(db)
	if err != nil {
		t.Fatalf("create migrator: %v", err)
	}
	defer func() { _ = runner.Close() }()
	if err := runner.Up(); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return cfg
}
