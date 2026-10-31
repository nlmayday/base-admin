package system

import (
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/perm"
)

// Name is the module name and permission prefix.
const Name = "system"

// Permission codes.
const (
	permDictView       = "system:dict:view"
	permDictCreate     = "system:dict:create"
	permDictUpdate     = "system:dict:update"
	permDictDelete     = "system:dict:delete"
	permConfigView     = "system:config:view"
	permConfigCreate   = "system:config:create"
	permConfigUpdate   = "system:config:update"
	permConfigDelete   = "system:config:delete"
	permSettingsView   = "system:settings:view"
	permSettingsUpdate = "system:settings:update"
)

// Non-view codes leave Requires nil, which means "the view code of the same
// resource" (perm.Def.EffectiveRequires).
var permissionDefs = []perm.Def{
	{Code: permDictView, Name: "查看字典", Menu: "system.dict"},
	{Code: permDictCreate, Name: "新增字典", Menu: "system.dict"},
	{Code: permDictUpdate, Name: "编辑字典", Menu: "system.dict"},
	{Code: permDictDelete, Name: "删除字典", Menu: "system.dict"},

	{Code: permConfigView, Name: "查看参数", Menu: "system.config"},
	{Code: permConfigCreate, Name: "新增参数", Menu: "system.config"},
	{Code: permConfigUpdate, Name: "编辑参数", Menu: "system.config"},
	{Code: permConfigDelete, Name: "删除参数", Menu: "system.config"},

	{Code: permSettingsView, Name: "查看基础设置", Menu: "system.settings"},
	{Code: permSettingsUpdate, Name: "修改基础设置", Menu: "system.settings"},
}

func page(code, title, path, component, permission, icon string, sort int) menu.Def {
	return menu.Def{
		Code: code, Parent: "system.root", Type: menu.TypeMenu, Title: title, Path: path,
		Component: component, Permission: permission, Icon: icon, Sort: sort,
	}
}

var menuDefs = []menu.Def{
	{Code: "system.root", Type: menu.TypeDir, Title: "系统配置", Path: "/sysconfig", Icon: "ri:equalizer-line", Sort: 91},
	page("system.dict", "数据字典", "/sysconfig/dict", "sysconfig/dict/index", permDictView, "ri:book-2-line", 1),
	page("system.config", "参数配置", "/sysconfig/params", "sysconfig/params/index", permConfigView, "ri:list-settings-line", 2),
	page("system.settings", "基础设置", "/sysconfig/settings", "sysconfig/settings/index", permSettingsView, "ri:settings-4-line", 3),
}
