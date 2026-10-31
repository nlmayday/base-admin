// Package dbmigrate applies the embedded SQL migrations with golang-migrate.
// Migrations come in tracks, each with its own numbering and version table:
// the scaffold's and the application's. A PostgreSQL advisory lock per
// version table prevents concurrent runs.
package dbmigrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"base-admin/server/migrations"
)

// NilVersion is the Force target that marks the schema as having no
// migrations applied.
const NilVersion = database.NilVersion

// Track is one sequence of migrations with its own version table.
type Track struct {
	Name  string
	Table string
	Files fs.FS
}

// The scaffold track keeps the original version table, so existing
// databases need no conversion; the application track starts empty.
var (
	BaseTrack = Track{Name: "base", Table: "schema_migrations", Files: migrations.FS}
	AppTrack  = Track{Name: "app", Table: "app_schema_migrations", Files: migrations.App}
)

// Tracks returns the tracks in the order they are applied: application
// migrations may reference scaffold tables, never the reverse.
func Tracks() []Track { return []Track{BaseTrack, AppTrack} }

// TrackByName finds a track of Tracks.
func TrackByName(name string) (Track, bool) {
	for _, t := range Tracks() {
		if t.Name == name {
			return t, true
		}
	}
	return Track{}, false
}

// Runner applies the migrations of one track. It takes ownership of the
// *sql.DB passed to New and closes it in Close.
type Runner struct {
	m   *migrate.Migrate
	src source.Driver
	// empty is set when the track has no migrations: golang-migrate
	// cannot run up from an empty source.
	empty bool
	track string
}

// Status is the current schema version.
type Status struct {
	Version uint
	Dirty   bool
	// Applied is false when no migration has run yet.
	Applied bool
}

// New creates a Runner for a track.
func New(db *sql.DB, t Track) (*Runner, error) {
	src, err := iofs.New(t.Files, ".")
	if err != nil {
		return nil, fmt.Errorf("open %s migrations: %w", t.Name, err)
	}
	_, err = src.First()
	empty := errors.Is(err, os.ErrNotExist)
	if err != nil && !empty {
		return nil, fmt.Errorf("read %s migrations: %w", t.Name, err)
	}
	driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{MigrationsTable: t.Table})
	if err != nil {
		return nil, fmt.Errorf("open migration driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	if err != nil {
		return nil, fmt.Errorf("create migrator: %w", err)
	}
	return &Runner{m: m, src: src, empty: empty, track: t.Name}, nil
}

// StopOnDone asks a running Up/Down to stop after the current migration once
// ctx is done, so a stop signal never interrupts a migration halfway. Call
// the returned function when the run finishes.
func (r *Runner) StopOnDone(ctx context.Context) (release func()) {
	finished := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			r.m.GracefulStop <- true
		case <-finished:
		}
	}()
	return func() { close(finished) }
}

// Up applies all pending migrations. Running it again is a no-op.
func (r *Runner) Up() error {
	if r.empty {
		return nil
	}
	if err := r.m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// Down rolls back exactly steps migrations. Rolling back everything requires
// an explicit count to avoid accidental data loss.
func (r *Runner) Down(steps int) error {
	if steps <= 0 {
		return fmt.Errorf("migrate down: steps must be positive, got %d", steps)
	}
	if r.empty {
		return fmt.Errorf("migrate down: the %s track has no migrations", r.track)
	}
	if err := r.m.Steps(-steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate down %d: %w", steps, err)
	}
	return nil
}

// Force sets the version without running migrations, to recover from a
// dirty state after manual repair. version must exist in the source, or be
// NilVersion to mark nothing as applied; any other value would leave the
// schema in a state that later migrations cannot start from.
func (r *Runner) Force(version int) error {
	if err := r.checkForceTarget(version); err != nil {
		return err
	}
	if err := r.m.Force(version); err != nil {
		return fmt.Errorf("migrate force %d: %w", version, err)
	}
	return nil
}

func (r *Runner) checkForceTarget(version int) error {
	if version == NilVersion {
		return nil
	}
	if version < 0 {
		return fmt.Errorf("migrate force: version must be %d or an existing version, got %d", NilVersion, version)
	}
	body, _, err := r.src.ReadUp(uint(version))
	if err != nil {
		return fmt.Errorf("migrate force: version %d does not exist: %w", version, err)
	}
	return body.Close()
}

// Status reports the current schema version.
func (r *Runner) Status() (Status, error) {
	version, dirty, err := r.m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, fmt.Errorf("read migration version: %w", err)
	}
	return Status{Version: version, Dirty: dirty, Applied: true}, nil
}

// Close releases the source and the database pool.
func (r *Runner) Close() error {
	srcErr, dbErr := r.m.Close()
	return errors.Join(srcErr, dbErr)
}

// Latest returns the highest version of the track; 0 when it has none.
func (t Track) Latest() (uint, error) {
	entries, err := fs.ReadDir(t.Files, ".")
	if err != nil {
		return 0, fmt.Errorf("read %s migrations: %w", t.Name, err)
	}
	var latest uint
	for _, e := range entries {
		var v uint
		if _, err := fmt.Sscanf(e.Name(), "%d_", &v); err == nil && v > latest {
			latest = v
		}
	}
	return latest, nil
}

// RequireCurrent fails unless every track is clean and at its latest
// version, so the server never runs against an unmigrated database.
func RequireCurrent(ctx context.Context, db *sql.DB) error {
	for _, t := range Tracks() {
		if err := t.RequireCurrent(ctx, db); err != nil {
			return err
		}
	}
	return nil
}

// RequireCurrent fails unless the track is clean and at its latest version.
func (t Track) RequireCurrent(ctx context.Context, db *sql.DB) error {
	latest, err := t.Latest()
	if err != nil {
		return err
	}
	version, dirty, err := t.State(ctx, db)
	if err != nil {
		return err
	}
	if dirty || version != uint64(latest) {
		return fmt.Errorf("%s schema is at version %d (dirty=%t) but the binary needs %d; run `admin migrate up`",
			t.Name, version, dirty, latest)
	}
	return nil
}

// State reads the version table; a missing table or row is version 0.
func (t Track) State(ctx context.Context, db *sql.DB) (version uint64, dirty bool, err error) {
	var exists bool
	if err := db.QueryRowContext(ctx, "SELECT to_regclass($1) IS NOT NULL", t.Table).Scan(&exists); err != nil {
		return 0, false, fmt.Errorf("read %s schema version: %w", t.Name, err)
	}
	if !exists {
		return 0, false, nil
	}
	// The table name is a constant of the track, not input.
	err = db.QueryRowContext(ctx, "SELECT version, dirty FROM "+t.Table+" LIMIT 1").Scan(&version, &dirty) //nolint:gosec // G202: constant table name
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read %s schema version: %w", t.Name, err)
	}
	return version, dirty, nil
}
