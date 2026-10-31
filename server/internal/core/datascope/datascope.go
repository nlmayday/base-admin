// Package datascope implements row-level data permissions (design doc 7.3):
// per-role scope types, their multi-role union, and the query filter every
// scoped repository must apply.
package datascope

import (
	"slices"
	"strings"
)

// Type is a role's data scope.
type Type string

// Scope types.
const (
	All             Type = "all"
	DeptAndChildren Type = "dept_and_children"
	Dept            Type = "dept"
	Self            Type = "self"
	Custom          Type = "custom"
)

// Valid reports whether t is a known scope type.
func (t Type) Valid() bool {
	switch t {
	case All, DeptAndChildren, Dept, Self, Custom:
		return true
	default:
		return false
	}
}

// RoleScope is one enabled role's scope configuration.
type RoleScope struct {
	Type          Type
	CustomDeptIDs []int64
}

// Resolved is the union of a user's role scopes. An empty value (no field
// set) grants no rows.
type Resolved struct {
	All         bool     `json:"all"`
	Self        bool     `json:"self"`
	DeptIDs     []int64  `json:"deptIds"`
	DeptSubtree []string `json:"deptSubtree"`
}

// Empty reports whether the scope grants nothing.
func (r Resolved) Empty() bool {
	return !r.All && !r.Self && len(r.DeptIDs) == 0 && len(r.DeptSubtree) == 0
}

// UserDept is the user's own department, if any.
type UserDept struct {
	ID   int64
	Path string
}

// Merge unions role scopes. There are no deny rules: any role granting
// "all" yields All. Dept-relative scopes contribute nothing when the user
// has no department.
func Merge(dept *UserDept, roles []RoleScope) Resolved {
	var r Resolved
	ids := map[int64]struct{}{}
	for _, role := range roles {
		switch role.Type {
		case All:
			return Resolved{All: true}
		case Self:
			r.Self = true
		case Dept:
			if dept != nil {
				ids[dept.ID] = struct{}{}
			}
		case DeptAndChildren:
			if dept != nil && dept.Path != "" && !slices.Contains(r.DeptSubtree, dept.Path) {
				r.DeptSubtree = append(r.DeptSubtree, dept.Path)
			}
		case Custom:
			for _, id := range role.CustomDeptIDs {
				ids[id] = struct{}{}
			}
		}
	}
	for id := range ids {
		r.DeptIDs = append(r.DeptIDs, id)
	}
	slices.Sort(r.DeptIDs)
	slices.Sort(r.DeptSubtree)
	return r
}

// Spec names the columns a resource is isolated by. Column names must be
// trusted constants, never user input.
type Spec struct {
	// DeptColumn holds the owning department id, e.g. "sys_users.dept_id".
	DeptColumn string
	// OwnerColumn holds the owning user id, e.g. "sys_users.id".
	OwnerColumn string
}

// Condition renders the scope as a SQL boolean expression with arguments.
func Condition(r Resolved, userID int64, spec Spec) (string, []any) {
	if r.All {
		return "TRUE", nil
	}
	var parts []string
	var args []any
	if r.Self && spec.OwnerColumn != "" {
		parts = append(parts, spec.OwnerColumn+" = ?")
		args = append(args, userID)
	}
	if spec.DeptColumn != "" {
		if len(r.DeptIDs) > 0 {
			parts = append(parts, spec.DeptColumn+" IN ?")
			args = append(args, r.DeptIDs)
		}
		for _, prefix := range r.DeptSubtree {
			parts = append(parts, spec.DeptColumn+" IN (SELECT id FROM sys_departments WHERE path LIKE ?)")
			args = append(args, escapeLike(prefix)+"%")
		}
	}
	if len(parts) == 0 {
		return "FALSE", nil
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
