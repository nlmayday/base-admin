//go:build integration

package dbmigrate

import (
	"context"
	"database/sql"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/testinfra"
)

func newRunner(t *testing.T) (*Runner, *sql.DB) {
	t.Helper()
	cfg := testinfra.Database(t)
	ctx := context.Background()

	db, err := database.OpenSQL(ctx, cfg, "migrate-test")
	require.NoError(t, err)
	runner, err := New(db)
	require.NoError(t, err)

	// A separate pool for assertions; the runner owns and closes its own.
	probe, err := database.OpenSQL(ctx, cfg, "migrate-probe")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = probe.Close()
		_ = runner.Close()
	})
	return runner, probe
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRow("SELECT to_regclass($1) IS NOT NULL", name).Scan(&exists))
	return exists
}

func TestUpIsRepeatableAndTracked(t *testing.T) {
	runner, probe := newRunner(t)

	status, err := runner.Status()
	require.NoError(t, err)
	assert.Equal(t, Status{}, status, "fresh database has no version")

	require.NoError(t, runner.Up())
	require.NoError(t, runner.Up(), "second run must be a no-op")

	latest, err := Latest()
	require.NoError(t, err)
	status, err = runner.Status()
	require.NoError(t, err)
	assert.Equal(t, Status{Version: latest, Dirty: false, Applied: true}, status)
	require.NoError(t, RequireCurrent(context.Background(), probe))

	var version int64
	require.NoError(t, probe.QueryRow("SELECT version FROM sys_auth_version WHERE id = 1").Scan(&version))
	assert.Equal(t, int64(1), version)
}

func TestDownAndUpAgain(t *testing.T) {
	runner, probe := newRunner(t)
	require.NoError(t, runner.Up())

	latest, err := Latest()
	require.NoError(t, err)
	require.NoError(t, runner.Down(1))
	assert.Error(t, RequireCurrent(context.Background(), probe), "one step behind must be refused")
	require.NoError(t, runner.Down(int(latest)-1))
	assert.False(t, tableExists(t, probe, "sys_auth_version"))
	status, err := runner.Status()
	require.NoError(t, err)
	assert.False(t, status.Applied)
	assert.Error(t, RequireCurrent(context.Background(), probe))

	require.NoError(t, runner.Up())
	assert.True(t, tableExists(t, probe, "sys_auth_version"))
}

func TestDownRequiresPositiveSteps(t *testing.T) {
	runner, _ := newRunner(t)
	assert.Error(t, runner.Down(0))
	assert.Error(t, runner.Down(-1))
}

func TestAuthVersionConstraints(t *testing.T) {
	runner, probe := newRunner(t)
	require.NoError(t, runner.Up())

	_, err := probe.Exec("INSERT INTO sys_auth_version (id, version) VALUES (2, 1)")
	assert.Error(t, err, "only one row is allowed")
	_, err = probe.Exec("UPDATE sys_auth_version SET version = 0")
	assert.Error(t, err, "version must stay positive")
}

func TestFailedMigrationLeavesDirtyStateThatForceRecovers(t *testing.T) {
	cfg := testinfra.Database(t)
	db, err := database.OpenSQL(context.Background(), cfg, "migrate-test")
	require.NoError(t, err)

	broken := fstest.MapFS{
		"000001_ok.up.sql":    {Data: []byte("CREATE TABLE t1 (id int);")},
		"000001_ok.down.sql":  {Data: []byte("DROP TABLE t1;")},
		"000002_bad.up.sql":   {Data: []byte("CREATE TABLE t2 (id int); SELECT broken syntax here;")},
		"000002_bad.down.sql": {Data: []byte("DROP TABLE IF EXISTS t2;")},
	}
	runner, err := NewWithSource(db, broken)
	require.NoError(t, err)
	defer func() { _ = runner.Close() }()

	require.Error(t, runner.Up())
	status, err := runner.Status()
	require.NoError(t, err)
	assert.Equal(t, Status{Version: 2, Dirty: true, Applied: true}, status)

	assert.Error(t, runner.Force(0), "version 0 does not exist")
	assert.Error(t, runner.Force(9), "unknown versions are refused")
	assert.Error(t, runner.Force(-2))

	require.NoError(t, runner.Force(1))
	status, err = runner.Status()
	require.NoError(t, err)
	assert.Equal(t, Status{Version: 1, Dirty: false, Applied: true}, status)

	require.NoError(t, runner.Force(NilVersion))
	status, err = runner.Status()
	require.NoError(t, err)
	assert.Equal(t, Status{}, status)
}

func TestStopOnDoneStopsBeforeNextMigration(t *testing.T) {
	cfg := testinfra.Database(t)
	db, err := database.OpenSQL(context.Background(), cfg, "migrate-test")
	require.NoError(t, err)

	files := fstest.MapFS{
		"000001_a.up.sql":   {Data: []byte("CREATE TABLE a (id int);")},
		"000001_a.down.sql": {Data: []byte("DROP TABLE a;")},
		"000002_b.up.sql":   {Data: []byte("CREATE TABLE b (id int);")},
		"000002_b.down.sql": {Data: []byte("DROP TABLE b;")},
	}
	runner, err := NewWithSource(db, files)
	require.NoError(t, err)
	defer func() { _ = runner.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	release := runner.StopOnDone(ctx)
	require.Eventually(t, func() bool { return len(runner.m.GracefulStop) == 1 }, time.Second, 5*time.Millisecond)
	err = runner.Up()
	release()

	require.NoError(t, err)
	status, err := runner.Status()
	require.NoError(t, err)
	assert.False(t, status.Dirty, "a graceful stop never leaves the schema dirty")
	assert.Less(t, status.Version, uint(2), "the run must stop before applying everything")
}
