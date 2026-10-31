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

	var timeout, appName string
	require.NoError(t, db.QueryRowContext(ctx, "SHOW statement_timeout").Scan(&timeout))
	require.NoError(t, db.QueryRowContext(ctx, "SHOW application_name").Scan(&appName))
	assert.Equal(t, "15s", timeout)
	assert.Equal(t, "base-admin-test", appName)
	assert.Equal(t, cfg.MaxOpenConns, db.Stats().MaxOpenConnections)
}

func TestOpenSQLWithoutStatementTimeout(t *testing.T) {
	ctx := context.Background()
	cfg := testinfra.Database(t)
	cfg.StatementTimeout = 0

	db, err := OpenSQL(ctx, cfg, "base-admin-test")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var timeout string
	require.NoError(t, db.QueryRowContext(ctx, "SHOW statement_timeout").Scan(&timeout))
	assert.Equal(t, "0", timeout)
}

func TestStatementTimeoutCancelsLongQueries(t *testing.T) {
	ctx := context.Background()
	cfg := testinfra.Database(t)
	cfg.StatementTimeout = 100 * time.Millisecond

	db, err := OpenSQL(ctx, cfg, "base-admin-test")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	_, err = db.ExecContext(ctx, "SELECT pg_sleep(1)")
	assert.ErrorContains(t, err, "statement timeout")
}

func TestOpenSQLErrorsDoNotLeakPassword(t *testing.T) {
	cfg := testinfra.Database(t)
	cfg.URL = "postgres://admin:supersecret@127.0.0.1:1/nowhere?sslmode=disable"
	cfg.ConnectTimeout = time.Second

	_, err := OpenSQL(context.Background(), cfg, "base-admin-test")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "supersecret")
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
