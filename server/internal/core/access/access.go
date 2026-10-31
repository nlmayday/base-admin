// Package access defines the access level every route must declare
// (design doc 4.2). A route cannot be registered without one.
package access

import (
	"slices"
	"strings"
)

// Kind is the category of an access level.
type Kind int

// Access kinds.
const (
	KindPublic Kind = iota + 1
	KindAuthenticated
	KindPermission
)

func (k Kind) String() string {
	switch k {
	case KindPublic:
		return "public"
	case KindAuthenticated:
		return "authenticated"
	case KindPermission:
		return "permission"
	default:
		return "invalid"
	}
}

// Level is an immutable access requirement.
type Level struct {
	kind  Kind
	codes []string
}

// Public routes need no session.
func Public() Level { return Level{kind: KindPublic} }

// Authenticated routes need a valid session but no permission code.
func Authenticated() Level { return Level{kind: KindAuthenticated} }

// Perm routes need the given permission code.
func Perm(code string) Level { return Level{kind: KindPermission, codes: []string{code}} }

// AnyPerm routes need at least one of the given codes.
func AnyPerm(codes ...string) Level {
	return Level{kind: KindPermission, codes: append([]string(nil), codes...)}
}

// Kind returns the level's kind; the zero Level is invalid.
func (l Level) Kind() Kind { return l.kind }

// Codes returns a copy of the permission codes, any of which grants access.
func (l Level) Codes() []string { return append([]string(nil), l.codes...) }

// Valid reports whether the level was built by a constructor.
func (l Level) Valid() bool {
	switch l.kind {
	case KindPublic, KindAuthenticated:
		return true
	case KindPermission:
		return len(l.codes) > 0
	default:
		return false
	}
}

// Allows reports whether a holder of has satisfies a permission level.
// Callers handle public and authenticated levels before calling it.
func (l Level) Allows(has func(code string) bool) bool {
	return slices.ContainsFunc(l.codes, has)
}

func (l Level) String() string {
	if l.kind != KindPermission {
		return l.kind.String()
	}
	return "perm(" + strings.Join(l.codes, "|") + ")"
}
