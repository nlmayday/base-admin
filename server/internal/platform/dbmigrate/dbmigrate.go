// Package dbmigrate applies the embedded SQL migrations with golang-migrate.
// A PostgreSQL advisory lock prevents concurrent runs.
package dbmigrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

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

// Runner applies migrations. It takes ownership of the *sql.DB passed to
// New and closes it in Close.
type Runner struct {
	m   *migrate.Migrate
	src source.Driver
}

// Status is the current schema version.
type Status struct {
	Version uint
	Dirty   bool
	// Applied is false when no migration has run yet.
	Applied bool
}

// New creates a Runner using the embedded migrations.
func New(db *sql.DB) (*Runner, error) {
	return NewWithSource(db, migrations.FS)
}

// NewWithSource creates a Runner from an arbitrary migration file system.
func NewWithSource(db *sql.DB, files fs.FS) (*Runner, error) {
	src, err := iofs.New(files, ".")
	if err != nil {
		return nil, fmt.Errorf("open migration source: %w", err)
	}
	driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{})
	if err != nil {
		return nil, fmt.Errorf("open migration driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	if err != nil {
		return nil, fmt.Errorf("create migrator: %w", err)
	}
	return &Runner{m: m, src: src}, nil
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

// Latest returns the highest version in the embedded migrations.
func Latest() (uint, error) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return 0, fmt.Errorf("read migrations: %w", err)
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

// RequireCurrent fails unless the schema is clean and at the latest version,
// so the server never runs against an unmigrated database.
func RequireCurrent(ctx context.Context, db *sql.DB) error {
	latest, err := Latest()
	if err != nil {
		return err
	}
	var version uint64
	var dirty bool
	err = db.QueryRowContext(ctx, "SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty)
	if err != nil {
		return fmt.Errorf("schema not migrated (run `admin migrate up`): %w", err)
	}
	if dirty || version != uint64(latest) {
		return fmt.Errorf("schema is at version %d (dirty=%t) but the binary needs %d; run `admin migrate up`", version, dirty, latest)
	}
	return nil
}
