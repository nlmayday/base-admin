package devtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/devtools/codegen"
	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/httpx"
)

type tableView = codegen.TableInfo

func (m *Module) requireDev() error {
	if m.deps.Env != config.EnvDev {
		return errCodegenDisabled
	}
	return nil
}

func (m *Module) codegenTables(c *gin.Context) (any, error) {
	if err := m.requireDev(); err != nil {
		return nil, err
	}
	return codegen.ListTables(c.Request.Context(), m.deps.DB, m.deps.Database.MigrationsTable, m.deps.Database.AppMigrationsTable)
}

type previewRequest struct {
	Table         string                `json:"table" binding:"required,max=63" doc:"数据表名"`
	Module        string                `json:"module" binding:"required,max=31" doc:"模块名（Go 包名），如 sales"`
	Resource      string                `json:"resource" binding:"required,max=40" doc:"资源名（kebab-case），如 order"`
	Name          string                `json:"name" binding:"required,max=20" doc:"中文名，如 订单"`
	ModuleTitle   string                `json:"moduleTitle" binding:"max=20" doc:"新模块的目录菜单标题"`
	MenuTitle     string                `json:"menuTitle,omitempty" binding:"max=20" doc:"页面菜单标题，默认「中文名 + 管理」"`
	MenuSort      *int                  `json:"menuSort,omitempty" doc:"页面菜单排序（0–9999，小的在前），默认 1"`
	Fields        []codegen.FieldOption `json:"fields" binding:"max=200,dive" doc:"字段配置（中文名、列表、表单、搜索方式）；不传的字段使用默认值"`
	KeyMode       string                `json:"keyMode,omitempty" binding:"omitempty,oneof=auto generated input" doc:"主键来源：auto 数据库自增，generated 程序生成（雪花 ID），input 新增时填写；不传按列自动判断"`
	ReadOnly      bool                  `json:"readOnly,omitempty" doc:"只读：只生成列表、详情和导出"`
	SoftDelete    *bool                 `json:"softDelete,omitempty" doc:"false 时 deleted_at 作为普通时间字段（如注销时间）：列出全部行，删除即删除行；不传或 true 时按软删除处理"`
	Children      []codegen.ChildSpec   `json:"children,omitempty" binding:"max=5" doc:"随主表一起编辑的子表（如多语言表），每个键（如语言）一行"`
	OrderBy       string                `json:"orderBy,omitempty" binding:"max=200" doc:"列表和导出的排序，如 created_at DESC, id DESC；不传为 sort 后主键倒序"`
	NoCreate      bool                  `json:"noCreate,omitempty" doc:"不生成新增（接口、权限码、按钮）"`
	NoDelete      bool                  `json:"noDelete,omitempty" doc:"不生成删除（接口、权限码、按钮），如只能下架不能删"`
	DeleteConfirm string                `json:"deleteConfirm,omitempty" binding:"max=200" doc:"删除确认文案，{列名} 换成该行的值，如 删除分类「{code}」？"`
}

type generateRequest struct {
	previewRequest
	// Force overwrites files that already exist.
	Force bool `json:"force" doc:"覆盖已存在的文件"`
}

type generateView struct {
	Files []string `json:"files"`
	// NewModule: a module was created (else a resource was added to one).
	NewModule bool `json:"newModule"`
	// Wired: the new module was registered in modules.go, or the new
	// resource in its module's module.go.
	Wired bool     `json:"wired"`
	Notes []string `json:"notes"`
}

// wire registers a new module in modules.go, or a new resource in its
// existing module's module.go, and explains any manual step left.
func wire(root string, spec codegen.Spec, res codegen.Result, notes []string) (bool, []string) {
	if spec.NewModule {
		wired, err := codegen.Wire(root, spec.Module)
		if err != nil {
			notes = append(notes, "模块未能自动接入 modules.go（"+err.Error()+"），请按预览中的「接入步骤」手动添加")
		}
		return wired, notes
	}
	wired, err := codegen.WireResource(root, spec)
	if err != nil {
		notes = append(notes, "新资源未能自动注册（"+err.Error()+"）："+res.ManualWiring)
	}
	return wired, notes
}

