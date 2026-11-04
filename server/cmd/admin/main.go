// Command admin runs the base-admin API server and its maintenance tasks.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"go.uber.org/zap"

	"base-admin/server/internal/app"
	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/observability"
)

const usage = `usage: admin [-config file] <command>

commands:
  serve                  run the API server (default)
  health [live|ready]    probe the running server (default ready); for container healthchecks
  %s

configuration: defaults < config file (-config or ADMIN_CONFIG) < ADMIN_* environment variables
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("admin", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { _, _ = fmt.Fprintf(stderr, usage, indent(app.MigrateUsage)) }
	configPath := flags.String("config", os.Getenv("ADMIN_CONFIG"), "path to a YAML config file")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "config error:", err)
		return 1
	}
	logger, err := observability.NewLogger(cfg.Log)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "logger error:", err)
		return 1
	}
	defer func() { _ = logger.Sync() }()
	// zap.L() is this logger too: generated modules log through it (the
	// AfterCommit hooks) without a logger in their Deps.
	defer zap.ReplaceGlobals(logger)()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// After the first signal starts a graceful stop, restore default signal
	// handling so that a second signal terminates the process immediately.
	go func() {
		<-ctx.Done()
		stop()
	}()

	if err := dispatch(ctx, cfg, logger, stdout, flags.Args()); err != nil {
		if errors.Is(err, app.ErrUsage) {
			_, _ = fmt.Fprintln(stderr, err)
			flags.Usage()
			return 2
		}
		logger.Error("command failed", zap.Error(err))
		return 1
	}
	return 0
}

func dispatch(ctx context.Context, cfg config.Config, logger *zap.Logger, stdout io.Writer, args []string) error {
	if len(args) == 0 {
		return app.Serve(ctx, cfg, logger)
	}
	switch args[0] {
	case "serve":
		if len(args) != 1 {
			return fmt.Errorf("%w: serve takes no arguments", app.ErrUsage)
		}
		return app.Serve(ctx, cfg, logger)
	case "migrate":
		return app.Migrate(ctx, cfg, logger, stdout, args[1:])
	case "health":
		return probeHealth(ctx, cfg.HTTP.Addr, args[1:])
	default:
		return fmt.Errorf("%w: unknown command %q", app.ErrUsage, args[0])
	}
}

func indent(s string) string {
	return strings.ReplaceAll(s, "\n", "\n  ")
}
