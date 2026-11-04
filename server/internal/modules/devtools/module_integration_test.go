//go:build integration

package devtools

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/core/perm"
	"base-admin/server/internal/devtools/codegen"
	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
	"base-admin/server/internal/platform/testinfra"
	"base-admin/server/internal/platform/testinfra/migrated"
)

// headerGuard admits public routes; other levels need the X-Test-User
// header, and permission levels one of the codes in X-Test-Perms.
type headerGuard struct{}

func (headerGuard) Require(level access.Level) gin.HandlerFunc {
	return func(c *gin.Context) {
		if level.Kind() == access.KindPublic {
			return
		}
		if c.GetHeader("X-Test-User") == "" {
			httpx.Fail(c, httpx.NewError(http.StatusUnauthorized, 20001, "未登录"))
			return
		}
		held := strings.Split(c.GetHeader("X-Test-Perms"), ",")
		if level.Kind() == access.KindPermission && !level.Allows(func(code string) bool {
			for _, h := range held {
				if h == code {
					return true
				}
			}
			return false
		}) {
			httpx.Fail(c, httpx.NewError(http.StatusForbidden, 20002, "无权限"))
			return
		}
		uid, _ := strconv.ParseInt(c.GetHeader("X-Test-User"), 10, 64)
		c.Request = c.Request.WithContext(actor.With(c.Request.Context(), &actor.Actor{Snapshot: &actor.Snapshot{UserID: uid}}))
	}
}

type widgetView struct {
	ID       ids.ID   `json:"id"`
	Name     string   `json:"name" doc:"名称"`
	ParentID *ids.ID  `json:"parentId"`
	Tags     []string `json:"tags"`
}

type widgetRequest struct {
	Name   string `json:"name" binding:"required,max=32"`
	Status int16  `json:"status" binding:"omitempty,oneof=1 2"`
}

type widgetFilter struct {
	Keyword string `form:"keyword" binding:"max=64"`
}

// fakeModule declares a few documented routes.
type fakeModule struct{}

func (fakeModule) Name() string { return "demo" }
func (fakeModule) Permissions() []perm.Def {
	return []perm.Def{
		{Code: "demo:widget:view", Name: "查看部件", Menu: "demo.widget"},
		{Code: "demo:widget:create", Name: "新增部件", Menu: "demo.widget"},
	}
}
func (fakeModule) Menus() []menu.Def {
	return []menu.Def{
		{Code: "demo.root", Type: menu.TypeDir, Title: "演示", Path: "/demo"},
		{Code: "demo.widget", Parent: "demo.root", Type: menu.TypeMenu, Title: "部件", Path: "/demo/widget",
			Component: "demo/widget/index", Permission: "demo:widget:view"},
	}
}
func (fakeModule) Routes(r *module.Router) {
	ok := func(c *gin.Context) { httpx.OK(c, nil) }
	r.GET("/widgets", access.Perm("demo:widget:view"), ok).Doc(module.Doc{Summary: "部件列表", Query: widgetFilter{}, Response: httpx.Page[widgetView]{}})
	r.POST("/widgets", access.Perm("demo:widget:create"), ok).Doc(module.Doc{Summary: "新增部件", Body: widgetRequest{}, Response: widgetView{}})
	r.GET("/widgets/:id", access.AnyPerm("demo:widget:view", "demo:widget:create"), ok).Doc(module.Doc{Summary: "部件详情", Response: widgetView{}})
	r.GET("/widgets/export", access.Perm("demo:widget:view"), ok).Doc(module.Doc{Summary: "导出部件", Sensitive: true})
	r.POST("/ping", access.Public(), ok).Doc(module.Doc{Summary: "探活"})
	r.GET("/whoami", access.Authenticated(), ok).Doc(module.Doc{Summary: "当前用户", Response: []widgetView{}})
}
func (fakeModule) Seed(context.Context, *gorm.DB) error { return nil }

