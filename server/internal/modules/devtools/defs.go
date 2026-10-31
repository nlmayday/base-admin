package devtools

import (
	"net/http"

	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/perm"
	"base-admin/server/internal/platform/httpx"
)

// Name is the module name and permission prefix.
const Name = "devtools"

// Permission codes.
const (
	PermAPIDoc  = "devtools:apidoc:view"
	PermCodegen = "devtools:codegen:view"
	// PermAgentKeys manages the keys of the AI agent API.
	PermAgentKeys = "devtools:agent:view"
)

// Devtools error codes (8xxxx, docs/p1-p3-plan.md).
const (
	CodeCodegenDisabled = 80001
	// CodeCodegenExists: generated files already exist (409).
	CodeCodegenExists = 80002
	// CodeCodegenNoRepo: the repository root was not found (409).
	CodeCodegenNoRepo = 80003
	// CodeMigrationState: the database is not at the last migration on
	// disk, so the agent API will not add one (409).
	CodeMigrationState = 80004
)

var errCodegenDisabled = httpx.NewError(http.StatusForbidden, CodeCodegenDisabled,
	"代码生成仅在开发环境（app.env=dev）可用")

var permissionDefs = []perm.Def{
	{Code: PermAPIDoc, Name: "查看接口文档", Menu: "devtools.apidoc"},
	{Code: PermCodegen, Name: "使用代码生成", Menu: "devtools.codegen"},
	{Code: PermAgentKeys, Name: "管理 AI 接口密钥", Menu: "devtools.agent"},
}

var menuDefs = []menu.Def{
	{Code: "devtools.root", Type: menu.TypeDir, Title: "开发工具", Path: "/devtools", Icon: "ri:code-box-line", Sort: 96},
	{
		Code: "devtools.apidoc", Parent: "devtools.root", Type: menu.TypeMenu, Title: "接口文档",
		Path: "/devtools/apidoc", Component: "devtools/apidoc/index", Permission: PermAPIDoc,
		Icon: "ri:file-list-3-line", Sort: 1,
	},
	{
		Code: "devtools.codegen", Parent: "devtools.root", Type: menu.TypeMenu, Title: "代码生成",
		Path: "/devtools/codegen", Component: "devtools/codegen/index", Permission: PermCodegen,
		Icon: "ri:magic-line", Sort: 2,
	},
	{
		Code: "devtools.agent", Parent: "devtools.root", Type: menu.TypeMenu, Title: "AI 接口",
		Path: "/devtools/agent", Component: "devtools/agent/index", Permission: PermAgentKeys,
		Icon: "ri:robot-2-line", Sort: 3,
	},
}

// moduleTitles names the known modules in the API documentation.
var moduleTitles = map[string]string{
	"auth":     "认证与安全",
	"iam":      "系统管理",
	"system":   "系统配置",
	"audit":    "日志审计",
	"file":     "资源管理",
	"ops":      "运维管理",
	"devtools": "开发工具",
}
