package datascope

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeUnion(t *testing.T) {
	dept := &UserDept{ID: 5, Path: "/1/5/"}
	cases := []struct {
		name  string
		dept  *UserDept
		roles []RoleScope
		want  Resolved
	}{
		{"no roles sees nothing", dept, nil, Resolved{}},
		{"all wins", dept, []RoleScope{{Type: Self}, {Type: All}}, Resolved{All: true}},
		{"self", dept, []RoleScope{{Type: Self}}, Resolved{Self: true}},
		{"dept", dept, []RoleScope{{Type: Dept}}, Resolved{DeptIDs: []int64{5}}},
		{"dept and children", dept, []RoleScope{{Type: DeptAndChildren}}, Resolved{DeptSubtree: []string{"/1/5/"}}},
		{"union of self, dept and custom", dept,
			[]RoleScope{{Type: Self}, {Type: Dept}, {Type: Custom, CustomDeptIDs: []int64{9, 5}}},
			Resolved{Self: true, DeptIDs: []int64{5, 9}}},
		{"dept scopes without a department", nil, []RoleScope{{Type: Dept}, {Type: DeptAndChildren}}, Resolved{}},
		{"duplicate subtree once", dept, []RoleScope{{Type: DeptAndChildren}, {Type: DeptAndChildren}}, Resolved{DeptSubtree: []string{"/1/5/"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Merge(tc.dept, tc.roles)
			assert.Equal(t, tc.want.All, got.All)
			assert.Equal(t, tc.want.Self, got.Self)
			assert.ElementsMatch(t, tc.want.DeptIDs, got.DeptIDs)
			assert.ElementsMatch(t, tc.want.DeptSubtree, got.DeptSubtree)
		})
	}
	assert.True(t, Merge(nil, nil).Empty())
}

func TestCondition(t *testing.T) {
	spec := Spec{DeptColumn: "u.dept_id", OwnerColumn: "u.id"}

	cond, args := Condition(Resolved{All: true}, 7, spec)
	assert.Equal(t, "TRUE", cond)
	assert.Nil(t, args)

	cond, _ = Condition(Resolved{}, 7, spec)
	assert.Equal(t, "FALSE", cond, "empty scope fails closed")

	cond, args = Condition(Resolved{Self: true, DeptIDs: []int64{3}, DeptSubtree: []string{"/1/_%/"}}, 7, spec)
	assert.Equal(t, "(u.id = ? OR u.dept_id IN ? OR u.dept_id IN (SELECT id FROM sys_departments WHERE path LIKE ?))", cond)
	assert.Equal(t, []any{int64(7), []int64{3}, `/1/\_\%/%`}, args)

	cond, _ = Condition(Resolved{Self: true}, 7, Spec{DeptColumn: "d.id"})
	assert.Equal(t, "FALSE", cond, "self on a resource without owner column grants nothing")
}

func TestTypeValid(t *testing.T) {
	for _, ty := range []Type{All, DeptAndChildren, Dept, Self, Custom} {
		assert.True(t, ty.Valid())
	}
	assert.False(t, Type("everything").Valid())
}
