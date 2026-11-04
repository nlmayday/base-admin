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

// Database creates a fresh database with every migration track applied.
func Database(t *testing.T) config.DatabaseConfig {
	t.Helper()
	cfg := testinfra.Database(t)
	for _, track := range dbmigrate.Tracks() {
		db, err := database.OpenSQL(context.Background(), cfg, "testinfra-migrate", database.ForMigrations())
		if err != nil {
			t.Fatalf("open test database: %v", err)
		}
		runner, err := dbmigrate.New(db, track)
		if err != nil {
			t.Fatalf("create %s migrator: %v", track.Name, err)
		}
		err = runner.Up()
		_ = runner.Close()
		if err != nil {
			t.Fatalf("migrate test database: %v", err)
		}
	}
	return cfg
}
