package module

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/perm"
)

func init() { gin.SetMode(gin.TestMode) }

type fakeModule struct {
	name   string
	perms  []perm.Def
	menus  []menu.Def
	routes func(r *Router)
}

func (f fakeModule) Name() string            { return f.name }
func (f fakeModule) Permissions() []perm.Def { return f.perms }
func (f fakeModule) Menus() []menu.Def       { return f.menus }
func (f fakeModule) Routes(r *Router) {
	if f.routes != nil {
		f.routes(r)
	}
}
func (f fakeModule) Seed(context.Context, *gorm.DB) error { return nil }

// recordingGuard denies everything except public routes and records levels.
type recordingGuard struct{ seen []access.Level }

func (g *recordingGuard) Require(level access.Level) gin.HandlerFunc {
	g.seen = append(g.seen, level)
	return func(c *gin.Context) {
		if level.Kind() != access.KindPublic {
			c.AbortWithStatus(http.StatusForbidden)
		}
	}
}

func okHandler(c *gin.Context) { c.Status(http.StatusOK) }

func validModule() fakeModule {
	return fakeModule{
		name: "iam",
		perms: []perm.Def{
			{Code: "iam:user:view", Name: "查看用户", Menu: "iam.user"},
			{Code: "iam:user:create", Name: "新增用户", Menu: "iam.user"},
		},
		menus: []menu.Def{
			{Code: "iam.system", Type: menu.TypeDir, Title: "系统管理", Path: "/system"},
			{Code: "iam.user", Parent: "iam.system", Type: menu.TypeMenu, Title: "用户管理",
				Path: "/system/user", Component: "system/user/index", Permission: "iam:user:view"},
		},
		routes: func(r *Router) {
			g := r.Group("/users")
			g.GET("", access.Perm("iam:user:view"), okHandler)
			g.POST("", access.Perm("iam:user:create"), okHandler)
		},
	}
}

type harness struct {
	engine *gin.Engine
	reg    *Registry
	guard  *recordingGuard
	api    *gin.RouterGroup
}

func newHarness() harness {
	engine := gin.New()
	return harness{engine: engine, reg: NewRegistry(), guard: &recordingGuard{}, api: engine.Group("/api/v1")}
}

func (h harness) mount(mods ...Module) {
	for _, m := range mods {
		m.Routes(NewRouter(h.api, m.Name(), h.guard, h.reg))
	}
}

func (h harness) validate(mods ...Module) Report {
	h.mount(mods...)
	return Validate(mods, h.reg, h.engine.Routes(), "/api/v1")
}

func TestValidModulePasses(t *testing.T) {
	rep := newHarness().validate(validModule())
	assert.Empty(t, rep.Errors)
	assert.NoError(t, rep.Err())
}

func TestRouterRecordsAndGuardsRoutes(t *testing.T) {
	h := newHarness()
	mod := validModule()
	mod.routes = func(r *Router) {
		r.GET("/ping", access.Public(), okHandler)
		g := r.Group("/users")
		g.GET("/", access.Perm("iam:user:view"), okHandler)
		g.PUT("/:id", access.Authenticated(), okHandler)
		g.PATCH("/:id", access.Authenticated(), okHandler)
		g.DELETE("/:id", access.Authenticated(), okHandler)
		g.POST("", access.Authenticated(), okHandler)
	}
	h.mount(mod)

	var got []string
	for _, r := range h.reg.Routes() {
		got = append(got, r.Method+" "+r.Path+" "+r.Access.String()+" "+r.Module)
	}
	assert.Equal(t, []string{
		"GET /api/v1/ping public iam",
		"GET /api/v1/users/ perm(iam:user:view) iam",
		"PUT /api/v1/users/:id authenticated iam",
		"PATCH /api/v1/users/:id authenticated iam",
		"DELETE /api/v1/users/:id authenticated iam",
		"POST /api/v1/users authenticated iam",
	}, got)
	assert.Len(t, h.guard.seen, 6, "every route goes through the guard")

	rec := httptest.NewRecorder()
	h.engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	rec = httptest.NewRecorder()
	h.engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users/", nil))
	assert.Equal(t, http.StatusForbidden, rec.Code, "guard runs before the handler")
}

