package openapi

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fieldByName(fields []Field, name string) Field {
	for _, f := range fields {
		if f.Name == name {
			return f
		}
	}
	return Field{}
}

func TestFieldsTree(t *testing.T) {
	doc := build(t)
	body := doc.Operation(http.MethodPost, "/api/v1/things").RequestBody.Content[jsonType].Schema
	fields := doc.Fields(body)
	require.NotEmpty(t, fields)

	name := fieldByName(fields, "name")
	assert.Equal(t, Field{Name: "name", Type: "string", Required: true, Constraints: []string{"长度 ≥ 2", "长度 ≤ 64"}}, name)
	assert.Equal(t, []string{"长度 = 6"}, fieldByName(fields, "code").Constraints)
	assert.Equal(t, []string{"可选值：1 | 2"}, fieldByName(fields, "status").Constraints)
	assert.Equal(t, "integer(int32)", fieldByName(fields, "status").Type)
	assert.Equal(t, []string{"> 0", "≤ 100"}, fieldByName(fields, "score").Constraints)
	assert.Equal(t, []string{"最多 10 项"}, fieldByName(fields, "ids").Constraints)
	assert.Equal(t, "array<string(id)>", fieldByName(fields, "ids").Type)
	assert.Equal(t, []string{"格式：date"}, fieldByName(fields, "day").Constraints)

	owner := fieldByName(fields, "owner")
	assert.Equal(t, "openapi.fxOwner", owner.Type)
	assert.Equal(t, "负责人", owner.Description)
	assert.True(t, owner.Required)
	require.Len(t, owner.Children, 1)
	assert.Equal(t, "nickname", owner.Children[0].Name)
}

func TestFieldsTreeHandlesCyclesAndArrays(t *testing.T) {
	doc := build(t)
	data, _ := doc.Operation(http.MethodGet, "/api/v1/me").Responses["200"].Content[jsonType].Schema.Properties.Get("data")
	fields := doc.Fields(data)
	require.Len(t, fields, 1)
	root := fields[0]
	assert.Equal(t, "[]", root.Name)
	assert.Equal(t, "openapi.fxNode", root.Type)
	children := fieldByName(root.Children, "children")
	assert.Equal(t, "array<openapi.fxNode>", children.Type)
	assert.Empty(t, children.Children, "recursion stops at the enclosing type")
	assert.True(t, children.Circular)
	parent := fieldByName(root.Children, "parent")
	assert.True(t, parent.Circular)
	assert.True(t, parent.Nullable)

	view, _ := doc.Operation(http.MethodGet, "/api/v1/things/:id").Responses["200"].Content[jsonType].Schema.Properties.Get("data")
	vf := doc.Fields(view)
	assert.Equal(t, "string(id)", fieldByName(vf, "parentId").Type)
	assert.True(t, fieldByName(vf, "parentId").Nullable)
	assert.Equal(t, "名称", fieldByName(vf, "name").Description)
	assert.Equal(t, "map<string, integer(int64)>", fieldByName(vf, "meta").Type)
	assert.True(t, fieldByName(vf, "children").Circular, "an array of the enclosing type is marked circular")
	assert.Empty(t, fieldByName(vf, "children").Children)

	scalar := doc.Fields(&Schema{Type: "string"})
	assert.Equal(t, []Field{{Name: "(value)", Type: "string", Required: true, Constraints: []string{}}}, scalar)
	assert.Equal(t, []Field{}, doc.Fields(nil))
}

func TestExamples(t *testing.T) {
	doc := build(t)
	body := doc.Operation(http.MethodPost, "/api/v1/things").RequestBody.Content[jsonType].Schema
	assert.JSONEq(t, `{"name":"string","code":"string","status":1,"email":"user@example.com","ids":["1"],
		"codes":["a"],"score":1,"owner":{"nickname":"string"},"version":0,"day":"2026-01-01",
		"at":"2026-01-01T08:00:00+08:00"}`, toJSON(t, doc.Example(body)))

	data, _ := doc.Operation(http.MethodGet, "/api/v1/me").Responses["200"].Content[jsonType].Schema.Properties.Get("data")
	assert.JSONEq(t, `[{"name":"string","children":[null],"parent":null}]`, toJSON(t, doc.Example(data)),
		"recursive references end in null")

	assert.JSONEq(t, `{"code":0,"message":"success","data":{"a":1},"requestId":"req_0123456789abcdef"}`,
		toJSON(t, EnvelopeExample(Object{{"a", 1}})))
	assert.Nil(t, doc.Example(nil))
	assert.JSONEq(t, `{"key":0}`, toJSON(t, doc.Example(&Schema{Type: "object", AdditionalProperties: &Schema{Type: "integer"}})))
}

func TestObjectKeepsOrder(t *testing.T) {
	assert.Equal(t, `{"z":1,"a":[true]}`, toJSON(t, Object{{"z", 1}, {"a", []any{true}}}))
	assert.Equal(t, `{}`, toJSON(t, Object{}))
	assert.Equal(t, `{"b":{},"a":{"type":"string"}}`, toJSON(t, Properties{{"b", &Schema{}}, {"a", &Schema{Type: "string"}}}))
}
