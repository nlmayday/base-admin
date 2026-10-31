package openapi

import (
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/platform/httpx"
)

func fixtureRoutes() []module.Route {
	return []module.Route{
		{Method: http.MethodGet, Path: "/api/v1/things", Module: "demo", Access: access.Perm("demo:thing:view"),
			Doc: module.Doc{Summary: "列表", Query: fxFilter{}, Response: httpx.Page[fxView]{}}},
		{Method: http.MethodPost, Path: "/api/v1/things", Module: "demo", Access: access.Perm("demo:thing:create"),
			Doc: module.Doc{Summary: "新增", Body: fxRequest{}, Response: fxView{}}},
		{Method: http.MethodGet, Path: "/api/v1/things/:id", Module: "demo", Access: access.AnyPerm("demo:thing:view", "demo:other:view"),
			Doc: module.Doc{Summary: "详情", Response: fxView{}}},
		{Method: http.MethodDelete, Path: "/api/v1/things/:id", Module: "demo", Access: access.Perm("demo:thing:delete"),
			Doc: module.Doc{Summary: "删除"}},
		{Method: http.MethodGet, Path: "/api/v1/things/export", Module: "demo", Access: access.Perm("demo:thing:export"),
			Doc: module.Doc{Summary: "导出", Query: fxFilter{}, Sensitive: true}},
		{Method: http.MethodPost, Path: "/api/v1/auth/login", Module: "auth", Access: access.Public(),
			Doc: module.Doc{Summary: "登录", Body: fxOwner{}}},
		{Method: http.MethodGet, Path: "/api/v1/me", Module: "auth", Access: access.Authenticated(),
			Doc: module.Doc{Summary: "当前用户", Response: []fxNode{}}},
		{Method: http.MethodGet, Path: "/api/v1/files/*path", Module: "file", Access: access.Authenticated(),
			Doc: module.Doc{Summary: "下载", Response: Raw{ContentType: "application/pdf"}}},
		{Method: http.MethodPut, Path: "/api/v1/users/:userId/roles/:roleId", Module: "demo", Access: access.Perm("demo:thing:update")},
	}
}

func build(t *testing.T) *Built {
	t.Helper()
	return Build(fixtureRoutes(), Options{BasePath: "/api/v1/"})
}

func TestBuildPathsAndParameters(t *testing.T) {
	doc := build(t)
	assert.Equal(t, Version, doc.OpenAPI)
	assert.Equal(t, []Server{{URL: "/api/v1"}}, doc.Servers)
	paths := sortedKeys(doc.Paths)
	assert.Equal(t, []string{"/auth/login", "/files/{path}", "/me", "/things", "/things/export", "/things/{id}",
		"/users/{userId}/roles/{roleId}"}, paths)

	get := doc.Paths["/things/{id}"]["get"]
	require.NotNil(t, get)
	require.Len(t, get.Parameters, 1)
	assert.Equal(t, Parameter{Name: "id", In: "path", Required: true, Schema: &Schema{Type: "string", Pattern: IDPattern}}, get.Parameters[0])

	put := doc.Paths["/users/{userId}/roles/{roleId}"]["put"]
	assert.Equal(t, []string{"userId", "roleId"}, []string{put.Parameters[0].Name, put.Parameters[1].Name})
	assert.Equal(t, IDPattern, put.Parameters[1].Schema.Pattern)
	assert.Equal(t, "putUsersByUserIdRolesByRoleId", put.OperationID)

	file := doc.Paths["/files/{path}"]["get"]
	assert.Equal(t, &Schema{Type: "string"}, file.Parameters[0].Schema)

	list := doc.Paths["/things"]["get"]
	var names []string
	for _, p := range list.Parameters {
		names = append(names, p.Name)
	}
	assert.Equal(t, []string{"page", "pageSize", "keyword", "status", "deptId", "ids", "from"}, names,
		"paged responses document page and pageSize first")
	assert.Equal(t, doc.Operation(http.MethodGet, "/api/v1/things"), list)
	assert.Nil(t, doc.Operation(http.MethodPatch, "/api/v1/things"))
}

