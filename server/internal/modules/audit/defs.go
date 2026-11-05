package audit

import (
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/perm"
	"base-admin/server/internal/modules/auth"
)

// Name is the module name and permission prefix.
const Name = "audit"

// Permission codes.
const (
	PermLoginLogView       = "audit:login-log:view"
	PermLoginLogExport     = "audit:login-log:export"
	PermOperationLogView   = "audit:operation-log:view"
	PermOperationLogExport = "audit:operation-log:export"
)

// Tables written and read by this module (constants, never user input).
const (
	tableLoginLogs     = "sys_login_logs"
	tableOperationLogs = "sys_operation_logs"
)

var permissionDefs = []perm.Def{
	{Code: PermLoginLogView, Name: "查看登录日志", Menu: "audit.login"},
	{Code: PermLoginLogExport, Name: "导出登录日志", Menu: "audit.login"},
	{Code: PermOperationLogView, Name: "查看操作日志", Menu: "audit.operation"},
	{Code: PermOperationLogExport, Name: "导出操作日志", Menu: "audit.operation"},
}

var menuDefs = []menu.Def{
	{Code: "audit.root", Type: menu.TypeDir, Title: "日志审计", Path: "/audit", Icon: "ri:file-list-3-line", Sort: 93},
	{
		Code: "audit.login", Parent: "audit.root", Type: menu.TypeMenu, Title: "登录日志", Path: "/audit/login-log",
		Component: "audit/login-log/index", Permission: PermLoginLogView, Icon: "ri:login-box-line", Sort: 1,
	},
	{
		Code: "audit.operation", Parent: "audit.root", Type: menu.TypeMenu, Title: "操作日志", Path: "/audit/operation-log",
		Component: "audit/operation-log/index", Permission: PermOperationLogView, Icon: "ri:history-line", Sort: 2,
	},
}

// reasonLabels translates login failure reasons for exports.
var reasonLabels = map[string]string{
	auth.ReasonBadCredentials: "账号或密码错误",
	auth.ReasonRateLimited:    "请求过于频繁",
	auth.ReasonCaptcha:        "验证码错误",
	auth.ReasonLocked:         "账号已锁定",
	auth.ReasonError:          "系统错误",
}

func reasonLabel(reason string) string {
	if label, ok := reasonLabels[reason]; ok {
		return label
	}
	return reason
}
