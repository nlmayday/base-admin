//go:build integration

package dbmigrate

import (
	"context"
	"database/sql"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/testinfra"
)

func newRunner(t *testing.T) (*Runner, *sql.DB) {
	t.Helper()
	cfg := testinfra.Database(t)
	ctx := context.Background()

	db, err := database.OpenSQL(ctx, cfg, "migrate-test", database.ForMigrations())
	require.NoError(t, err)
	runner, err := New(db, BaseTrack)
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

func testTrack(files fstest.MapFS) Track {
	return Track{Name: "test", Table: "schema_migrations", Files: bothDialects(files)}
}

// bothDialects uses the same files, plain SQL both accept, for each dialect.
func bothDialects(files fs.FS) map[database.Dialect]fs.FS {
	return map[database.Dialect]fs.FS{database.Postgres: files, database.MySQL: files}
}

func hasTable(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	exists, err := tableExists(context.Background(), db, name)
	require.NoError(t, err)
	return exists
}

func TestUpIsRepeatableAndTracked(t *testing.T) {
	runner, probe := newRunner(t)

	status, err := runner.Status()
	require.NoError(t, err)
	assert.Equal(t, Status{}, status, "fresh database has no version")

	require.NoError(t, runner.Up())
	require.NoError(t, runner.Up(), "second run must be a no-op")

	latest, err := BaseTrack.Latest(database.SQLDialect(probe))
	require.NoError(t, err)
	status, err = runner.Status()
	require.NoError(t, err)
	assert.Equal(t, Status{Version: latest, Dirty: false, Applied: true}, status)
	require.NoError(t, BaseTrack.RequireCurrent(context.Background(), probe))

	var version int64
	require.NoError(t, probe.QueryRow("SELECT version FROM sys_auth_version WHERE id = 1").Scan(&version))
	assert.Equal(t, int64(1), version)
}

func TestDownAndUpAgain(t *testing.T) {
	runner, probe := newRunner(t)
	require.NoError(t, runner.Up())

	latest, err := BaseTrack.Latest(database.SQLDialect(probe))
	require.NoError(t, err)
	require.NoError(t, runner.Down(1))
	assert.Error(t, BaseTrack.RequireCurrent(context.Background(), probe), "one step behind must be refused")
	require.NoError(t, runner.Down(int(latest)-1))
	assert.False(t, hasTable(t, probe, "sys_auth_version"))
	status, err := runner.Status()
	require.NoError(t, err)
	assert.False(t, status.Applied)
	assert.Error(t, BaseTrack.RequireCurrent(context.Background(), probe))

	require.NoError(t, runner.Up())
	assert.True(t, hasTable(t, probe, "sys_auth_version"))
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
	db, err := database.OpenSQL(context.Background(), cfg, "migrate-test", database.ForMigrations())
	require.NoError(t, err)

	broken := fstest.MapFS{
		"000001_ok.up.sql":    {Data: []byte("CREATE TABLE t1 (id int);")},
		"000001_ok.down.sql":  {Data: []byte("DROP TABLE t1;")},
		"000002_bad.up.sql":   {Data: []byte("CREATE TABLE t2 (id int); SELECT broken syntax here;")},
		"000002_bad.down.sql": {Data: []byte("DROP TABLE IF EXISTS t2;")},
	}
	runner, err := New(db, testTrack(broken))
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
	db, err := database.OpenSQL(context.Background(), cfg, "migrate-test", database.ForMigrations())
	require.NoError(t, err)

	files := fstest.MapFS{
		"000001_a.up.sql":   {Data: []byte("CREATE TABLE a (id int);")},
		"000001_a.down.sql": {Data: []byte("DROP TABLE a;")},
		"000002_b.up.sql":   {Data: []byte("CREATE TABLE b (id int);")},
		"000002_b.down.sql": {Data: []byte("DROP TABLE b;")},
	}
	runner, err := New(db, testTrack(files))
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

// Tracks number their migrations independently: the same version on both
// runs both, and a scaffold migration added after the application track
// moved on still runs.
func TestTracksAreIndependent(t *testing.T) {
	cfg := testinfra.Database(t)
	ctx := context.Background()
	probe, err := database.OpenSQL(ctx, cfg, "migrate-probe")
	require.NoError(t, err)
	defer func() { _ = probe.Close() }()

	base := Track{Name: "base", Table: "schema_migrations", Files: bothDialects(fstest.MapFS{
		"000001_base.up.sql":   {Data: []byte("CREATE TABLE base_a (id int PRIMARY KEY);")},
		"000001_base.down.sql": {Data: []byte("DROP TABLE base_a;")},
	})}
	app := Track{Name: "app", Table: "app_schema_migrations", Files: bothDialects(fstest.MapFS{
		"000001_app.up.sql":   {Data: []byte("CREATE TABLE app_a (id int REFERENCES base_a (id));")},
		"000001_app.down.sql": {Data: []byte("DROP TABLE app_a;")},
		"000002_app.up.sql":   {Data: []byte("CREATE TABLE app_b (id int);")},
		"000002_app.down.sql": {Data: []byte("DROP TABLE app_b;")},
	})}
	up := func(tr Track) Status {
		t.Helper()
		db, err := database.OpenSQL(ctx, cfg, "migrate-test", database.ForMigrations())
		require.NoError(t, err)
		r, err := New(db, tr)
		require.NoError(t, err)
		defer func() { _ = r.Close() }()
		require.NoError(t, r.Up())
		st, err := r.Status()
		require.NoError(t, err)
		return st
	}

	assert.Error(t, app.RequireCurrent(ctx, probe), "app track not applied yet")
	assert.Equal(t, uint(1), up(base).Version)
	assert.Equal(t, uint(2), up(app).Version)
	require.NoError(t, base.RequireCurrent(ctx, probe))
	require.NoError(t, app.RequireCurrent(ctx, probe))

	// Upstream adds scaffold migration 2 after the app track reached 2.
	base.Files[database.Postgres].(fstest.MapFS)["000002_base.up.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE base_b (id int);")}
	base.Files[database.Postgres].(fstest.MapFS)["000002_base.down.sql"] = &fstest.MapFile{Data: []byte("DROP TABLE base_b;")}
	assert.Error(t, base.RequireCurrent(ctx, probe), "a new scaffold migration is pending")
	assert.Equal(t, uint(2), up(base).Version)
	assert.True(t, hasTable(t, probe, "base_b"))
	require.NoError(t, base.RequireCurrent(ctx, probe))
	require.NoError(t, app.RequireCurrent(ctx, probe))
}

// An application track without migrations is a no-op and counts as current.
func TestEmptyTrack(t *testing.T) {
	cfg := testinfra.Database(t)
	ctx := context.Background()
	db, err := database.OpenSQL(ctx, cfg, "migrate-test", database.ForMigrations())
	require.NoError(t, err)
	empty := Track{Name: "app", Table: "app_schema_migrations", Files: bothDialects(fstest.MapFS{"README.md": {Data: []byte("x")}})}
	r, err := New(db, empty)
	require.NoError(t, err)
	defer func() { _ = r.Close() }()
	require.NoError(t, r.Up())
	assert.ErrorContains(t, r.Down(1), "no migrations")
	st, err := r.Status()
	require.NoError(t, err)
	assert.Equal(t, Status{}, st)
	assert.NoError(t, empty.RequireCurrent(ctx, db))
}

// TestConfiguredVersionTables runs both tracks with configured version
// tables in a database where another service (golang-migrate as well)
// already keeps its versions in schema_migrations: neither touches the
// other's table.
func TestConfiguredVersionTables(t *testing.T) {
	cfg := testinfra.Database(t)
	cfg.MigrationsTable, cfg.AppMigrationsTable = "admin_schema_migrations", "admin_app_schema_migrations"
	ctx := context.Background()
	probe, err := database.OpenSQL(ctx, cfg, "migrate-probe")
	require.NoError(t, err)
	t.Cleanup(func() { _ = probe.Close() })
	_, err = probe.ExecContext(ctx, "CREATE TABLE schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL)")
	require.NoError(t, err)
	_, err = probe.ExecContext(ctx, "INSERT INTO schema_migrations (version, dirty) VALUES (42, false)")
	require.NoError(t, err)

	tracks := TracksFor(cfg)
	require.Equal(t, "admin_schema_migrations", tracks[0].Table)
	require.Equal(t, "admin_app_schema_migrations", tracks[1].Table)
	for _, track := range tracks {
		db, err := database.OpenSQL(ctx, cfg, "migrate-test", database.ForMigrations())
		require.NoError(t, err)
		runner, err := New(db, track)
		require.NoError(t, err)
		require.NoError(t, runner.Up(), track.Name)
		require.NoError(t, runner.Close())
	}
	require.NoError(t, RequireCurrent(ctx, probe, tracks))
	assert.Error(t, RequireCurrent(ctx, probe, TracksFor(config.DatabaseConfig{})),
		"the default tables belong to the other service here")

	var version int64
	var dirty bool
	require.NoError(t, probe.QueryRowContext(ctx, "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty))
	assert.Equal(t, int64(42), version, "the other service's version is untouched")
	latest, err := BaseTrack.Latest(database.SQLDialect(probe))
	require.NoError(t, err)
	got, _, err := tracks[0].State(ctx, probe)
	require.NoError(t, err)
	assert.Equal(t, uint64(latest), got)
}