// lastGeneration is a generation result with its request, kept so the page
// can show it after a dev-server reload.
type lastGeneration struct {
	Request generateRequest `json:"request"`
	Result  generateView    `json:"result"`
	At      time.Time       `json:"at"`
}

// lastGenerationTTL bounds how long a result is offered after the fact.
const lastGenerationTTL = 5 * time.Minute

// codegenLast returns the caller's latest generation result, or null.
func (m *Module) codegenLast(c *gin.Context) (any, error) {
	if err := m.requireDev(); err != nil {
		return nil, err
	}
	uid := callerID(c)
	m.lastMu.Lock()
	defer m.lastMu.Unlock()
	last, ok := m.last[uid]
	if !ok || time.Since(last.At) > lastGenerationTTL {
		return nil, nil
	}
	return last, nil
}

// repeatedGeneration returns the caller's previous result when the same
// request (without force) is sent again shortly after and its files are
// still there, e.g. a retry after a lost response.
func (m *Module) repeatedGeneration(c *gin.Context, root string, req generateRequest) (generateView, bool) {
	if req.Force {
		return generateView{}, false
	}
	m.lastMu.Lock()
	last, ok := m.last[callerID(c)]
	m.lastMu.Unlock()
	if !ok || time.Since(last.At) > lastGenerationTTL || !reflect.DeepEqual(last.Request, req) {
		return generateView{}, false
	}
	files := make([]codegen.File, len(last.Result.Files))
	for i, p := range last.Result.Files {
		files[i] = codegen.File{Path: p}
	}
	if existing, err := codegen.Existing(root, files); err != nil || len(existing) != len(files) {
		return generateView{}, false
	}
	view := last.Result
	view.Notes = append(slices.Clone(view.Notes), "与上次请求相同（重复请求），返回上次的生成结果，未改动文件")
	return view, true
}

func (m *Module) rememberGeneration(c *gin.Context, req generateRequest, view generateView) {
	uid := callerID(c)
	m.lastMu.Lock()
	defer m.lastMu.Unlock()
	if m.last == nil {
		m.last = map[int64]lastGeneration{}
	}
	m.last[uid] = lastGeneration{Request: req, Result: view, At: time.Now()}
}

// callerID is the authenticated user, or 0 when no actor is attached.
func callerID(c *gin.Context) int64 {
	if a := actor.From(c.Request.Context()); a != nil && a.Snapshot != nil {
		return a.UserID()
	}
	return 0
}

// dictOption is a data dictionary the codegen page can bind a field to.
type dictOption struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// codegenDicts lists the data dictionaries (系统配置 → 数据字典).
func (m *Module) codegenDicts(c *gin.Context) (any, error) {
	if err := m.requireDev(); err != nil {
		return nil, err
	}
	out := []dictOption{}
	err := m.deps.DB.WithContext(c.Request.Context()).
		Raw("SELECT code, name, status = 1 AS enabled FROM sys_dict_types ORDER BY code").Scan(&out).Error
	return out, err
}

// missingDicts returns the field problems for dictionaries that do not exist.
func (m *Module) missingDicts(c *gin.Context, fields []codegen.FieldOption, children []codegen.ChildSpec) ([]codegen.Problem, error) {
	var codes []string
	for _, f := range fields {
		if code := dictCode(f); code != "" {
			codes = append(codes, code)
		}
	}
	for _, ch := range children {
		if ch.KeyDict != "" {
			codes = append(codes, ch.KeyDict)
		}
	}
	if len(codes) == 0 {
		return nil, nil
	}
	var found []string
	if err := m.deps.DB.WithContext(c.Request.Context()).
		Raw("SELECT code FROM sys_dict_types WHERE code IN ?", codes).Scan(&found).Error; err != nil {
		return nil, err
	}
	var out []codegen.Problem
	for _, f := range fields {
		if code := dictCode(f); code != "" && !slices.Contains(found, code) {
			out = append(out, codegen.Problem{Field: "fields", Message: "字段 " + f.Column + " 绑定的字典 " + code + " 不存在，请先在「数据字典」中创建"})
		}
	}
	for _, ch := range children {
		if ch.KeyDict != "" && !slices.Contains(found, ch.KeyDict) {
			out = append(out, codegen.Problem{Field: "children", Message: "子表 " + ch.Table + " 的键字典 " + ch.KeyDict + " 不存在，请先在「数据字典」中创建"})
		}
	}
	return out, nil
}