func TestBuildSecurityAndExtensions(t *testing.T) {
	doc := build(t)
	login := doc.Paths["/auth/login"]["post"]
	assert.Equal(t, []map[string][]string{}, login.Security, "public routes need no credentials")
	assert.Equal(t, "public", login.Access)
	assert.Equal(t, []string{}, login.Permissions)

	me := doc.Paths["/me"]["get"]
	assert.Equal(t, []map[string][]string{{SchemeSession: {}}}, me.Security, "safe methods need only the cookie")
	assert.Equal(t, "authenticated", me.Access)

	create := doc.Paths["/things"]["post"]
	assert.Equal(t, []map[string][]string{{SchemeSession: {}, SchemeCSRF: {}}}, create.Security)
	assert.Equal(t, []string{"demo:thing:create"}, create.Permissions)
	assert.Equal(t, "permission", create.Access)

	get := doc.Paths["/things/{id}"]["get"]
	assert.Equal(t, []string{"demo:thing:view", "demo:other:view"}, get.Permissions)

	schemes := doc.Components.SecuritySchemes
	assert.Equal(t, SecurityScheme{Type: "apiKey", In: "cookie", Name: "sid", Description: schemes[SchemeSession].Description}, schemes[SchemeSession])
	assert.Equal(t, "X-XSRF-TOKEN", schemes[SchemeCSRF].Name)

	prod := Build(fixtureRoutes(), Options{SessionCookie: "__Host-sid"})
	assert.Equal(t, "__Host-sid", prod.Components.SecuritySchemes[SchemeSession].Name)
	assert.Nil(t, prod.Servers)
	assert.Contains(t, prod.Paths, "/api/v1/things")
}

func TestBuildBodiesAndResponses(t *testing.T) {
	doc := build(t)
	create := doc.Paths["/things"]["post"]
	require.NotNil(t, create.RequestBody)
	assert.True(t, create.RequestBody.Required)
	assert.Equal(t, RefPrefix+"openapi.fxRequest", create.RequestBody.Content[jsonType].Schema.Ref)

	env := create.Responses["200"].Content[jsonType].Schema
	assert.Equal(t, []string{"code", "message", "data", "requestId"}, env.Required)
	data, _ := env.Properties.Get("data")
	assert.Equal(t, RefPrefix+"openapi.fxView", data.Ref)
	assert.Equal(t, "#/components/responses/"+ErrorComponent, create.Responses["default"].Ref)

	list := doc.Paths["/things"]["get"].Responses["200"].Content[jsonType].Schema
	data, _ = list.Properties.Get("data")
	assert.Equal(t, RefPrefix+"httpx.Page_openapi.fxView", data.Ref)

	del := doc.Paths["/things/{id}"]["delete"].Responses["200"].Content[jsonType].Schema
	data, _ = del.Properties.Get("data")
	assert.True(t, data.Nullable, "no response type documents data: null")
	assert.Nil(t, doc.Paths["/things/{id}"]["delete"].RequestBody)

	export := doc.Paths["/things/export"]["get"]
	assert.Contains(t, export.Responses["200"].Content, "text/csv")
	assert.True(t, export.Sensitive)

	file := doc.Paths["/files/{path}"]["get"].Responses["200"]
	assert.Equal(t, &Schema{Type: "string", Format: "binary"}, file.Content["application/pdf"].Schema)

	for _, name := range []string{"openapi.fxView", "openapi.fxRequest", "openapi.fxOwner", "openapi.fxNode",
		"httpx.Page_openapi.fxView", ErrorComponent} {
		assert.Contains(t, doc.Components.Schemas, name)
	}
	assert.Equal(t, []Tag{{Name: "auth"}, {Name: "demo"}, {Name: "file"}}, doc.Tags)
}

func TestBuildIsDeterministic(t *testing.T) {
	want := toJSON(t, build(t).Document)
	for i := range 20 {
		routes := fixtureRoutes()
		r := rand.New(rand.NewPCG(uint64(i), 7))
		r.Shuffle(len(routes), func(a, b int) { routes[a], routes[b] = routes[b], routes[a] })
		got := toJSON(t, Build(routes, Options{BasePath: "/api/v1"}).Document)
		require.Equal(t, want, got, "shuffle %d", i)
	}
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(want), &parsed), "output is valid JSON")
}

func TestOperationIDsAreUnique(t *testing.T) {
	routes := []module.Route{
		{Method: http.MethodGet, Path: "/a-b", Module: "m", Access: access.Public()},
		{Method: http.MethodGet, Path: "/a/b", Module: "m", Access: access.Public()},
	}
	doc := Build(routes, Options{})
	ids := []string{doc.Paths["/a-b"]["get"].OperationID, doc.Paths["/a/b"]["get"].OperationID}
	slices.Sort(ids)
	assert.Equal(t, []string{"getAB", "getAB2"}, ids)
}

func TestConvertPath(t *testing.T) {
	for in, want := range map[string]string{
		"/":                 "/",
		"/users":            "/users",
		"/users/:id":        "/users/{id}",
		"/a/:x/b/*rest":     "/a/{x}/b/{rest}",
		"/files/:name.json": "/files/{name.json}",
	} {
		got, _ := convertPath(in)
		assert.Equal(t, want, got, in)
	}
	assert.Equal(t, "/", stripBase("/api/v1", "/api/v1"))
	assert.Equal(t, "/api/v10/x", stripBase("/api/v10/x", "/api/v1"))
}
