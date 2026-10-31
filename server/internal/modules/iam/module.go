// Package iam implements users, roles, departments, positions, menus,
// permission grants and the current-user endpoints (design doc section 7).
package iam

import (
	"context"
	"fmt"
	"slices"

	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/core/perm"
	"base-admin/server/internal/modules/auth"
	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/httpx"
)

// Module is the IAM module.
type Module struct {
	db        *gorm.DB
	core      *auth.Core
	registry  *module.Registry
	bootstrap config.BootstrapConfig
}

var _ module.Module = (*Module)(nil)

// New creates the IAM module. registry is read by the API list endpoint.
func New(db *gorm.DB, core *auth.Core, registry *module.Registry, bootstrap config.BootstrapConfig) *Module {
	return &Module{db: db, core: core, registry: registry, bootstrap: bootstrap}
}

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Permissions implements module.Module.
func (m *Module) Permissions() []perm.Def { return slices.Clone(permissionDefs) }

// Menus implements module.Module.
func (m *Module) Menus() []menu.Def { return slices.Clone(menuDefs) }

// Routes implements module.Module.
func (m *Module) Routes(r *module.Router) {
	r.GET("/me", access.Authenticated(), handle(m.getMe)).Doc(module.Doc{Summary: "当前用户", Response: meView{}})
	r.GET("/me/menus", access.Authenticated(), handle(m.myMenus)).Doc(module.Doc{Summary: "当前用户菜单", Response: []asyncRoute{}})
	r.PUT("/me/profile", access.Authenticated(), handle(m.updateProfile)).Doc(module.Doc{Summary: "修改个人资料", Body: profileRequest{}, Response: meView{}})

	users := r.Group("/users")
	users.GET("", access.Perm("iam:user:view"), handle(m.listUsers)).Doc(module.Doc{Summary: "用户列表", Query: userFilter{}, Response: httpx.Page[userView]{}})
	users.GET("/options", access.Authenticated(), handle(m.userOptions)).Doc(module.Doc{Summary: "用户下拉选项", Response: []userOption{}})
	users.GET("/export", access.Perm("iam:user:export"), m.exportUsers).Doc(module.Doc{Summary: "导出用户", Query: userFilter{}, Sensitive: true})
	users.GET("/import-template", access.Perm("iam:user:import"), m.importTemplate).Doc(module.Doc{Summary: "下载用户导入模板"})
	users.POST("/import", access.Perm("iam:user:import"), handle(m.importUsers)).
		MaxBody(maxImportBytes + 64<<10).
		Doc(module.Doc{Summary: "导入用户", Description: "multipart：file 为 CSV（UTF-8 或 GBK），password 为初始密码；任一行有误则整批不导入，错误在 details.rows 中逐行返回", Response: importResult{}})
	users.GET("/:id", access.Perm("iam:user:view"), handle(m.getUser)).Doc(module.Doc{Summary: "用户详情", Response: userView{}})
	users.POST("", access.Perm("iam:user:create"), handle(m.createUser)).Doc(module.Doc{Summary: "新增用户", Body: createUserRequest{}, Response: userView{}})
	users.PUT("/:id", access.Perm("iam:user:update"), handle(m.updateUser)).Doc(module.Doc{Summary: "编辑用户", Body: updateUserRequest{}, Response: userView{}})
	users.PUT("/:id/status", access.Perm("iam:user:update"), handle(m.setUserStatus)).Doc(module.Doc{Summary: "启用/停用用户", Body: statusRequest{}, Response: userView{}})
	users.PUT("/:id/password", access.Perm("iam:user:reset-password"), handle(m.resetPassword)).Doc(module.Doc{Summary: "重置用户密码", Body: resetPasswordRequest{}})
	users.DELETE("/:id", access.Perm("iam:user:delete"), handle(m.deleteUser)).Doc(module.Doc{Summary: "删除用户"})

	roles := r.Group("/roles")
	roles.GET("", access.Perm("iam:role:view"), handle(m.listRoles)).Doc(module.Doc{Summary: "角色列表", Response: httpx.Page[roleView]{}})
	roles.GET("/options", access.Authenticated(), handle(m.roleOptions)).Doc(module.Doc{Summary: "角色下拉选项", Response: []option{}})
	roles.POST("", access.Perm("iam:role:create"), handle(m.createRole)).Doc(module.Doc{Summary: "新增角色", Body: roleRequest{}, Response: roleView{}})
	roles.PUT("/:id", access.Perm("iam:role:update"), handle(m.updateRole)).Doc(module.Doc{Summary: "编辑角色", Body: roleRequest{}, Response: roleView{}})
	roles.DELETE("/:id", access.Perm("iam:role:delete"), handle(m.deleteRole)).Doc(module.Doc{Summary: "删除角色"})
	roles.GET("/:id/permissions", access.Perm("iam:role:grant"), handle(m.getRolePermissions)).Doc(module.Doc{Summary: "角色权限", Response: grantPayload{}})
	roles.PUT("/:id/permissions", access.Perm("iam:role:grant"), handle(m.setRolePermissions)).Doc(module.Doc{Summary: "分配角色权限", Body: grantPayload{}, Response: grantPayload{}})
	r.GET("/permissions/tree", access.Perm("iam:role:grant"), handle(m.permissionTree)).Doc(module.Doc{Summary: "权限树", Response: []*permNode{}})

	depts := r.Group("/departments")
	depts.GET("/tree", access.AnyPerm("iam:dept:view", "iam:user:view"), handle(m.deptTree)).Doc(module.Doc{Summary: "部门树", Response: []*deptView{}})
	depts.POST("", access.Perm("iam:dept:create"), handle(m.createDept)).Doc(module.Doc{Summary: "新增部门", Body: deptRequest{}, Response: deptView{}})
	depts.PUT("/:id", access.Perm("iam:dept:update"), handle(m.updateDept)).Doc(module.Doc{Summary: "编辑部门", Body: deptRequest{}, Response: deptView{}})
	depts.DELETE("/:id", access.Perm("iam:dept:delete"), handle(m.deleteDept)).Doc(module.Doc{Summary: "删除部门"})

	positions := r.Group("/positions")
	positions.GET("", access.Perm("iam:position:view"), handle(m.listPositions)).Doc(module.Doc{Summary: "岗位列表", Response: httpx.Page[positionView]{}})
	positions.GET("/options", access.Authenticated(), handle(m.positionOptions)).Doc(module.Doc{Summary: "岗位下拉选项", Response: []option{}})
	positions.POST("", access.Perm("iam:position:create"), handle(m.createPosition)).Doc(module.Doc{Summary: "新增岗位", Body: positionRequest{}, Response: positionView{}})
	positions.PUT("/:id", access.Perm("iam:position:update"), handle(m.updatePosition)).Doc(module.Doc{Summary: "编辑岗位", Body: positionRequest{}, Response: positionView{}})
	positions.DELETE("/:id", access.Perm("iam:position:delete"), handle(m.deletePosition)).Doc(module.Doc{Summary: "删除岗位"})

	menus := r.Group("/menus")
	menus.GET("/tree", access.Perm("iam:menu:view"), handle(m.menuTree)).Doc(module.Doc{Summary: "菜单树", Response: []*menuNode{}})
	menus.POST("", access.Perm("iam:menu:create"), handle(m.createMenu)).Doc(module.Doc{Summary: "新增菜单", Body: menuRequest{}, Response: menuNode{}})
	menus.PUT("/:id", access.Perm("iam:menu:update"), handle(m.updateMenu)).Doc(module.Doc{Summary: "编辑菜单", Body: menuRequest{}, Response: menuNode{}})
	menus.DELETE("/:id", access.Perm("iam:menu:delete"), handle(m.deleteMenu)).Doc(module.Doc{Summary: "删除菜单"})
}