// dictCode is the dictionary a field option binds, or "" (none, or the
// comment options).
func dictCode(f codegen.FieldOption) string {
	if f.Dict == nil || *f.Dict == codegen.OptionsFromComment {
		return ""
	}
	return *f.Dict
}

type fieldsQuery struct {
	Table string `form:"table" binding:"required,max=63" doc:"数据表名"`
}

// codegenFields describes the columns of a table with their defaults, for
// the field configuration on the codegen page.
func (m *Module) codegenFields(c *gin.Context) (any, error) {
	if err := m.requireDev(); err != nil {
		return nil, err
	}
	var q fieldsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		return nil, tableError("请选择数据表")
	}
	table, err := m.loadTable(c, q.Table)
	if err != nil {
		return nil, err
	}
	return codegen.Describe(table)
}

// loadSpecTable is loadTable with the tables the spec refers to (declared
// relations, child tables).
func (m *Module) loadSpecTable(c *gin.Context, name string, spec codegen.Spec) (codegen.Table, error) {
	table, err := codegen.Load(c.Request.Context(), m.deps.DB, name, spec)
	switch {
	case errors.Is(err, codegen.ErrTableNotFound):
		return table, tableError("数据表不存在")
	case errors.Is(err, codegen.ErrSpec):
		return table, httpx.Validation([]httpx.FieldError{{Field: "fields", Message: codegen.SpecMessage(err)}})
	case err != nil:
		return table, err
	}
	if err := codegen.CheckTable(table); err != nil {
		return table, tableError(err.Error())
	}
	return table, nil
}

// loadTable introspects and checks a table, mapping problems to field errors.
func (m *Module) loadTable(c *gin.Context, name string) (codegen.Table, error) {
	table, err := codegen.Introspect(c.Request.Context(), m.deps.DB, name)
	if errors.Is(err, codegen.ErrTableNotFound) {
		return table, tableError("数据表不存在")
	}
	if err != nil {
		return table, err
	}
	if err := codegen.CheckTable(table); err != nil {
		return table, tableError(err.Error())
	}
	return table, nil
}

// prepare validates a request and generates the files in memory.
func (m *Module) prepare(c *gin.Context, req previewRequest) (codegen.Result, codegen.Spec, error) {
	spec := codegen.Spec{Module: req.Module, Resource: req.Resource, Name: req.Name, ModuleTitle: req.ModuleTitle,
		MenuTitle: req.MenuTitle, MenuSort: req.MenuSort,
		Fields: req.Fields, KeyMode: req.KeyMode, ReadOnly: req.ReadOnly, SoftDelete: req.SoftDelete, Children: req.Children,
		OrderBy: req.OrderBy, NoCreate: req.NoCreate, NoDelete: req.NoDelete, DeleteConfirm: req.DeleteConfirm}
	if problems := spec.Problems(); len(problems) > 0 {
		return codegen.Result{}, spec, problemsError(problems)
	}
	table, err := m.loadSpecTable(c, req.Table, spec)
	if err != nil {
		return codegen.Result{}, spec, err
	}
	problems := spec.FieldProblems(table)
	missing, err := m.missingDicts(c, spec.Fields, spec.Children)
	if err != nil {
		return codegen.Result{}, spec, err
	}
	if problems = append(problems, missing...); len(problems) > 0 {
		return codegen.Result{}, spec, problemsError(problems)
	}
	if root, err := m.root(); err == nil {
		spec.NewModule = !codegen.ModuleExists(root, spec.Module)
	} else {
		spec.NewModule = true
	}
	res, err := codegen.Generate(table, spec)
	return res, spec, err
}

