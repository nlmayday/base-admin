package perm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParts(t *testing.T) {
	m, r, a, ok := Parts("iam:user:reset-password")
	assert.True(t, ok)
	assert.Equal(t, []string{"iam", "user", "reset-password"}, []string{m, r, a})

	for _, bad := range []string{"", "iam:user", "iam:user:view:x", "IAM:user:view", "iam::view", "iam:user:9x", "iam:user:view "} {
		assert.False(t, ValidCode(bad), bad)
	}
	assert.Equal(t, "iam:user:view", ViewCode("iam:user:delete"))
	assert.Empty(t, ViewCode("bad"))
	assert.True(t, IsView("iam:user:view"))
	assert.False(t, IsView("iam:user:create"))
}

func TestEffectiveRequires(t *testing.T) {
	assert.Equal(t, []string{"iam:user:view"}, Def{Code: "iam:user:create"}.EffectiveRequires(), "default depends on view")
	assert.Equal(t, []string{}, Def{Code: "iam:user:view"}.EffectiveRequires(), "view has no default")
	assert.Equal(t, []string{}, Def{Code: "iam:user:create", Requires: []string{}}.EffectiveRequires(), "explicit opt-out")
	assert.Equal(t, []string{"iam:role:view"}, Def{Code: "iam:role:grant", Requires: []string{"iam:role:view"}}.EffectiveRequires())
}

func TestMissingDependencies(t *testing.T) {
	c := NewCatalog([]Def{
		{Code: "iam:user:view"},
		{Code: "iam:user:create"},
		{Code: "iam:user:delete"},
		{Code: "iam:role:view"},
		{Code: "iam:role:grant", Requires: []string{"iam:role:view", "iam:user:view"}},
	})
	missing, unknown := c.MissingDependencies([]string{"iam:user:create", "iam:user:delete", "iam:role:grant", "iam:ghost:view"})
	assert.Equal(t, []string{"iam:role:view", "iam:user:view"}, missing)
	assert.Equal(t, []string{"iam:ghost:view"}, unknown)

	missing, unknown = c.MissingDependencies([]string{"iam:user:view", "iam:user:create"})
	assert.Empty(t, missing)
	assert.Empty(t, unknown)

	assert.True(t, c.Has("iam:user:view"))
	assert.Equal(t, []string{"iam:role:view", "iam:user:view"}, c.Requires("iam:role:grant"))
	assert.Len(t, c.Codes(), 5)
	d, ok := c.Get("iam:user:view")
	assert.True(t, ok)
	assert.Equal(t, "iam:user:view", d.Code)
}

func TestFindCycle(t *testing.T) {
	assert.Nil(t, FindCycle(map[string][]string{"a": {"b"}, "b": {"c"}, "c": nil}))
	cycle := FindCycle(map[string][]string{"a": {"b"}, "b": {"c"}, "c": {"a"}})
	assert.Equal(t, "a -> b -> c -> a", FormatCycle(cycle))
	assert.Equal(t, []string{"x", "x"}, FindCycle(map[string][]string{"x": {"x"}}))
}