// Seed creates the built-in super admin role and, on an empty system, the
// first administrator (design doc 4.4). It is idempotent.
func (m *Module) Seed(ctx context.Context, tx *gorm.DB) error {
	if err := tx.Exec(`INSERT INTO sys_roles (code, name, data_scope, is_builtin, sort, remark)
		VALUES (?, '超级管理员', 'all', true, 0, '内置角色，拥有全部权限') ON CONFLICT (code) DO NOTHING`,
		auth.SuperAdminRole).Error; err != nil {
		return fmt.Errorf("seed super admin role: %w", err)
	}
	var users int64
	if err := tx.Raw("SELECT count(*) FROM sys_users").Scan(&users).Error; err != nil {
		return err
	}
	if users > 0 {
		return nil
	}
	if m.bootstrap.AdminPassword == "" {
		return fmt.Errorf("no user exists: set ADMIN_BOOTSTRAP_ADMIN_PASSWORD to create the first administrator")
	}
	hash, err := m.core.HashPassword(ctx, m.bootstrap.AdminPassword)
	if err != nil {
		return err
	}
	var id int64
	if err := tx.Raw(`INSERT INTO sys_users (username, password_hash, nickname, must_change_password)
		VALUES (?, ?, '超级管理员', true) RETURNING id`, m.bootstrap.AdminUsername, hash).Scan(&id).Error; err != nil {
		return fmt.Errorf("seed admin user: %w", err)
	}
	return tx.Exec(`INSERT INTO sys_user_roles (user_id, role_id) SELECT ?, id FROM sys_roles WHERE code = ?`,
		id, auth.SuperAdminRole).Error
}
