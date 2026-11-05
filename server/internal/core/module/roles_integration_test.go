//go:build integration

package module

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"base-admin/server/internal/core/perm"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/testinfra/migrated"
)

func TestSyncRoles(t *testing.T) {
	ctx := context.Background()
	cfg := migrated.Database(t)
	sqlDB, err := database.OpenSQL(ctx, cfg, "module-roles-test")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := database.NewGorm(sqlDB, cfg, zap.NewNop())
	require.NoError(t, err)

	mod := roleModule{
		fakeModule: fakeModule{name: "game", perms: []perm.Def{
			{Code: "game:item:view", Name: "查看道具"}, {Code: "game:item:update", Name: "编辑道具"},
			{Code: "game:order:view", Name: "查看订单"},
		}},
		roles: []RoleDef{{Code: "game_ops", Name: "运营", DataScope: "all", Remark: "代码声明",
			Permissions: []string{"game:item:*"}}},
	}
	grants := func() []string {
		var codes []string
		require.NoError(t, db.Raw(`SELECT rp.permission_code FROM sys_role_permissions rp
			JOIN sys_roles r ON r.id = rp.role_id WHERE r.code = 'game_ops' ORDER BY rp.permission_code`).Scan(&codes).Error)
		return codes
	}
	require.NoError(t, Sync(ctx, db, []Module{mod}))
	var role struct {
		Name, DataScope, Remark string
	}
	require.NoError(t, db.Raw("SELECT name, data_scope, remark FROM sys_roles WHERE code = 'game_ops'").Scan(&role).Error)
	assert.Equal(t, "运营", role.Name)
	assert.Equal(t, "all", role.DataScope)
	assert.Equal(t, []string{"game:item:update", "game:item:view"}, grants())

	// An administrator takes a permission away and renames the role: a
	// restart keeps both.
	require.NoError(t, db.Exec(`DELETE FROM sys_role_permissions WHERE permission_code = 'game:item:update'`).Error)
	require.NoError(t, db.Exec(`UPDATE sys_roles SET name = '运营组' WHERE code = 'game_ops'`).Error)
	require.NoError(t, Sync(ctx, db, []Module{mod}))
	assert.Equal(t, []string{"game:item:view"}, grants())
	require.NoError(t, db.Raw("SELECT name, data_scope, remark FROM sys_roles WHERE code = 'game_ops'").Scan(&role).Error)
	assert.Equal(t, "运营组", role.Name)

	// A later version adds a resource the role covers: its new codes are
	// granted; an old code newly listed is not.
	mod.perms = append(mod.perms, perm.Def{Code: "game:item:export", Name: "导出道具"})
	mod.roles[0].Permissions = []string{"game:item:*", "game:order:view"}
	require.NoError(t, Sync(ctx, db, []Module{mod}))
	assert.Equal(t, []string{"game:item:export", "game:item:view"}, grants())
}
