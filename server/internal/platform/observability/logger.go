// Package observability provides logging, metrics and health checks.
package observability

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"base-admin/server/internal/platform/config"
)

// NewLogger builds a zap logger: JSON for production, human-readable
// console output for local development.
func NewLogger(cfg config.LogConfig) (*zap.Logger, error) {
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}

	zc := zap.NewProductionConfig()
	if cfg.Format == "console" {
		zc = zap.NewDevelopmentConfig()
	}
	zc.Level = zap.NewAtomicLevelAt(level)

	logger, err := zc.Build()
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}
	return logger, nil
}
