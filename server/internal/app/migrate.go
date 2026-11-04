package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"go.uber.org/zap"

	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/dbmigrate"
)

// MigrateUsage documents the migrate subcommands.
const MigrateUsage = `migrate [track] up             apply pending migrations (all tracks, base first)
migrate [track] version        print the schema versions (exit 1 if dirty)
migrate <track> down <n>       roll back n migrations of a track
migrate <track> force <v>      mark version v as applied without running it (dirty-state recovery);
                                 v must be an existing version, or -1 for "nothing applied"
tracks: base (scaffold, server/migrations), app (application, server/migrations/app)`

// ErrUsage reports invalid command-line arguments.
var ErrUsage = errors.New("invalid arguments")

// ErrDirtySchema reports that a migration failed halfway.
var ErrDirtySchema = errors.New("schema is dirty")

type migrateCmd struct {
	tracks []dbmigrate.Track
	action string
	n      int
}

// Migrate runs a migration subcommand on each selected track and prints the
// resulting schema status to out. Migrations run without the request
// statement timeout because schema changes can legitimately take longer. A
// dirty schema is reported as an error so scripts can detect it from the
// exit code.
func Migrate(ctx context.Context, cfg config.Config, logger *zap.Logger, out io.Writer, args []string) error {
	cmd, err := parseMigrateArgs(dbmigrate.TracksFor(cfg.Database), args)
	if err != nil {
		return err
	}
	dbCfg := cfg.Database
	dbCfg.StatementTimeout = 0
	var dirty []error
	for _, track := range cmd.tracks {
		status, err := migrateTrack(ctx, dbCfg, cfg.App.Name, track, cmd)
		if err != nil {
			return err
		}
		if err := reportStatus(track, status, cmd.action, logger, out); err != nil {
			dirty = append(dirty, err)
		}
	}
	return errors.Join(dirty...)
}

// migrateTrack runs the action on one track with its own connection pool,
// which the runner closes.
func migrateTrack(ctx context.Context, dbCfg config.DatabaseConfig, appName string, track dbmigrate.Track, cmd migrateCmd) (status dbmigrate.Status, err error) {
	sqlDB, err := database.OpenSQL(ctx, dbCfg, appName+"-migrate", database.ForMigrations())
	if err != nil {
		return status, err
	}
	runner, err := dbmigrate.New(sqlDB, track)
	if err != nil {
		_ = sqlDB.Close()
		return status, err
	}
	defer func() { err = errors.Join(err, runner.Close()) }()

	release := runner.StopOnDone(ctx)
	actionErr := runMigrateAction(runner, cmd.action, cmd.n)
	release()
	if actionErr != nil {
		return status, fmt.Errorf("%s track: %w", track.Name, actionErr)
	}
	return runner.Status()
}

func reportStatus(track dbmigrate.Track, status dbmigrate.Status, action string, logger *zap.Logger, out io.Writer) error {
	logger.Info("schema status",
		zap.String("track", track.Name),
		zap.String("action", action),
		zap.Uint("version", status.Version),
		zap.Bool("dirty", status.Dirty),
		zap.Bool("applied", status.Applied),
	)
	if _, err := fmt.Fprintf(out, "%s version=%d dirty=%t applied=%t\n", track.Name, status.Version, status.Dirty, status.Applied); err != nil {
		return fmt.Errorf("write status: %w", err)
	}
	if status.Dirty {
		return fmt.Errorf("%w: %s version %d; repair the schema, then run `migrate %s force <version>`",
			ErrDirtySchema, track.Name, status.Version, track.Name)
	}
	return nil
}

// parseMigrateArgs reads `[track] action [n]`. up and version default to
// every track; down and force change one track and must name it.
func parseMigrateArgs(tracks []dbmigrate.Track, args []string) (migrateCmd, error) {
	cmd := migrateCmd{tracks: tracks}
	if len(args) > 0 {
		if t, ok := dbmigrate.TrackByName(tracks, args[0]); ok {
			cmd.tracks, args = []dbmigrate.Track{t}, args[1:]
		}
	}
	if len(args) == 0 {
		return cmd, fmt.Errorf("%w: missing migrate action", ErrUsage)
	}
	cmd.action = args[0]
	switch cmd.action {
	case "up", "version":
		if len(args) != 1 {
			return cmd, fmt.Errorf("%w: migrate %s takes no arguments", ErrUsage, cmd.action)
		}
		return cmd, nil
	case "down", "force":
		if len(cmd.tracks) != 1 {
			return cmd, fmt.Errorf("%w: migrate %s changes one track; name it, e.g. `migrate app %s ...`", ErrUsage, cmd.action, cmd.action)
		}
		if len(args) != 2 {
			return cmd, fmt.Errorf("%w: migrate %s requires one number", ErrUsage, cmd.action)
		}
		n, err := strconv.Atoi(args[1])
		if err != nil || !validMigrateNumber(cmd.action, n) {
			return cmd, fmt.Errorf("%w: invalid number %q for migrate %s", ErrUsage, args[1], cmd.action)
		}
		cmd.n = n
		return cmd, nil
	default:
		return cmd, fmt.Errorf("%w: unknown migrate action %q", ErrUsage, cmd.action)
	}
}

func validMigrateNumber(action string, n int) bool {
	if action == "down" {
		return n > 0
	}
	return n >= dbmigrate.NilVersion
}

func runMigrateAction(runner *dbmigrate.Runner, action string, n int) error {
	switch action {
	case "up":
		return runner.Up()
	case "down":
		return runner.Down(n)
	case "force":
		return runner.Force(n)
	default: // "version" only reports status
		return nil
	}
}
