//go:build integration

package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/testinfra"
)

// TestExtraDatabaseKeepsServerDefaults opens the test database a second
// time as an extra database: the session settings of the main database
// (sql_mode, time zone, isolation level; statement_timeout on PostgreSQL)
// are not applied, and GORM works on it.
func TestExtraDatabaseKeepsServerDefaults(t *testing.T) {
	main := testinfra.Database(t)
	extra := config.ExtraDatabaseConfig{URL: main.URL, MaxOpenConns: 2, MaxIdleConns: 1,
		ConnMaxLifetime: time.Minute, ConnMaxIdleTime: time.Minute, ConnectTimeout: 5 * time.Second}
	sqlDB, err := OpenExtra("game", extra, "extra-test")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, sqlDB.PingContext(context.Background()))
	db, err := NewGorm(sqlDB, config.DatabaseConfig{SlowQueryThreshold: time.Second}, zap.NewNop())
	require.NoError(t, err)

	if IsMySQL(db) {
		var mode, zone, isolation string
		require.NoError(t, db.Raw("SELECT @@SESSION.sql_mode, @@SESSION.time_zone, @@SESSION.transaction_isolation").
			Row().Scan(&mode, &zone, &isolation))
		assert.NotContains(t, mode, "ANSI_QUOTES")
		assert.NotContains(t, mode, "PIPES_AS_CONCAT")
		assert.NotEqual(t, "+00:00", zone)
		assert.Equal(t, "REPEATABLE-READ", isolation, "the server default")
		var quoted string
		require.NoError(t, db.Raw("SELECT \"double quotes are a string\"").Scan(&quoted).Error)
		assert.Equal(t, "double quotes are a string", quoted, "no ANSI_QUOTES")
		var now time.Time
		require.NoError(t, db.Raw("SELECT NOW()").Scan(&now).Error, "parseTime is on by default")
		assert.False(t, now.IsZero())
		return
	}
	var timeout string
	require.NoError(t, db.Raw("SHOW statement_timeout").Scan(&timeout).Error)
	assert.Equal(t, "0", timeout, "no statement timeout")
	var app string
	require.NoError(t, db.Raw("SHOW application_name").Scan(&app).Error)
	assert.Equal(t, "extra-test", app)
}

func TestOpenExtraRejectsBadURLs(t *testing.T) {
	for _, u := range []string{"redis://x", "mysql://u:p@db/", "mysql://u:p@db/a/b"} {
		_, err := OpenExtra("game", config.ExtraDatabaseConfig{URL: u, MaxOpenConns: 1}, "x")
		assert.Error(t, err, u)
	}
}
