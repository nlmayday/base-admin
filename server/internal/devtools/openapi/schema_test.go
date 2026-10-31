package openapi

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

type fxEmbedded struct {
	Version int    `json:"version"`
	Name    string `json:"name"` // shadowed by fxView.Name
}

type fxOwner struct {
	Nickname string `json:"nickname"`
}

type fxView struct {
	ID        ids.ID          `json:"id"`
	ParentID  *ids.ID         `json:"parentId"`
	Name      string          `json:"name" doc:"名称"`
	Tags      []string        `json:"tags"`
	Meta      map[string]int  `json:"meta"`
	CreatedAt time.Time       `json:"createdAt"`
	DeletedAt *time.Time      `json:"deletedAt"`
	Children  []*fxView       `json:"children"`
	Raw       json.RawMessage `json:"raw"`
	Count     int64           `json:"count,string"`
	hidden    string          //nolint:unused // proves unexported fields are skipped
	Skip      string          `json:"-"`
	fxEmbedded
	Inner struct {
		A int `json:"a"`
	} `json:"inner"`
	Owner   *fxOwner `json:"owner"`
	Any     any      `json:"any"`
	Bytes   []byte   `json:"bytes"`
	Ratio   float32  `json:"ratio"`
	Enabled bool     `json:"enabled"`
	NoTag   string
}

type fxRequest struct {
	Name    string     `json:"name" binding:"required,min=2,max=64"`
	Code    string     `json:"code" binding:"len=6"`
	Status  int16      `json:"status" binding:"omitempty,oneof=1 2"`
	Email   string     `json:"email" binding:"omitempty,email"`
	IDs     []ids.ID   `json:"ids" binding:"max=10,dive,required"`
	Codes   []string   `json:"codes" binding:"dive,oneof=a b"`
	Score   float64    `json:"score" binding:"gt=0,lte=100"`
	Owner   fxOwner    `json:"owner" binding:"required" doc:"负责人"`
	Version *int       `json:"version"`
	Day     string     `json:"day" binding:"omitempty,datetime=2006-01-02"`
	At      *time.Time `json:"at" binding:"omitempty"`
}

type fxFilter struct {
	Keyword  string   `form:"keyword" binding:"max=64" doc:"关键字"`
	Status   int16    `form:"status" binding:"omitempty,oneof=1 2"`
	DeptID   string   `form:"deptId" binding:"required"`
	IDs      []string `form:"ids"`
	Nested   fxOwner  `form:"nested"`
	Skipped  string   `form:"-"`
	internal bool     //nolint:unused // proves unexported fields are skipped
	fxFilterBase
}

type fxFilterBase struct {
	From time.Time `form:"from"`
}

func reflectSchema(t *testing.T, v any) (*schemaBuilder, *Schema) {
	t.Helper()
	b := newSchemaBuilder()
	s := b.valueSchema(v)
	require.NotNil(t, s)
	return b, s
}

func toJSON(t *testing.T, v any) string {
	t.Helper()
	out, err := json.Marshal(v)
	require.NoError(t, err)
	return string(out)
}

func TestSchemaReflectionMatrix(t *testing.T) {
	b, root := reflectSchema(t, fxView{})
	assert.Equal(t, RefPrefix+"openapi.fxView", root.Ref)
	view := b.schemas["openapi.fxView"]
	require.NotNil(t, view)

	cases := map[string]string{
		"id":        `{"type":"string","pattern":"^[0-9]+$"}`,
		"parentId":  `{"type":"string","pattern":"^[0-9]+$","nullable":true}`,
		"name":      `{"type":"string","description":"名称"}`,
		"tags":      `{"type":"array","items":{"type":"string"}}`,
		"meta":      `{"type":"object","additionalProperties":{"type":"integer","format":"int64"}}`,
		"createdAt": `{"type":"string","format":"date-time"}`,
		"deletedAt": `{"type":"string","format":"date-time","nullable":true}`,
		"children":  `{"type":"array","items":{"allOf":[{"$ref":"#/components/schemas/openapi.fxView"}],"nullable":true}}`,
		"raw":       `{}`,
		"count":     `{"type":"string"}`,
		"version":   `{"type":"integer","format":"int64"}`,
		"inner":     `{"type":"object","properties":{"a":{"type":"integer","format":"int64"}}}`,
		"owner":     `{"allOf":[{"$ref":"#/components/schemas/openapi.fxOwner"}],"nullable":true}`,
		"any":       `{}`,
		"bytes":     `{"type":"string","format":"byte"}`,
		"ratio":     `{"type":"number","format":"float"}`,
		"enabled":   `{"type":"boolean"}`,
		"NoTag":     `{"type":"string"}`,
	}
	for name, want := range cases {
		got, ok := view.Properties.Get(name)
		require.True(t, ok, name)
		assert.JSONEq(t, want, toJSON(t, got), name)
	}
	names := make([]string, len(view.Properties))
	for i, p := range view.Properties {
		names[i] = p.Name
	}
	assert.Equal(t, []string{"id", "parentId", "name", "tags", "meta", "createdAt", "deletedAt", "children",
		"raw", "count", "version", "inner", "owner", "any", "bytes", "ratio", "enabled", "NoTag"}, names,
		"declaration order, embedded fields flattened, shadowed/unexported/skipped fields dropped")
	assert.Contains(t, b.schemas, "openapi.fxOwner")
}

