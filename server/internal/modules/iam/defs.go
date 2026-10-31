package iam

import (
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/perm"
)

// Name is the module name and permission prefix.
const Name = "iam"

var permissionDefs = []perm.Def{
	{Code: "iam:user:view", Name: "查看用户", Menu: "iam.user"},
	{Code: "iam:user:create", Name: "新增用户", Menu: "iam.user"},
	{Code: "iam:user:update", Name: "编辑用户", Menu: "iam.user"},
	{Code: "iam:user:delete", Name: "删除用户", Menu: "iam.user"},
	{Code: "iam:user:reset-password", Name: "重置密码", Menu: "iam.user"},
	{Code: "iam:user:export", Name: "导出用户", Menu: "iam.user"},
	{Code: "iam:user:import", Name: "导入用户", Menu: "iam.user"},

	{Code: "iam:role:view", Name: "查看角色", Menu: "iam.role"},
	{Code: "iam:role:create", Name: "新增角色", Menu: "iam.role"},
	{Code: "iam:role:update", Name: "编辑角色", Menu: "iam.role"},
	{Code: "iam:role:delete", Name: "删除角色", Menu: "iam.role"},
	{Code: "iam:role:grant", Name: "分配权限", Menu: "iam.role"},

	{Code: "iam:dept:view", Name: "查看部门", Menu: "iam.dept"},
	{Code: "iam:dept:create", Name: "新增部门", Menu: "iam.dept"},
	{Code: "iam:dept:update", Name: "编辑部门", Menu: "iam.dept"},
	{Code: "iam:dept:delete", Name: "删除部门", Menu: "iam.dept"},

	{Code: "iam:position:view", Name: "查看岗位", Menu: "iam.position"},
	{Code: "iam:position:create", Name: "新增岗位", Menu: "iam.position"},
	{Code: "iam:position:update", Name: "编辑岗位", Menu: "iam.position"},
	{Code: "iam:position:delete", Name: "删除岗位", Menu: "iam.position"},

	{Code: "iam:menu:view", Name: "查看菜单", Menu: "iam.menu"},
	{Code: "iam:menu:create", Name: "新增菜单", Menu: "iam.menu"},
	{Code: "iam:menu:update", Name: "编辑菜单", Menu: "iam.menu"},
	{Code: "iam:menu:delete", Name: "删除菜单", Menu: "iam.menu"},
}

func page(code, title, path, component, permission, icon string, sort int) menu.Def {
	return menu.Def{
		Code: code, Parent: "iam.system", Type: menu.TypeMenu, Title: title, Path: path,
		Component: component, Permission: permission, Icon: icon, Sort: sort,
	}
}

var menuDefs = []menu.Def{
	{Code: "iam.system", Type: menu.TypeDir, Title: "系统管理", Path: "/system", Icon: "ri:settings-3-line", Sort: 90},
	page("iam.user", "用户管理", "/system/user", "system/user/index", "iam:user:view", "ri:user-3-line", 1),
	page("iam.role", "角色管理", "/system/role", "system/role/index", "iam:role:view", "ri:admin-line", 2),
	page("iam.dept", "部门管理", "/system/dept", "system/dept/index", "iam:dept:view", "ri:git-branch-line", 3),
	page("iam.position", "岗位管理", "/system/position", "system/position/index", "iam:position:view", "ri:briefcase-line", 4),
	page("iam.menu", "菜单管理", "/system/menu", "system/menu/index", "iam:menu:view", "ep:menu", 5),
}