type harness struct {
	t      *testing.T
	db     *gorm.DB
	engine *gin.Engine
}

func newHarness(t *testing.T, env config.Env, root string) *harness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, httpx.RegisterJSONFieldNames())
	cfg := migrated.Database(t)
	sqlDB, err := database.OpenSQL(context.Background(), cfg, "devtools-test")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := database.NewGorm(sqlDB, cfg, zap.NewNop())
	require.NoError(t, err)

	reg := module.NewRegistry()
	dev := New(Deps{Registry: reg, DB: db, Env: env, Logger: zap.NewNop(), RepoRoot: root, Database: cfg})
	mods := []module.Module{fakeModule{}, dev}
	engine := gin.New()
	api := engine.Group("/api/v1")
	for _, m := range mods {
		m.Routes(module.NewRouter(api, m.Name(), headerGuard{}, reg))
	}
	require.NoError(t, module.Validate(mods, reg, engine.Routes(), "/api/v1").Err())
	require.NoError(t, module.Sync(context.Background(), db, mods))
	return &harness{t: t, db: db, engine: engine}
}

type response struct {
	status int
	header http.Header
	body   []byte
	env    struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
}

func (h *harness) do(method, path, perms string, body any) response {
	h.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(h.t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User", "1")
	req.Header.Set("X-Test-Perms", perms)
	rec := httptest.NewRecorder()
	h.engine.ServeHTTP(rec, req)
	r := response{status: rec.Code, header: rec.Header(), body: rec.Body.Bytes()}
	_ = json.Unmarshal(r.body, &r.env)
	return r
}

func TestOpenAPIDocument(t *testing.T) {
	h := newHarness(t, config.EnvProd, "")
	r := h.do(http.MethodGet, "/api/v1/devtools/openapi.json", PermAPIDoc, nil)
	require.Equal(t, http.StatusOK, r.status)
	assert.True(t, strings.HasPrefix(r.header.Get("Content-Type"), "application/json"))

	var doc struct {
		OpenAPI string                               `json:"openapi"`
		Servers []struct{ URL string }               `json:"servers"`
		Paths   map[string]map[string]map[string]any `json:"paths"`
		Tags    []struct{ Name, Description string } `json:"tags"`
	}
	require.NoError(t, json.Unmarshal(r.body, &doc), "raw document, not an envelope")
	assert.Equal(t, "3.0.3", doc.OpenAPI)
	assert.Equal(t, "/api/v1", doc.Servers[0].URL)
	for _, p := range []string{"/widgets", "/widgets/{id}", "/widgets/export", "/ping", "/whoami",
		"/devtools/openapi.json", "/devtools/routes", "/devtools/codegen/tables", "/devtools/codegen/preview"} {
		assert.Contains(t, doc.Paths, p)
	}
	get := doc.Paths["/widgets/{id}"]["get"]
	assert.Equal(t, []any{"demo:widget:view", "demo:widget:create"}, get["x-permissions"])
	assert.Equal(t, "permission", get["x-access"])
	assert.Equal(t, []any{}, doc.Paths["/ping"]["post"]["security"])
	assert.Contains(t, doc.Tags, struct{ Name, Description string }{"devtools", "开发工具"})

	again := h.do(http.MethodGet, "/api/v1/devtools/openapi.json", PermAPIDoc, nil)
	assert.Equal(t, r.body, again.body, "cached and deterministic")
}

func TestRouteListForTheUI(t *testing.T) {
	h := newHarness(t, config.EnvProd, "")
	r := h.do(http.MethodGet, "/api/v1/devtools/routes", PermAPIDoc, nil)
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	var data routesView
	require.NoError(t, json.Unmarshal(r.env.Data, &data))

	assert.Equal(t, []moduleView{{Name: "demo", Title: "demo", Count: 6}, {Name: "devtools", Title: "开发工具", Count: 18}}, data.Modules)
	byKey := map[string]routeView{}
	for _, rv := range data.Routes {
		byKey[rv.Method+" "+rv.Path] = rv
	}
	list := byKey["GET /api/v1/widgets"]
	assert.Equal(t, "部件列表", list.Summary)
	assert.Equal(t, []permissionView{{Code: "demo:widget:view", Name: "查看部件"}}, list.Permissions, "names come from sys_permissions")
	var params []string
	for _, p := range list.Parameters {
		params = append(params, p.Name)
	}
	assert.Equal(t, []string{"page", "pageSize", "keyword"}, params)
	assert.Equal(t, []string{"长度 ≤ 64"}, list.Parameters[2].Constraints)
	assert.False(t, list.CSRF)
	require.NotEmpty(t, list.Response)
	assert.Equal(t, "list", list.Response[0].Name)

	create := byKey["POST /api/v1/widgets"]
	assert.True(t, create.CSRF)
	require.Len(t, create.RequestBody, 2)
	assert.Equal(t, "name", create.RequestBody[0].Name)
	assert.True(t, create.RequestBody[0].Required)
	assert.JSONEq(t, `{"name":"string","status":1}`, mustJSON(t, create.RequestExample))
	assert.JSONEq(t, `{"code":0,"message":"success","data":{"id":"1","name":"string","parentId":"1","tags":["string"]},
		"requestId":"req_0123456789abcdef"}`, mustJSON(t, create.ResponseExample))

	get := byKey["GET /api/v1/widgets/:id"]
	assert.Equal(t, "id", get.Parameters[0].Name)
	assert.Equal(t, "path", get.Parameters[0].In)
	assert.Equal(t, "string(id)", get.Parameters[0].Type)
	assert.Len(t, get.Permissions, 2)

	export := byKey["GET /api/v1/widgets/export"]
	assert.Equal(t, "text/csv", export.ResponseType)
	assert.True(t, export.Sensitive)
	assert.Nil(t, export.ResponseExample)

	assert.Equal(t, "public", byKey["POST /api/v1/ping"].Access)
	assert.Equal(t, "authenticated", byKey["GET /api/v1/whoami"].Access)
	assert.Equal(t, "[]", byKey["GET /api/v1/whoami"].Response[0].Name)
	assert.Equal(t, "application/json", byKey["GET /api/v1/devtools/openapi.json"].ResponseType)
}

func TestPermissionLevels(t *testing.T) {
	h := newHarness(t, config.EnvDev, "")
	for _, tc := range []struct {
		method, path, perms string
		status              int
	}{
		{http.MethodGet, "/api/v1/devtools/routes", "", http.StatusForbidden},
		{http.MethodGet, "/api/v1/devtools/openapi.json", PermCodegen, http.StatusForbidden},
		{http.MethodGet, "/api/v1/devtools/routes", PermAPIDoc, http.StatusOK},
		{http.MethodGet, "/api/v1/devtools/codegen/tables", PermAPIDoc, http.StatusForbidden},
		{http.MethodGet, "/api/v1/devtools/codegen/tables", PermCodegen, http.StatusOK},
	} {
		r := h.do(tc.method, tc.path, tc.perms, nil)
		assert.Equal(t, tc.status, r.status, "%s %s with %q", tc.method, tc.path, tc.perms)
	}
}

func TestCodegenIsDevOnly(t *testing.T) {
	for _, env := range []config.Env{config.EnvProd, config.EnvTest} {
		h := newHarness(t, env, "")
		for _, call := range []struct{ method, path string }{
			{http.MethodGet, "/api/v1/devtools/codegen/tables"},
			{http.MethodPost, "/api/v1/devtools/codegen/preview"},
		} {
			r := h.do(call.method, call.path, PermCodegen, map[string]string{"table": "x"})
			assert.Equal(t, http.StatusForbidden, r.status, env)
			assert.Equal(t, CodeCodegenDisabled, r.env.Code)
			assert.Contains(t, r.env.Message, "开发环境")
		}
	}
}

const gadgetDDL = `CREATE TABLE biz_gadgets (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	name varchar(64) NOT NULL,
	dept_id bigint,
	status smallint NOT NULL DEFAULT 1,
	version integer NOT NULL DEFAULT 0,
	created_at timestamptz NOT NULL DEFAULT now(),
	created_by bigint,
	deleted_at timestamptz
);
COMMENT ON TABLE biz_gadgets IS '小工具';
CREATE TABLE biz_bad (id integer PRIMARY KEY);`

// mysqlGadgetDDL is gadgetDDL for MySQL.
const mysqlGadgetDDL = `CREATE TABLE biz_gadgets (
	id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
	name varchar(64) NOT NULL,
	dept_id bigint,
	status smallint NOT NULL DEFAULT 1,
	version integer NOT NULL DEFAULT 0,
	created_at datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
	created_by bigint,
	deleted_at datetime(6)
) COMMENT='小工具';
CREATE TABLE biz_bad (id integer PRIMARY KEY);`

// createGadgets creates the gadget tables of the tests.
func (h *harness) createGadgets() {
	h.t.Helper()
	ddl := gadgetDDL
	if testinfra.IsMySQL(h.t) {
		ddl = mysqlGadgetDDL
	}
	for _, stmt := range strings.Split(ddl, ";\n") {
		require.NoError(h.t, h.db.Exec(stmt).Error, stmt)
	}
}

func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range []string{"server/go.mod", "web/package.json", "web/src/api/shop/gadget.ts"} {
		p := filepath.Join(root, filepath.FromSlash(f))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o600))
	}
	return root
}

