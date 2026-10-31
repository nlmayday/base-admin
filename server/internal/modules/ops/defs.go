package ops

import (
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/perm"
)

// Name is the module name and permission prefix.
const Name = "ops"

// Permission codes.
const (
	PermJobView    = "ops:job:view"
	PermJobCreate  = "ops:job:create"
	PermJobUpdate  = "ops:job:update"
	PermJobDelete  = "ops:job:delete"
	PermJobExecute = "ops:job:execute"
	PermRunView    = "ops:run:view"
	PermMonitor    = "ops:monitor:view"
)

var permissionDefs = []perm.Def{
	{Code: PermJobView, Name: "查看定时任务", Menu: "ops.jobs"},
	{Code: PermJobCreate, Name: "新增定时任务", Menu: "ops.jobs"},
	{Code: PermJobUpdate, Name: "编辑定时任务", Menu: "ops.jobs"},
	{Code: PermJobDelete, Name: "删除定时任务", Menu: "ops.jobs"},
	{Code: PermJobExecute, Name: "立即执行任务", Menu: "ops.jobs"},
	{Code: PermRunView, Name: "查看执行记录", Menu: "ops.runs"},
	{Code: PermMonitor, Name: "查看服务器监控", Menu: "ops.monitor"},
}

func page(code, title, path, component, permission, icon string, sort int) menu.Def {
	return menu.Def{
		Code: code, Parent: "ops.root", Type: menu.TypeMenu, Title: title, Path: path,
		Component: component, Permission: permission, Icon: icon, Sort: sort,
	}
}

var menuDefs = []menu.Def{
	{Code: "ops.root", Type: menu.TypeDir, Title: "运维管理", Path: "/ops", Icon: "ri:tools-line", Sort: 95},
	page("ops.jobs", "定时任务", "/ops/jobs", "ops/jobs/index", PermJobView, "ri:timer-line", 1),
	page("ops.runs", "执行记录", "/ops/runs", "ops/runs/index", PermRunView, "ri:history-line", 2),
	page("ops.monitor", "服务器监控", "/ops/monitor", "ops/monitor/index", PermMonitor, "ri:dashboard-3-line", 3),
}

// Ops error codes (7xxxx, docs/p1-p3-plan.md section 2).
const (
	// CodeHandlerMissing: the job's handler is not registered in this build.
	CodeHandlerMissing = 70001
	// CodeSchedulerStopping: the scheduler is shutting down.
	CodeSchedulerStopping = 70002
	// CodeMonitorUnavailable: system statistics could not be collected.
	CodeMonitorUnavailable = 70003
)