func problemsError(problems []codegen.Problem) error {
	fields := make([]httpx.FieldError, len(problems))
	for i, p := range problems {
		fields[i] = httpx.FieldError{Field: p.Field, Message: p.Message}
	}
	return httpx.Validation(fields)
}

// codegenGenerate writes the generated files into the repository, formats
// the frontend files and registers a new module in modules.go. Development
// only; existing files are kept unless force is set.
func (m *Module) codegenGenerate(c *gin.Context) (any, error) {
	if err := m.requireDev(); err != nil {
		return nil, err
	}
	var req generateRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	root, err := m.root()
	if err != nil {
		return nil, httpx.NewError(http.StatusConflict, CodeCodegenNoRepo, "未找到项目根目录，请用命令行生成")
	}
	// One generation at a time: the existence check, the writes and the
	// edits of modules.go / module.go must not interleave.
	m.genMu.Lock()
	defer m.genMu.Unlock()
	if view, ok := m.repeatedGeneration(c, root, req); ok {
		return view, nil
	}
	res, spec, err := m.prepare(c, req.previewRequest)
	if err != nil {
		return nil, err
	}
	kept, err := codegen.Kept(root, res.Files)
	if err != nil {
		return nil, err
	}
	if err := codegen.Write(root, res.Files, req.Force); err != nil {
		if errors.Is(err, codegen.ErrExists) {
			existing, _ := codegen.Conflicts(root, res.Files)
			return nil, httpx.NewError(http.StatusConflict, CodeCodegenExists,
				"以下文件已存在，勾选「覆盖已有文件」后再生成："+strings.Join(existing, "、"))
		}
		return nil, err
	}
	view := generateView{Notes: res.Notes, NewModule: spec.NewModule}
	written := make([]codegen.File, 0, len(res.Files))
	for _, f := range res.Files {
		if slices.Contains(kept, f.Path) {
			continue
		}
		written = append(written, f)
		view.Files = append(view.Files, f.Path)
	}
	if len(kept) > 0 {
		view.Notes = append(view.Notes, "已保留扩展文件（不会覆盖）："+strings.Join(kept, "、"))
	}
	// Detached: the dev server may reload the page (and abort this request)
	// as soon as the new view files appear.
	ctx := context.WithoutCancel(c.Request.Context())
	if note := codegen.FormatWeb(ctx, root, written); note != "" {
		view.Notes = append(view.Notes, note)
	}
	if note := codegen.FormatGo(ctx, root, written); note != "" {
		view.Notes = append(view.Notes, note)
	}
	view.Wired, view.Notes = wire(root, spec, res, view.Notes)
	m.rememberGeneration(c, req, view)
	m.log.Info("codegen generate", zap.String("table", req.Table), zap.String("module", spec.Module),
		zap.String("resource", spec.Resource), zap.Bool("force", req.Force), zap.Strings("files", view.Files))
	return view, nil
}

type previewFile struct {
	Path     string `json:"path"`
	Language string `json:"language"`
	Content  string `json:"content"`
	// Exists reports a file the CLI would refuse to overwrite without -force.
	Exists bool `json:"exists"`
	// Keep: hand-written extension points, written only when missing;
	// Kept: it exists and is left as it is.
	Keep bool `json:"keep"`
	Kept bool `json:"kept"`
}

