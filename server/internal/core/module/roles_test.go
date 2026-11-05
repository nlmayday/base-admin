package module

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"base-admin/server/internal/core/perm"
)

type roleModule struct {
	fakeModule
	roles []RoleDef
}

func (m roleModule) Roles() []RoleDef { return m.roles }

func gameDefs() map[string]perm.Def {
	defs := map[string]perm.Def{}
	for _, d := range []perm.Def{
		{Code: "game:item:view"}, {Code: "game:item:update"}, {Code: "game:item:delete"},
		{Code: "game:order:view"}, {Code: "game:order:export"}, {Code: "iam:user:view"},
	} {
		defs[d.Code] = d
	}
	return defs
}

func TestExpandPermissions(t *testing.T) {
	defs := gameDefs()
	assert.Equal(t, []string{"game:item:delete", "game:item:update", "game:item:view", "game:order:export", "game:order:view"},
		expandPermissions([]string{"game:*"}, defs))
	assert.Equal(t, []string{"game:order:export", "game:order:view"}, expandPermissions([]string{"game:order:export"}, defs),
		"the view code it requires comes along")
	assert.Empty(t, expandPermissions([]string{"shop:*", "game:nope:view"}, defs))
}

func TestValidateRoles(t *testing.T) {
	defs := gameDefs()
	mod := roleModule{fakeModule: fakeModule{name: "game"}, roles: []RoleDef{
		{Code: "game_ops", Name: "运营", Permissions: []string{"game:*"}},
		{Code: "game_ops", Name: "重复"},
		{Code: "Bad Code", Name: "x"},
		{Code: "cs", Name: "客服", DataScope: "everyone"},
		{Code: "viewer", Name: "只读", Permissions: []string{"game:order:view", "shop:*"}},
	}}
	var rep Report
	validateRoles(&rep, []Module{mod}, defs)
	assert.Len(t, rep.Errors, 4)
	joined := ""
	for _, e := range rep.Errors {
		joined += e + "\n"
	}
	for _, want := range []string{"declared twice", `"Bad Code" is invalid`, `data scope "everyone"`, `"shop:*" matches no`} {
		assert.Contains(t, joined, want)
	}
}