func TestCodegenTablesAndPreview(t *testing.T) {
	root := fakeRepo(t)
	h := newHarness(t, config.EnvDev, root)
	h.createGadgets()

	r := h.do(http.MethodGet, "/api/v1/devtools/codegen/tables", PermCodegen, nil)
	require.Equal(t, http.StatusOK, r.status)
	var tables []tableView
	require.NoError(t, json.Unmarshal(r.env.Data, &tables))
	// Migrations of downstream projects add their own business tables, so
	// look the test tables up by name.
	byName := map[string]tableView{}
	for _, tv := range tables {
		assert.False(t, strings.HasPrefix(tv.Name, "sys_"), "sys_* tables are hidden: %s", tv.Name)
		assert.NotEqual(t, "schema_migrations", tv.Name)
		byName[tv.Name] = tv
	}
	bad := byName["biz_bad"]
	assert.Equal(t, tableView{Name: "biz_bad", Supported: false, Reason: bad.Reason, Columns: 1}, bad)
	assert.NotEmpty(t, bad.Reason)
	assert.Equal(t, tableView{Name: "biz_gadgets", Comment: "小工具", Supported: true, Columns: 8}, byName["biz_gadgets"])

	req := map[string]string{"table": "biz_gadgets", "module": "shop", "resource": "gadget", "name": "小工具", "moduleTitle": "商店"}
	r = h.do(http.MethodPost, "/api/v1/devtools/codegen/preview", PermCodegen, req)
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	var preview previewView
	require.NoError(t, json.Unmarshal(r.env.Data, &preview))
	assert.True(t, preview.NewModule)
	require.Len(t, preview.Files, 7)
	exists := map[string]bool{}
	for _, f := range preview.Files {
		exists[f.Path] = f.Exists
		assert.NotEmpty(t, f.Content)
	}
	assert.True(t, exists["web/src/api/shop/gadget.ts"], "existing files are flagged")
	assert.False(t, exists["server/internal/modules/shop/gadget.go"])
	assert.Equal(t, "shop:gadget:view", preview.Permissions[0].Code)
	assert.Equal(t, "shop.root", preview.Menus[0].Code)
	assert.Contains(t, preview.Command, "-module-title '商店'")
	assert.Contains(t, preview.Wiring, "shop.New(shop.Deps{DB: d.db, Dict: systemModule})")
	_, err := os.Stat(filepath.Join(root, "server", "internal"))
	assert.True(t, os.IsNotExist(err), "preview writes nothing")
}

