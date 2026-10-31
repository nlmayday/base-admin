// Package devtools serves the developer tools: the OpenAPI document and API
// doc data built from the route registry, and the code generator preview
// (dev only). See docs/p1-p3-plan.md, devtools.
package devtools

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"sync"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/core/perm"
	"base-admin/server/internal/devtools/codegen"
	"base-admin/server/internal/devtools/openapi"
	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/httpx"
)

// Deps are the module's dependencies.
type Deps struct {
	// Registry is the application's route registry; it is read lazily, after
	// every module has registered its routes.
	Registry *module.Registry
	DB       *gorm.DB
	// Env gates the code generator: it only runs in config.EnvDev.
	Env    config.Env
	Logger *zap.Logger

	// APIPrefix is stripped from paths in the OpenAPI document; default "/api/v1".
	APIPrefix string
	// SessionCookie is documented as the cookie security scheme; default "sid".
	SessionCookie string
	// Database opens the dedicated connection that applies migrations
	// created through the AI agent API.
	Database config.DatabaseConfig
	// RepoRoot is where the preview checks for existing files; by default it
	// is searched upwards from the working directory.
	RepoRoot string
}

// apiDoc is the cached OpenAPI document and its JSON encoding.
type apiDoc struct {
	built *openapi.Built
	raw   []byte
}

// Module is the devtools module.
type Module struct {
	deps Deps
	log  *zap.Logger
	doc  func() (apiDoc, error)
	root func() (string, error)
	// last holds each user's latest generation result: writing new view
	// files makes the Vite dev server reload the page, often before the
	// response arrives, so the page fetches it afterwards.
	lastMu sync.Mutex
	last   map[int64]lastGeneration
	// migrateMu serializes numbering, writing and applying migrations;
	// genMu serializes code generation.
	migrateMu sync.Mutex
	genMu     sync.Mutex
}

var _ module.Module = (*Module)(nil)

// New creates the devtools module.
func New(deps Deps) *Module {
	deps.APIPrefix = cmp.Or(deps.APIPrefix, "/api/v1")
	deps.SessionCookie = cmp.Or(deps.SessionCookie, "sid")
	m := &Module{deps: deps, log: deps.Logger}
	if m.log == nil {
		m.log = zap.NewNop()
	}
	// Routes are complete once the server starts, so the document is built
	// on first use and cached.
	m.doc = sync.OnceValues(func() (apiDoc, error) {
		built := openapi.Build(deps.Registry.Routes(), openapi.Options{
			Title:           "Base Admin API",
			Description:     "由模块路由注册表自动生成。响应统一包裹在 {code, message, data, requestId} 信封中。",
			BasePath:        deps.APIPrefix,
			SessionCookie:   deps.SessionCookie,
			TagDescriptions: moduleTitles,
		})
		raw, err := json.Marshal(built.Document)
		return apiDoc{built: built, raw: raw}, err
	})
	m.root = sync.OnceValues(func() (string, error) { return repoRoot(deps.RepoRoot) })
	return m
}

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Permissions implements module.Module.
func (m *Module) Permissions() []perm.Def { return slices.Clone(permissionDefs) }

// Menus implements module.Module.
func (m *Module) Menus() []menu.Def { return slices.Clone(menuDefs) }

// Routes implements module.Module.
func (m *Module) Routes(r *module.Router) {
	g := r.Group("/devtools")
	g.GET("/openapi.json", access.Perm(PermAPIDoc), m.openAPI).Doc(module.Doc{
		Summary:     "OpenAPI 文档",
		Description: "OpenAPI 3.0.3 文档（JSON），可导入 Postman、Apifox 或 Swagger UI。",
		Response:    openapi.Raw{ContentType: "application/json", Description: "OpenAPI 文档本身，不带信封"},
	})
	g.GET("/routes", access.Perm(PermAPIDoc), handle(m.listRoutes)).Doc(module.Doc{
		Summary: "接口文档数据", Description: "按模块列出全部接口及其参数、请求与响应结构、示例。", Response: routesView{},
	})
	g.GET("/codegen/tables", access.Perm(PermCodegen), handle(m.codegenTables)).Doc(module.Doc{
		Summary: "代码生成：数据表", Description: "仅开发环境可用。", Response: []tableView{},
	})
	g.GET("/codegen/fields", access.Perm(PermCodegen), handle(m.codegenFields)).Doc(module.Doc{
		Summary: "代码生成：字段", Description: "仅开发环境可用；返回数据表各字段的类型、默认中文名、列表/表单/搜索的默认配置。",
		Query: fieldsQuery{}, Response: []codegen.FieldInfo{},
	})
	g.POST("/codegen/preview", access.Perm(PermCodegen), handle(m.codegenPreview)).Doc(module.Doc{
		Summary: "代码生成：预览", Description: "仅开发环境可用；只返回生成结果，不写入任何文件。",
		Body: previewRequest{}, Response: previewView{},
	})
	g.GET("/codegen/dicts", access.Perm(PermCodegen), handle(m.codegenDicts)).Doc(module.Doc{
		Summary: "代码生成：可绑定的字典", Description: "仅开发环境可用；列出「数据字典」中的字典类型，供字段绑定。",
		Response: []dictOption{},
	})
	g.GET("/codegen/last", access.Perm(PermCodegen), handle(m.codegenLast)).Doc(module.Doc{
		Summary: "代码生成：最近一次结果", Description: "仅开发环境可用；返回当前用户最近一次「生成到项目」的结果（页面被开发服务器刷新后用于恢复）。",
		Response: lastGeneration{},
	})
	g.POST("/codegen/generate", access.Perm(PermCodegen), handle(m.codegenGenerate)).Doc(module.Doc{
		Summary: "代码生成：生成到项目", Description: "仅开发环境可用；写入生成的文件、格式化前端代码，新模块自动接入 modules.go。已存在的文件默认不覆盖。",
		Body: generateRequest{}, Response: generateView{},
	})
	g.GET("/agent-keys", access.Perm(PermAgentKeys), handle(m.listAgentKeys)).Doc(module.Doc{
		Summary: "AI 接口：密钥列表", Response: agentKeyList{},
	})
	g.POST("/agent-keys", access.Perm(PermAgentKeys), handle(m.createAgentKey)).Doc(module.Doc{
		Summary: "AI 接口：创建密钥", Description: "完整密钥只在本次响应中返回；密钥以创建人的身份调用 AI 接口。",
		Body: createAgentKeyRequest{}, Response: createdAgentKey{},
	})
	g.DELETE("/agent-keys/:id", access.Perm(PermAgentKeys), handle(m.revokeAgentKey)).Doc(module.Doc{
		Summary: "AI 接口：撤销密钥",
	})
	m.agentRoutes(r)
}

// Seed implements module.Module; devtools has no data.
func (m *Module) Seed(context.Context, *gorm.DB) error { return nil }

// handle adapts a function returning (data, error) to a Gin handler.
func handle(fn func(c *gin.Context) (any, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := fn(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, data)
	}
}
