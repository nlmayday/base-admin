// Package actor carries the authenticated user through context.Context
// (design doc 7.2). Services read it without depending on HTTP types.
package actor

import (
	"context"
	"slices"

	"gorm.io/gorm"

	"base-admin/server/internal/core/datascope"
)

// Status values shared by users, roles, departments and positions.
const (
	StatusEnabled  int16 = 1
	StatusDisabled int16 = 2
)

// Snapshot is the cached authorization view of a user. It is only a cache
// of database rows for authorization decisions and must never be used as
// the source of user profile data (design doc 5.6).
type Snapshot struct {
	Version            int64              `json:"version"`
	UserID             int64              `json:"userId"`
	Status             int16              `json:"status"`
	SessionGen         int64              `json:"sessionGen"`
	DeptID             *int64             `json:"deptId"`
	IsSuper            bool               `json:"isSuper"`
	MustChangePassword bool               `json:"mustChangePassword"`
	RoleIDs            []int64            `json:"roleIds"`
	Permissions        []string           `json:"permissions"` // sorted
	DataScope          datascope.Resolved `json:"dataScope"`
}

// HasPermission reports whether the snapshot grants code.
func (s *Snapshot) HasPermission(code string) bool {
	if s.IsSuper {
		return true
	}
	_, found := slices.BinarySearch(s.Permissions, code)
	return found
}

// Actor is the authenticated caller of one request.
type Actor struct {
	Snapshot    *Snapshot
	SessionHash string
	IP          string
	UserAgent   string
}

// UserID returns the caller's user id.
func (a *Actor) UserID() int64 { return a.Snapshot.UserID }

type ctxKey struct{}

// With returns a copy of ctx carrying a.
func With(ctx context.Context, a *Actor) context.Context {
	return context.WithValue(ctx, ctxKey{}, a)
}

// From returns the actor in ctx, or nil for anonymous requests.
func From(ctx context.Context) *Actor {
	a, _ := ctx.Value(ctxKey{}).(*Actor)
	return a
}

// Scope filters db to the rows the actor in ctx may see. Without an actor
// it matches nothing, so a missing authentication step fails closed.
func Scope(ctx context.Context, db *gorm.DB, spec datascope.Spec) *gorm.DB {
	a := From(ctx)
	if a == nil {
		return db.Where("FALSE")
	}
	cond, args := datascope.Condition(a.Snapshot.DataScope, a.UserID(), spec)
	return db.Where(cond, args...)
}