func TestCodegenPreviewValidation(t *testing.T) {
	h := newHarness(t, config.EnvDev, fakeRepo(t))
	h.createGadgets()
	cases := []struct {
		body  map[string]string
		field string
	}{
		{map[string]string{"module": "shop", "resource": "gadget", "name": "x"}, "table"},
		{map[string]string{"table": "biz_gadgets", "module": "Shop", "resource": "gadget", "name": "x"}, "module"},
		{map[string]string{"table": "biz_gadgets", "module": "iam", "resource": "gadget", "name": "x"}, "module"},
		{map[string]string{"table": "biz_gadgets", "module": "shop", "resource": "Gadget!", "name": "x"}, "resource"},
		{map[string]string{"table": "biz_gadgets", "module": "shop", "resource": "gadget", "name": "<b>"}, "name"},
		{map[string]string{"table": "nope", "module": "shop", "resource": "gadget", "name": "x"}, "table"},
		{map[string]string{"table": "biz_bad", "module": "shop", "resource": "gadget", "name": "x"}, "table"},
	}
	for _, tc := range cases {
		r := h.do(http.MethodPost, "/api/v1/devtools/codegen/preview", PermCodegen, tc.body)
		require.Equal(t, http.StatusUnprocessableEntity, r.status, "%v: %s", tc.body, r.body)
		assert.Contains(t, string(r.env.Data), `"field":"`+tc.field+`"`, tc.body)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func TestCodegenFieldsAndGenerate(t *testing.T) {
	root := fakeRepo(t)
	modules := filepath.Join(root, "server", "internal", "app", "modules.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(modules), 0o750))
	require.NoError(t, os.WriteFile(modules, []byte(`package app

import (
	"base-admin/server/internal/modules/system"
)

func build() {
	mods := []module.Module{
		// codegen:modules
	}
	_ = mods
}
`), 0o600))
	h := newHarness(t, config.EnvDev, root)
	h.createGadgets()

	r := h.do(http.MethodGet, "/api/v1/devtools/codegen/fields?table=biz_gadgets", PermCodegen, nil)
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	var fields []codegen.FieldInfo
	require.NoError(t, json.Unmarshal(r.env.Data, &fields))
	by := map[string]codegen.FieldInfo{}
	for _, f := range fields {
		by[f.Column] = f
	}
	assert.Equal(t, codegen.SearchLike, by["name"].Search)
	assert.Equal(t, codegen.SearchEq, by["status"].Search)
	assert.True(t, by["version"].FormLocked)
	assert.Equal(t, http.StatusUnprocessableEntity,
		h.do(http.MethodGet, "/api/v1/devtools/codegen/fields?table=biz_nope", PermCodegen, nil).status)

	req := map[string]any{"table": "biz_gadgets", "module": "shop", "resource": "widget", "name": "小部件", "moduleTitle": "商店",
		"fields": []map[string]any{{"column": "name", "label": "部件名称", "search": "eq"}, {"column": "created_at", "search": "range"}}}
	bad := map[string]any{"table": "biz_gadgets", "module": "shop", "resource": "widget", "name": "小部件",
		"fields": []map[string]any{{"column": "status", "search": "like"}}}
	r = h.do(http.MethodPost, "/api/v1/devtools/codegen/preview", PermCodegen, bad)
	assert.Equal(t, http.StatusUnprocessableEntity, r.status)
	assert.Contains(t, string(r.body), "不支持「模糊」搜索")

	// Dictionaries: listed for the page; a missing one is refused.
	require.NoError(t, h.db.Exec(`INSERT INTO sys_dict_types (code, name) VALUES ('gadget_kind', '部件类型')`).Error)
	r = h.do(http.MethodGet, "/api/v1/devtools/codegen/dicts", PermCodegen, nil)
	require.Equal(t, http.StatusOK, r.status)
	assert.Contains(t, string(r.env.Data), `{"code":"gadget_kind","name":"部件类型","enabled":true}`)
	withDict := func(code string) map[string]any {
		return map[string]any{"table": "biz_gadgets", "module": "shop", "resource": "widget", "name": "小部件",
			"fields": []map[string]any{{"column": "name", "dict": code}}}
	}
	r = h.do(http.MethodPost, "/api/v1/devtools/codegen/preview", PermCodegen, withDict("no_such_dict"))
	assert.Equal(t, http.StatusUnprocessableEntity, r.status)
	assert.Contains(t, string(r.body), "字典 no_such_dict 不存在")
	r = h.do(http.MethodPost, "/api/v1/devtools/codegen/preview", PermCodegen, withDict("gadget_kind"))
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	assert.Contains(t, string(r.body), `checkDict(ctx, m.dict, \"gadget_kind\"`)

	r = h.do(http.MethodPost, "/api/v1/devtools/codegen/preview", PermCodegen, req)
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	var preview previewView
	require.NoError(t, json.Unmarshal(r.env.Data, &preview))
	assert.Contains(t, preview.Command, `-fields '[{"column":"name","label":"部件名称","search":"eq"}`)

	r = h.do(http.MethodPost, "/api/v1/devtools/codegen/generate", PermCodegen, req)
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	var gen generateView
	require.NoError(t, json.Unmarshal(r.env.Data, &gen))
	assert.True(t, gen.Wired, "a new module is registered in modules.go")

	// The page reloads when Vite sees the new views; it fetches the result.
	r = h.do(http.MethodGet, "/api/v1/devtools/codegen/last", PermCodegen, nil)
	require.Equal(t, http.StatusOK, r.status)
	var last lastGeneration
	require.NoError(t, json.Unmarshal(r.env.Data, &last))
	assert.Equal(t, "widget", last.Request.Resource)
	assert.True(t, last.Result.Wired)
	assert.Contains(t, gen.Files, "server/internal/modules/shop/widget.go")
	src, err := os.ReadFile(filepath.Join(root, "server", "internal", "modules", "shop", "widget.go"))
	require.NoError(t, err)
	assert.Contains(t, string(src), `q = q.Where("t.name = ?", v)`)
	assert.Contains(t, string(src), `doc:"部件名称"`)
	wired, err := os.ReadFile(modules)
	require.NoError(t, err)
	assert.Contains(t, string(wired), "shop.New(shop.Deps{DB: d.db, Dict: systemModule}),")

	// The same request again (a retry) returns the previous result; a
	// different one does not overwrite the files.
	r = h.do(http.MethodPost, "/api/v1/devtools/codegen/generate", PermCodegen, req)
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	var repeated generateView
	require.NoError(t, json.Unmarshal(r.env.Data, &repeated))
	assert.Equal(t, gen.Files, repeated.Files)
	assert.Contains(t, strings.Join(repeated.Notes, "\n"), "重复请求")
	req["name"] = "部件"
	r = h.do(http.MethodPost, "/api/v1/devtools/codegen/generate", PermCodegen, req)
	assert.Equal(t, http.StatusConflict, r.status, "existing files are kept")
	assert.Equal(t, CodeCodegenExists, r.env.Code)
	req["force"] = true
	r = h.do(http.MethodPost, "/api/v1/devtools/codegen/generate", PermCodegen, req)
	assert.Equal(t, http.StatusOK, r.status, "force overwrites")

	prod := newHarness(t, config.EnvProd, root)
	assert.Equal(t, http.StatusForbidden, prod.do(http.MethodPost, "/api/v1/devtools/codegen/generate", PermCodegen, req).status)
}
