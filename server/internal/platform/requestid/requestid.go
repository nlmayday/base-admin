// Package requestid generates request identifiers and carries them in a
// context.Context so that services can correlate logs without HTTP types.
package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// Header is the HTTP header used to read and echo the request ID.
const Header = "X-Request-ID"

const (
	prefix    = "req_"
	maxLength = 64
)

type ctxKey struct{}

// New returns a random request ID such as "req_3f9a...".
func New() string {
	b := make([]byte, 12)
	// crypto/rand.Read never returns an error on supported platforms.
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// Valid reports whether an externally supplied ID is safe to reuse in logs
// and response headers.
func Valid(id string) bool {
	if id == "" || len(id) > maxLength {
		return false
	}
	for _, r := range id {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !isAlnum && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}

// With returns a copy of ctx carrying id.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// From returns the request ID in ctx, or "" if none is set.
func From(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}
