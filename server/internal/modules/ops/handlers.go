package ops

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"base-admin/server/internal/core/jobs"
)

const (
	// defaultJobTimeout applies when neither the job nor its handler sets one.
	defaultJobTimeout = 5 * time.Minute
	maxJobTimeout     = 24 * time.Hour
)

// handlerKeyPattern is "<module>.<name>", e.g. "ops.cleanup-runs".
var handlerKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*\.[a-z0-9][a-z0-9.-]*$`)

// handlerSet is the immutable registry of runnable handlers. Only handlers
// registered in code can run (design doc 3.6).
type handlerSet struct {
	byKey map[string]jobs.Handler
	order []string
}

// newHandlerSet validates handlers. builtin handlers come first; a later
// handler with a builtin key is ignored so the app may pass the module's own
// handlers again. Any other duplicate is an error.
func newHandlerSet(builtin, others []jobs.Handler) (handlerSet, error) {
	set := handlerSet{byKey: map[string]jobs.Handler{}}
	own := map[string]bool{}
	var errs []error
	add := func(h jobs.Handler, isBuiltin bool) {
		if err := validateHandler(h); err != nil {
			errs = append(errs, err)
			return
		}
		if _, dup := set.byKey[h.Key]; dup {
			if !isBuiltin && own[h.Key] {
				return
			}
			errs = append(errs, fmt.Errorf("job handler %q is registered twice", h.Key))
			return
		}
		if isBuiltin {
			own[h.Key] = true
		}
		set.byKey[h.Key] = h
		set.order = append(set.order, h.Key)
	}
	for _, h := range builtin {
		add(h, true)
	}
	for _, h := range others {
		add(h, false)
	}
	return set, errors.Join(errs...)
}

func validateHandler(h jobs.Handler) error {
	switch {
	case !handlerKeyPattern.MatchString(h.Key) || len(h.Key) > 64:
		return fmt.Errorf("job handler key %q must look like \"module.name\" (max 64 chars)", h.Key)
	case h.Name == "":
		return fmt.Errorf("job handler %q has no name", h.Key)
	case h.Run == nil:
		return fmt.Errorf("job handler %q has no Run function", h.Key)
	case h.DefaultTimeout < 0 || h.DefaultTimeout > maxJobTimeout:
		return fmt.Errorf("job handler %q default timeout must be within 0..24h", h.Key)
	}
	return nil
}

func (s handlerSet) get(key string) (jobs.Handler, bool) {
	h, ok := s.byKey[key]
	return h, ok
}

func (s handlerSet) list() []jobs.Handler {
	out := make([]jobs.Handler, len(s.order))
	for i, k := range s.order {
		out[i] = s.byKey[k]
	}
	return out
}

// effectiveTimeout is the job's timeout, else the handler default, else
// defaultJobTimeout.
func effectiveTimeout(jobSeconds int, h jobs.Handler) time.Duration {
	switch {
	case jobSeconds > 0:
		return time.Duration(jobSeconds) * time.Second
	case h.DefaultTimeout > 0:
		return h.DefaultTimeout
	default:
		return defaultJobTimeout
	}
}