func TestSchemaConstraints(t *testing.T) {
	b, _ := reflectSchema(t, fxRequest{})
	req := b.schemas["openapi.fxRequest"]
	assert.Equal(t, []string{"name", "owner"}, req.Required)
	cases := map[string]string{
		"name":    `{"type":"string","minLength":2,"maxLength":64}`,
		"code":    `{"type":"string","minLength":6,"maxLength":6}`,
		"status":  `{"type":"integer","format":"int32","enum":[1,2]}`,
		"email":   `{"type":"string","format":"email"}`,
		"ids":     `{"type":"array","maxItems":10,"items":{"type":"string","pattern":"^[0-9]+$"}}`,
		"codes":   `{"type":"array","items":{"type":"string","enum":["a","b"]}}`,
		"score":   `{"type":"number","format":"double","minimum":0,"exclusiveMinimum":true,"maximum":100}`,
		"owner":   `{"allOf":[{"$ref":"#/components/schemas/openapi.fxOwner"}],"description":"负责人"}`,
		"version": `{"type":"integer","format":"int64","nullable":true}`,
		"day":     `{"type":"string","format":"date"}`,
		"at":      `{"type":"string","format":"date-time","nullable":true}`,
	}
	for name, want := range cases {
		got, ok := req.Properties.Get(name)
		require.True(t, ok, name)
		assert.JSONEq(t, want, toJSON(t, got), name)
	}
}

func TestParseBinding(t *testing.T) {
	c := parseBinding("required, max=5 ,dive,oneof=x y")
	assert.True(t, c.required)
	assert.Equal(t, "5", c.rules["max"])
	require.NotNil(t, c.items)
	assert.Equal(t, "x y", c.items.rules["oneof"])
	assert.False(t, parseBinding("").required)
}

func TestComponentNames(t *testing.T) {
	assert.Equal(t, "openapi.fxView", componentName(reflect.TypeFor[fxView]()))
	assert.Equal(t, "httpx.Page_openapi.fxView", componentName(reflect.TypeFor[httpx.Page[fxView]]()))
	assert.Equal(t, "httpx.Page_ids.ID", componentName(reflect.TypeFor[httpx.Page[ids.ID]]()))
	assert.Equal(t, "iam.userView", shortTypeString("base-admin/server/internal/modules/iam.userView"))
	assert.Equal(t, []string{"a.B", "map[string]c.D"}, splitTopLevel("a.B, map[string]c.D"))
	assert.Equal(t, "a_b", sanitize("[a b]"))
}

func TestComponentNameCollisionGetsSuffix(t *testing.T) {
	b := newSchemaBuilder()
	b.owners["openapi.fxOwner"] = reflect.TypeFor[fxView]() // pretend another package took it
	assert.Equal(t, "openapi.fxOwner_2", b.component(reflect.TypeFor[fxOwner]()))
	assert.Equal(t, "openapi.fxOwner_2", b.component(reflect.TypeFor[fxOwner]()), "stable on reuse")
}

func TestGenericPageSchema(t *testing.T) {
	b, root := reflectSchema(t, httpx.Page[fxOwner]{})
	assert.Equal(t, RefPrefix+"httpx.Page_openapi.fxOwner", root.Ref)
	page := b.schemas["httpx.Page_openapi.fxOwner"]
	list, ok := page.Properties.Get("list")
	require.True(t, ok)
	assert.JSONEq(t, `{"type":"array","items":{"$ref":"#/components/schemas/openapi.fxOwner"}}`, toJSON(t, list))
}

func TestQueryParams(t *testing.T) {
	b := newSchemaBuilder()
	params := b.queryParams(&fxFilter{})
	got := map[string]Parameter{}
	var names []string
	for _, p := range params {
		got[p.Name] = p
		names = append(names, p.Name)
		assert.Equal(t, "query", p.In)
	}
	assert.Equal(t, []string{"keyword", "status", "deptId", "ids", "from"}, names,
		"nested structs, '-' and unexported fields skipped; embedded flattened")
	assert.JSONEq(t, `{"type":"string","maxLength":64}`, toJSON(t, got["keyword"].Schema))
	assert.Equal(t, "关键字", got["keyword"].Description)
	assert.True(t, got["deptId"].Required)
	assert.JSONEq(t, `{"type":"integer","format":"int32","enum":[1,2]}`, toJSON(t, got["status"].Schema))
	assert.Nil(t, b.queryParams(nil))
	assert.Nil(t, b.queryParams(3))
}

type fxNode struct {
	Name     string    `json:"name"`
	Children []*fxNode `json:"children"`
	Parent   *fxNode   `json:"parent"`
}

func TestRecursiveTypesUseRefs(t *testing.T) {
	b, _ := reflectSchema(t, []*fxNode{})
	node := b.schemas["openapi.fxNode"]
	require.NotNil(t, node)
	parent, _ := node.Properties.Get("parent")
	assert.JSONEq(t, `{"allOf":[{"$ref":"#/components/schemas/openapi.fxNode"}],"nullable":true}`, toJSON(t, parent))
	assert.NotPanics(t, func() { _ = toJSON(t, b.schemas) })
}