type menuView struct {
	Code      string `json:"code"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Path      string `json:"path"`
	Component string `json:"component"`
}

type previewView struct {
	Files       []previewFile    `json:"files"`
	NewModule   bool             `json:"newModule"`
	Permissions []permissionView `json:"permissions"`
	Menus       []menuView       `json:"menus"`
	Wiring      string           `json:"wiring"`
	Notes       []string         `json:"notes"`
	Command     string           `json:"command"`
}

func (m *Module) codegenPreview(c *gin.Context) (any, error) {
	if err := m.requireDev(); err != nil {
		return nil, err
	}
	var req previewRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	res, spec, err := m.prepare(c, req)
	if err != nil {
		return nil, err
	}
	root, rootErr := m.root()
	m.log.Info("codegen preview", zap.String("table", req.Table), zap.String("module", spec.Module),
		zap.String("resource", spec.Resource))
	return buildPreview(res, spec, req, root, rootErr)
}

func tableError(msg string) error {
	return httpx.Validation([]httpx.FieldError{{Field: "table", Message: msg}})
}

func buildPreview(res codegen.Result, spec codegen.Spec, req previewRequest, root string, rootErr error) (previewView, error) {
	existing := map[string]bool{}
	if rootErr == nil {
		paths, err := codegen.Existing(root, res.Files)
		if err != nil {
			return previewView{}, err
		}
		for _, p := range paths {
			existing[p] = true
		}
	}
	v := previewView{
		NewModule: spec.NewModule, Wiring: res.Wiring, Notes: res.Notes, Command: cliCommand(req),
		Files: make([]previewFile, len(res.Files)), Permissions: make([]permissionView, len(res.Permissions)),
		Menus: make([]menuView, len(res.Menus)),
	}
	if rootErr != nil {
		v.Notes = append(v.Notes, "未找到仓库根目录，无法检查文件是否已存在")
	}
	for i, f := range res.Files {
		v.Files[i] = previewFile{Path: f.Path, Language: f.Language, Content: f.Content,
			Exists: existing[f.Path] && !f.Keep && !f.Refresh, Keep: f.Keep, Kept: existing[f.Path] && f.Keep}
	}
	for i, p := range res.Permissions {
		v.Permissions[i] = permissionView{Code: p.Code, Name: p.Name}
	}
	for i, mn := range res.Menus {
		v.Menus[i] = menuView{Code: mn.Code, Type: string(mn.Type), Title: mn.Title, Path: mn.Path, Component: mn.Component}
	}
	return v, nil
}

// cliCommand is the equivalent CLI invocation; inputs are validated, so
// single quotes are safe.
func cliCommand(req previewRequest) string {
	parts := []string{
		"cd server && go run ./cmd/gen", "-table " + req.Table, "-module " + req.Module,
		"-resource " + req.Resource, "-name '" + strings.TrimSpace(req.Name) + "'",
	}
	if req.ModuleTitle != "" {
		parts = append(parts, "-module-title '"+req.ModuleTitle+"'")
	}
	if len(req.Fields) > 0 {
		// Labels cannot contain quotes (validated), so single quotes are safe.
		raw, _ := json.Marshal(req.Fields)
		parts = append(parts, "-fields '"+string(raw)+"'")
	}
	if req.MenuTitle != "" {
		parts = append(parts, "-menu-title '"+req.MenuTitle+"'")
	}
	if req.MenuSort != nil {
		parts = append(parts, fmt.Sprintf("-menu-sort %d", *req.MenuSort))
	}
	if req.KeyMode != "" {
		parts = append(parts, "-key-mode "+req.KeyMode)
	}
	if req.ReadOnly {
		parts = append(parts, "-read-only")
	}
	if req.SoftDelete != nil && !*req.SoftDelete {
		parts = append(parts, "-no-soft-delete")
	}
	if req.OrderBy != "" {
		parts = append(parts, "-order-by "+shellQuote(req.OrderBy))
	}
	if req.DeleteConfirm != "" {
		parts = append(parts, "-delete-confirm "+shellQuote(req.DeleteConfirm))
	}
	if req.NoCreate {
		parts = append(parts, "-no-create")
	}
	if req.NoDelete {
		parts = append(parts, "-no-delete")
	}
	if len(req.Children) > 0 {
		raw, _ := json.Marshal(req.Children)
		parts = append(parts, "-children "+shellQuote(string(raw)))
	}
	return strings.Join(parts, " ")
}

// shellQuote quotes s for a POSIX shell (a key filter may hold quotes).
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func repoRoot(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("working directory: %w", err)
	}
	return codegen.FindRepoRoot(wd)
}
