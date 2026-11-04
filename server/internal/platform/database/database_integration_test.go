//go:build integration

package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"base-admin/server/internal/platform/testinfra"
)

func TestOpenSQLAppliesSessionSettings(t *testing.T) {
	ctx := context.Background()
	cfg := testinfra.Database(t)

	db, err := OpenSQL(ctx, cfg, "base-admin-test")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	assert.Equal(t, cfg.MaxOpenConns, db.Stats().MaxOpenConnections)

	if testinfra.IsMySQL(t) {
		var timeout int64
		var zone, isolation, mode string
		require.NoError(t, db.QueryRowContext(ctx,
			"SELECT @@session.max_execution_time, @@session.time_zone, @@session.transaction_isolation, @@session.sql_mode").
			Scan(&timeout, &zone, &isolation, &mode))
		assert.Equal(t, int64(15000), timeout)
		assert.Equal(t, "+00:00", zone)
		assert.Equal(t, "READ-COMMITTED", isolation)
		for _, m := range []string{"STRICT_ALL_TABLES", "ANSI_QUOTES", "PIPES_AS_CONCAT"} {
			assert.Contains(t, mode, m)
		}
		return
	}
	var timeout, appName string
	require.NoError(t, db.QueryRowContext(ctx, "SHOW statement_timeout").Scan(&timeout))
	require.NoError(t, db.QueryRowContext(ctx, "SHOW application_name").Scan(&appName))
	assert.Equal(t, "15s", timeout)
	assert.Equal(t, "base-admin-test", appName)
}

func TestOpenSQLWithoutStatementTimeout(t *testing.T) {
	ctx := context.Background()
	cfg := testinfra.Database(t)
	cfg.StatementTimeout = 0

	db, err := OpenSQL(ctx, cfg, "base-admin-test")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	query, want := "SHOW statement_timeout", "0"
	if testinfra.IsMySQL(t) {
		query = "SELECT @@session.max_execution_time"
	}
	var timeout string
	require.NoError(t, db.QueryRowContext(ctx, query).Scan(&timeout))
	assert.Equal(t, want, timeout)
}

func TestStatementTimeoutCancelsLongQueries(t *testing.T) {
	ctx := context.Background()
	cfg := testinfra.Database(t)
	cfg.StatementTimeout = 100 * time.Millisecond

	db, err := OpenSQL(ctx, cfg, "base-admin-test")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	if testinfra.IsMySQL(t) {
		// max_execution_time interrupts a SELECT that reads rows; a bare
		// SLEEP() would just return 1.
		_, err = db.ExecContext(ctx, `SELECT SLEEP(1) FROM information_schema.tables LIMIT 2`)
		assert.ErrorContains(t, err, "maximum statement execution time exceeded")
		return
	}
	_, err = db.ExecContext(ctx, "SELECT pg_sleep(1)")
	assert.ErrorContains(t, err, "statement timeout")
}

func TestOpenSQLErrorsDoNotLeakPassword(t *testing.T) {
	cfg := testinfra.Database(t)
	cfg.ConnectTimeout = time.Second
	for _, u := range []string{
		"postgres://admin:supersecret@127.0.0.1:1/nowhere?sslmode=disable",
		"mysql://admin:supersecret@127.0.0.1:1/nowhere",
		"mysql://admin:supersecret@127.0.0.1:1/nowhere?bogus%zz",
	} {
		cfg.URL = u
		_, err := OpenSQL(context.Background(), cfg, "base-admin-test")
		require.Error(t, err, u)
		assert.NotContains(t, err.Error(), "supersecret", u)
	}
}

func TestNewGorm(t *testing.T) {
	ctx := context.Background()
	cfg := testinfra.Database(t)
	db, err := OpenSQL(ctx, cfg, "base-admin-test")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	gdb, err := NewGorm(db, cfg, zap.NewNop())
	require.NoError(t, err)
	var one int
	require.NoError(t, gdb.WithContext(ctx).Raw("SELECT 1").Scan(&one).Error)
	assert.Equal(t, 1, one)
}