func TestValidationRules(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(m *fakeModule)
		wantErr string
	}{
		{"bad code format", func(m *fakeModule) { m.perms = append(m.perms, perm.Def{Code: "iam:user"}) }, "is not module:resource:action"},
		{"wrong prefix", func(m *fakeModule) {
			m.perms = append(m.perms, perm.Def{Code: "sys:user:delete", Requires: []string{}})
		}, `must start with "iam:"`},
		{"duplicate code", func(m *fakeModule) { m.perms = append(m.perms, perm.Def{Code: "iam:user:view"}) }, "declared by both"},
		{"route uses undeclared code", func(m *fakeModule) {
			prev := m.routes
			m.routes = func(r *Router) { prev(r); r.DELETE("/users/:id", access.Perm("iam:user:delete"), okHandler) }
		}, "references undeclared permission iam:user:delete"},
		{"route without level", func(m *fakeModule) {
			prev := m.routes
			m.routes = func(r *Router) { prev(r); r.GET("/x", access.Level{}, okHandler) }
		}, "has no valid access level"},
		{"menu references undeclared code", func(m *fakeModule) { m.menus[1].Permission = "iam:ghost:view" }, "permission iam:ghost:view is not declared"},
		{"page menu without view code", func(m *fakeModule) { m.menus[1].Permission = "iam:user:create" }, "must reference a view permission"},
		{"directory with permission", func(m *fakeModule) { m.menus[0].Permission = "iam:user:view" }, "directories must not reference a permission"},
		{"duplicate menu", func(m *fakeModule) { m.menus = append(m.menus, m.menus[0]) }, "menu iam.system is declared twice"},
		{"missing parent", func(m *fakeModule) { m.menus[1].Parent = "iam.nope" }, "parent iam.nope does not exist"},
		{"menu prefix", func(m *fakeModule) { m.menus[0].Code = "sys.root"; m.menus[1].Parent = "sys.root" }, `must start with "iam."`},
		{"menu cycle", func(m *fakeModule) { m.menus[0].Parent = "iam.user" }, "parent chain contains a cycle"},
		{"bad menu type", func(m *fakeModule) { m.menus[0].Type = "page" }, "invalid type"},
		{"menu without title", func(m *fakeModule) { m.menus[0].Title = "" }, "title is required"},
		{"page without component", func(m *fakeModule) { m.menus[1].Component = "" }, "path and component are required"},
		{"link without url", func(m *fakeModule) {
			m.menus = append(m.menus, menu.Def{Code: "iam.docs", Type: menu.TypeLink, Title: "Docs"})
		}, "links require an external URL"},
		{"requires undeclared", func(m *fakeModule) { m.perms[1].Requires = []string{"iam:user:ghost"} }, "requires undeclared permission iam:user:ghost"},
		{"default requires missing view", func(m *fakeModule) {
			m.perms = append(m.perms, perm.Def{Code: "iam:audit:export"})
		}, "requires undeclared permission iam:audit:view"},
		{"requires cycle", func(m *fakeModule) {
			m.perms[0].Requires = []string{"iam:user:create"}
		}, "contain a cycle"},
		{"permission groups under missing menu", func(m *fakeModule) { m.perms[1].Menu = "iam.ghost" }, "menu iam.ghost does not exist"},
		{"duplicate route", func(m *fakeModule) {
			prev := m.routes
			m.routes = func(r *Router) {
				prev(r)
				defer func() { _ = recover() }()
				r.GET("/users", access.Perm("iam:user:view"), okHandler)
			}
		}, "registered twice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mod := validModule()
			mod.menus = append([]menu.Def(nil), mod.menus...)
			mod.perms = append([]perm.Def(nil), mod.perms...)
			tc.mutate(&mod)
			rep := newHarness().validate(mod)
			require.Error(t, rep.Err())
			assert.Contains(t, strings.Join(rep.Errors, "\n"), tc.wantErr)
		})
	}
}

func TestInvalidModuleNames(t *testing.T) {
	a, b := validModule(), validModule()
	b.routes = nil
	rep := newHarness().validate(a, b)
	assert.Contains(t, strings.Join(rep.Errors, "\n"), `module "iam" is registered twice`)

	bad := fakeModule{name: "Bad_Name"}
	rep = newHarness().validate(bad)
	assert.Contains(t, strings.Join(rep.Errors, "\n"), "is invalid")
}

func TestRoutesBypassingRouterAreRejected(t *testing.T) {
	h := newHarness()
	mod := validModule()
	h.mount(mod)
	h.api.GET("/sneaky", okHandler)
	h.engine.GET("/health/live", okHandler) // outside the API prefix: allowed

	rep := Validate([]Module{mod}, h.reg, h.engine.Routes(), "/api/v1")
	require.Error(t, rep.Err())
	assert.Contains(t, strings.Join(rep.Errors, "\n"), "GET /api/v1/sneaky bypasses the module router")
	assert.NotContains(t, strings.Join(rep.Errors, "\n"), "/health/live")
}

func TestWarnings(t *testing.T) {
	mod := validModule()
	mod.perms = append(mod.perms, perm.Def{Code: "iam:user:print", Menu: "iam.user"})
	prev := mod.routes
	mod.routes = func(r *Router) { prev(r); r.GET("/open", access.Public(), okHandler) }
	rep := newHarness().validate(mod)
	assert.NoError(t, rep.Err())
	assert.Contains(t, rep.Warnings, "permission iam:user:print is not used by any route (frontend-only button?)")
	assert.Contains(t, rep.Warnings, "public route: GET /api/v1/open")
}

