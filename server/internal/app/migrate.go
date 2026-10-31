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
const MigrateUsage = `migrate up             apply all pending migrations
migrate down <n>       roll back n migrations
migrate version        print the current schema version (exit 1 if dirty)
migrate force <v>      mark version v as applied without running it (dirty-state recovery);
                         v must be an existing version, or -1 for "nothing applied"`

// ErrUsage reports invalid command-line arguments.
var ErrUsage = errors.New("invalid arguments")

// ErrDirtySchema reports that a migration failed halfway.
var ErrDirtySchema = errors.New("schema is dirty")

// Migrate runs a migration subcommand and prints the resulting schema
// status to out. Migrations run without the request statement timeout
// because schema changes can legitimately take longer. A dirty schema is
// reported as an error so scripts can detect it from the exit code.
func Migrate(ctx context.Context, cfg config.Config, logger *zap.Logger, out io.Writer, args []string) (err error) {
	action, n, err := parseMigrateArgs(args)
	if err != nil {
		return err
	}

	dbCfg := cfg.Database
	dbCfg.StatementTimeout = 0
	sqlDB, err := database.OpenSQL(ctx, dbCfg, cfg.App.Name+"-migrate")
	if err != nil {
		return err
	}
	runner, err := dbmigrate.New(sqlDB)
	if err != nil {
		_ = sqlDB.Close()
		return err
	}
	defer func() { err = errors.Join(err, runner.Close()) }()

	release := runner.StopOnDone(ctx)
	actionErr := runMigrateAction(runner, action, n)
	release()
	if actionErr != nil {
		return actionErr
	}
	return reportStatus(runner, action, logger, out)
}

func reportStatus(runner *dbmigrate.Runner, action string, logger *zap.Logger, out io.Writer) error {
	status, err := runner.Status()
	if err != nil {
		return err
	}
	logger.Info("schema status",
		zap.String("action", action),
		zap.Uint("version", status.Version),
		zap.Bool("dirty", status.Dirty),
		zap.Bool("applied", status.Applied),
	)
	if _, err := fmt.Fprintf(out, "version=%d dirty=%t applied=%t\n", status.Version, status.Dirty, status.Applied); err != nil {
		return fmt.Errorf("write status: %w", err)
	}
	if status.Dirty {
		return fmt.Errorf("%w: version %d; repair the schema, then run `migrate force <version>`", ErrDirtySchema, status.Version)
	}
	return nil
}

func parseMigrateArgs(args []string) (string, int, error) {
	if len(args) == 0 {
		return "", 0, fmt.Errorf("%w: missing migrate action", ErrUsage)
	}
	action := args[0]
	switch action {
	case "up", "version":
		if len(args) != 1 {
			return "", 0, fmt.Errorf("%w: migrate %s takes no arguments", ErrUsage, action)
		}
		return action, 0, nil
	case "down", "force":
		if len(args) != 2 {
			return "", 0, fmt.Errorf("%w: migrate %s requires one number", ErrUsage, action)
		}
		n, err := strconv.Atoi(args[1])
		if err != nil || !validMigrateNumber(action, n) {
			return "", 0, fmt.Errorf("%w: invalid number %q for migrate %s", ErrUsage, args[1], action)
		}
		return action, n, nil
	default:
		return "", 0, fmt.Errorf("%w: unknown migrate action %q", ErrUsage, action)
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
