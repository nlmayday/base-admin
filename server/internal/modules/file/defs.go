package file

import (
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/perm"
)

// Name is the module name and permission prefix.
const Name = "file"

// Permission codes.
const (
	permFileView    = "file:file:view"
	permFileUpload  = "file:file:upload"
	permFileUpdate  = "file:file:update"
	permFileDelete  = "file:file:delete"
	permStorageView = "file:storage:view"
)

// Non-view codes leave Requires nil, which means "the view code of the same
// resource" (perm.Def.EffectiveRequires).
var permissionDefs = []perm.Def{
	{Code: permFileView, Name: "查看文件", Menu: "file.files"},
	{Code: permFileUpload, Name: "上传文件", Menu: "file.files"},
	{Code: permFileUpdate, Name: "编辑文件与分类", Menu: "file.files"},
	{Code: permFileDelete, Name: "删除文件与分类", Menu: "file.files"},
	{Code: permStorageView, Name: "查看存储配置", Menu: "file.storage"},
}

func page(code, title, path, component, permission, icon string, sort int) menu.Def {
	return menu.Def{
		Code: code, Parent: "file.root", Type: menu.TypeMenu, Title: title, Path: path,
		Component: component, Permission: permission, Icon: icon, Sort: sort,
	}
}

var menuDefs = []menu.Def{
	{Code: "file.root", Type: menu.TypeDir, Title: "资源管理", Path: "/file", Icon: "ri:folder-3-line", Sort: 94},
	page("file.files", "文件管理", "/file/files", "file/files/index", permFileView, "ri:folder-image-line", 1),
	page("file.storage", "存储配置", "/file/storage", "file/storage/index", permStorageView, "ri:hard-drive-2-line", 2),
}
