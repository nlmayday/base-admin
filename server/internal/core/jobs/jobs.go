// Package jobs defines scheduled job handlers. Modules contribute handlers
// by implementing Provider; the ops module schedules and runs them. Only
// handlers registered in code can run: there is no arbitrary command
// execution (design doc 3.6 / v1 3.6).
package jobs

import (
	"context"
	"encoding/json"
	"time"
)

// Handler is one runnable job type.
type Handler struct {
	// Key is a stable identifier such as "audit.cleanup"; it must start
	// with the providing module's name and a dot.
	Key         string
	Name        string
	Description string
	// DefaultTimeout bounds one run unless the job overrides it.
	DefaultTimeout time.Duration
	// Run executes the job; params is the job's configured JSON (may be
	// empty). It must honour ctx cancellation.
	Run func(ctx context.Context, params json.RawMessage) error
}

// Provider is implemented by modules that contribute job handlers.
type Provider interface {
	Jobs() []Handler
}