func TestOrderedMenusPutsParentsFirst(t *testing.T) {
	mod := fakeModule{name: "iam", menus: []menu.Def{
		{Code: "iam.a.b.c", Parent: "iam.a.b"},
		{Code: "iam.a.b", Parent: "iam.a"},
		{Code: "iam.a"},
		{Code: "iam.z"},
	}}
	var order []string
	for _, mm := range orderedMenus([]Module{mod}) {
		order = append(order, mm.def.Code)
	}
	assert.Equal(t, []string{"iam.a", "iam.a.b", "iam.a.b.c", "iam.z"}, order)
}

func TestJoinPaths(t *testing.T) {
	assert.Equal(t, "/api/v1", joinPaths("/api/v1", ""))
	assert.Equal(t, "/api/v1/users", joinPaths("/api/v1", "users"))
	assert.Equal(t, "/api/v1/users/", joinPaths("/api/v1", "/users/"))
}

func TestRouteBuilderOverrides(t *testing.T) {
	h := newHarness()
	h.mount(fakeModule{
		name:  "iam",
		perms: validModule().perms,
		routes: func(r *Router) {
			r.POST("/upload", access.Perm("iam:user:create"), okHandler).
				Doc(Doc{Summary: "上传"}).MaxBody(1 << 30).Timeout(time.Hour)
			r.GET("/plain", access.Perm("iam:user:view"), okHandler)
		},
	})

	up, ok := h.reg.Find(http.MethodPost, "/api/v1/upload")
	require.True(t, ok)
	assert.Equal(t, "上传", up.Doc.Summary)
	assert.Equal(t, int64(1<<30), up.MaxBodyBytes)
	assert.Equal(t, time.Hour, up.Timeout)

	plain, ok := h.reg.Find(http.MethodGet, "/api/v1/plain")
	require.True(t, ok)
	assert.Zero(t, plain.MaxBodyBytes)
	assert.Zero(t, plain.Timeout)

	_, ok = h.reg.Find(http.MethodDelete, "/api/v1/upload")
	assert.False(t, ok)
}

// allowGuard admits requests that carry the X-Allow header.
type allowGuard struct{}

func (allowGuard) Require(access.Level) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("X-Allow") == "" {
			c.AbortWithStatus(http.StatusForbidden)
		}
	}
}

func TestRouteOverridesApplyAfterTheGuard(t *testing.T) {
	engine := gin.New()
	reg := NewRegistry()
	r := NewRouter(engine.Group("/api/v1"), "iam", allowGuard{}, reg)
	r.POST("/upload", access.Perm("iam:user:create"), func(c *gin.Context) {
		n, err := io.Copy(io.Discard, c.Request.Body)
		if err != nil {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}
		c.String(http.StatusOK, "%d", n)
	}).MaxBody(64).Timeout(10 * time.Second)

	srv := httptest.NewUnstartedServer(engine)
	srv.Config.ReadTimeout = 200 * time.Millisecond
	srv.Start()
	defer srv.Close()

	post := func(allow bool, body io.Reader, length int64) (int, error) {
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/upload", body)
		require.NoError(t, err)
		req.ContentLength = length
		if allow {
			req.Header.Set("X-Allow", "1")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, err
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode, nil
	}
	// A body trickling in for longer than read_timeout: the authorized
	// request outlives it thanks to the route's Timeout.
	slow := func() io.Reader {
		pr, pw := io.Pipe()
		go func() {
			for range 5 {
				time.Sleep(100 * time.Millisecond)
				if _, err := pw.Write([]byte("chunk")); err != nil {
					return
				}
			}
			_ = pw.Close()
		}()
		return pr
	}

	status, err := post(true, slow(), -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	// Unauthorized requests stop at the guard: no larger limit, no longer
	// deadlines.
	status, err = post(false, strings.NewReader(strings.Repeat("a", 32)), 32)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, status)

	// The route's own body limit applies after the guard.
	status, err = post(true, strings.NewReader(strings.Repeat("a", 100)), 100)
	require.NoError(t, err)
	assert.Equal(t, http.StatusRequestEntityTooLarge, status)
}

func TestPublicRouteCannotRaiseBodyLimit(t *testing.T) {
	h := newHarness()
	m := fakeModule{name: "iam", routes: func(r *Router) {
		r.POST("/open", access.Public(), okHandler).MaxBody(1 << 20)
	}}
	h.mount(m)
	report := Validate([]Module{m}, h.reg, h.engine.Routes(), "/api/v1")
	require.Error(t, report.Err())
	assert.Contains(t, report.Err().Error(), "cannot raise the body limit")
}
