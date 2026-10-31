package devtools

import (
	"cmp"
	"context"
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"

	"base-admin/server/internal/core/module"
	"base-admin/server/internal/devtools/openapi"
	"base-admin/server/internal/platform/httpx"
)

const jsonContent = "application/json"

func (m *Module) openAPI(c *gin.Context) {
	doc, err := m.doc()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Header("Content-Disposition", `inline; filename="openapi.json"`)
	c.Data(http.StatusOK, "application/json; charset=utf-8", doc.raw)
}

type permissionView struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type parameterView struct {
	Name        string   `json:"name"`
	In          string   `json:"in"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Constraints []string `json:"constraints"`
	Description string   `json:"description"`
}

type routeView struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Module      string `json:"module"`
	OperationID string `json:"operationId"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	// Access is public, authenticated or permission; any one of
	// Permissions grants access.
	Access      string           `json:"access"`
	Permissions []permissionView `json:"permissions"`
	Sensitive   bool             `json:"sensitive"`
	// CSRF reports whether the X-XSRF-TOKEN header is required.
	CSRF            bool            `json:"csrf"`
	Parameters      []parameterView `json:"parameters"`
	RequestBody     []openapi.Field `json:"requestBody"`
	RequestExample  any             `json:"requestExample"`
	ResponseType    string          `json:"responseType"`
	Response        []openapi.Field `json:"response"`
	ResponseExample any             `json:"responseExample"`
}

type moduleView struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Count int    `json:"count"`
}

type routesView struct {
	Modules []moduleView `json:"modules"`
	Routes  []routeView  `json:"routes"`
}

func (m *Module) listRoutes(c *gin.Context) (any, error) {
	doc, err := m.doc()
	if err != nil {
		return nil, err
	}
	built := doc.built
	names, err := m.permissionNames(c.Request.Context())
	if err != nil {
		return nil, err
	}
	routes := m.deps.Registry.Routes()
	out := routesView{Modules: []moduleView{}, Routes: make([]routeView, 0, len(routes))}
	counts := map[string]int{}
	for _, r := range routes {
		op := built.Operation(r.Method, r.Path)
		if op == nil {
			continue
		}
		out.Routes = append(out.Routes, describe(built, r, op, names))
		counts[r.Module]++
	}
	slices.SortFunc(out.Routes, func(a, b routeView) int {
		return cmp.Or(cmp.Compare(a.Module, b.Module), cmp.Compare(a.Path, b.Path), cmp.Compare(a.Method, b.Method))
	})
	for name, n := range counts {
		out.Modules = append(out.Modules, moduleView{Name: name, Title: cmp.Or(moduleTitles[name], name), Count: n})
	}
	slices.SortFunc(out.Modules, func(a, b moduleView) int { return cmp.Compare(a.Name, b.Name) })
	return out, nil
}

func (m *Module) permissionNames(ctx context.Context) (map[string]string, error) {
	var rows []permissionView
	if err := m.deps.DB.WithContext(ctx).Raw("SELECT code, name FROM sys_permissions").Scan(&rows).Error; err != nil {
		return nil, err
	}
	names := make(map[string]string, len(rows))
	for _, r := range rows {
		names[r.Code] = r.Name
	}
	return names, nil
}

func describe(doc *openapi.Built, r module.Route, op *openapi.Operation, names map[string]string) routeView {
	v := routeView{
		Method: r.Method, Path: r.Path, Module: r.Module, OperationID: op.OperationID,
		Summary: op.Summary, Description: op.Description, Access: op.Access, Sensitive: op.Sensitive,
		Permissions: make([]permissionView, 0, len(op.Permissions)),
		Parameters:  make([]parameterView, 0, len(op.Parameters)),
	}
	for _, code := range op.Permissions {
		v.Permissions = append(v.Permissions, permissionView{Code: code, Name: names[code]})
	}
	for _, sec := range op.Security {
		if _, ok := sec[openapi.SchemeCSRF]; ok {
			v.CSRF = true
		}
	}
	for _, p := range op.Parameters {
		f := doc.Fields(p.Schema)[0]
		v.Parameters = append(v.Parameters, parameterView{
			Name: p.Name, In: p.In, Type: f.Type, Required: p.Required,
			Constraints: f.Constraints, Description: cmp.Or(p.Description, f.Description),
		})
	}
	if op.RequestBody != nil {
		body := op.RequestBody.Content[jsonContent].Schema
		v.RequestBody = doc.Fields(body)
		v.RequestExample = doc.Example(body)
	}
	describeResponse(doc, op, &v)
	return v
}

func describeResponse(doc *openapi.Built, op *openapi.Operation, v *routeView) {
	ok := op.Responses["200"]
	media, isJSON := ok.Content[jsonContent]
	if !isJSON || media.Schema == nil || len(media.Schema.Properties) == 0 {
		for ct := range ok.Content {
			v.ResponseType = ct
		}
		return
	}
	v.ResponseType = jsonContent
	data, _ := media.Schema.Properties.Get("data")
	if data != nil && (data.Type != "" || data.Ref != "" || len(data.AllOf) > 0) {
		v.Response = doc.Fields(data)
		v.ResponseExample = openapi.EnvelopeExample(doc.Example(data))
		return
	}
	v.ResponseExample = openapi.EnvelopeExample(nil)
}
