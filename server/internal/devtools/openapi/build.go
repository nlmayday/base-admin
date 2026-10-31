package openapi

import (
	"cmp"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/platform/httpx"
)

// Security scheme names.
const (
	SchemeSession = "sessionCookie"
	SchemeCSRF    = "xsrfHeader"
)

// ErrorComponent names the shared error envelope schema and response.
const ErrorComponent = "ErrorEnvelope"

const jsonType = "application/json"

// Options configures the document.
type Options struct {
	Title       string
	Description string
	Version     string
	// BasePath (e.g. "/api/v1") is stripped from route paths and published
	// as the server URL.
	BasePath string
	// SessionCookie is the session cookie name ("sid", or "__Host-sid" in
	// production).
	SessionCookie string
	// CSRFHeader is the header carrying the XSRF-TOKEN cookie value.
	CSRFHeader string
	// TagDescriptions describes modules, keyed by module name.
	TagDescriptions map[string]string
}

func (o Options) withDefaults() Options {
	o.Title = cmp.Or(o.Title, "Base Admin API")
	o.Version = cmp.Or(o.Version, "1.0.0")
	o.SessionCookie = cmp.Or(o.SessionCookie, "sid")
	o.CSRFHeader = cmp.Or(o.CSRFHeader, "X-XSRF-TOKEN")
	o.BasePath = strings.TrimSuffix(o.BasePath, "/")
	return o
}

// Raw documents a response that is not wrapped in the JSON envelope, such
// as a file download. Use it as module.Doc.Response.
type Raw struct {
	ContentType string
	Description string
}

// Built is a document plus the lookup data the API doc UI needs.
type Built struct {
	*Document
	basePath string
}

// Build creates the OpenAPI document for routes. The output depends only on
// the routes and options, not on registration order.
func Build(routes []module.Route, opts Options) *Built {
	opts = opts.withDefaults()
	b := newSchemaBuilder()
	doc := &Document{
		OpenAPI: Version,
		Info:    Info{Title: opts.Title, Description: opts.Description, Version: opts.Version},
		Paths:   map[string]PathItem{},
	}
	if opts.BasePath != "" {
		doc.Servers = []Server{{URL: opts.BasePath}}
	}
	modules := map[string]bool{}
	opIDs := map[string]bool{}
	for _, r := range sortRoutes(routes) {
		p, params := convertPath(stripBase(r.Path, opts.BasePath))
		op := b.operation(r)
		op.Parameters = append(params, op.Parameters...)
		op.OperationID = uniqueOperationID(opIDs, r.Method, p)
		if doc.Paths[p] == nil {
			doc.Paths[p] = PathItem{}
		}
		doc.Paths[p][strings.ToLower(r.Method)] = op
		modules[r.Module] = true
	}
	for _, name := range sortedKeys(modules) {
		doc.Tags = append(doc.Tags, Tag{Name: name, Description: opts.TagDescriptions[name]})
	}
	b.schemas[ErrorComponent] = errorEnvelope()
	doc.Components = Components{
		Schemas: b.schemas,
		Responses: map[string]Response{ErrorComponent: {
			Description: "错误：code 为业务错误码，HTTP 状态码见 docs/api-p0.md",
			Content:     map[string]MediaType{jsonType: {Schema: &Schema{Ref: RefPrefix + ErrorComponent}}},
		}},
		SecuritySchemes: map[string]SecurityScheme{
			SchemeSession: {Type: "apiKey", In: "cookie", Name: opts.SessionCookie, Description: "登录后由服务端设置的 HttpOnly 会话 Cookie"},
			SchemeCSRF:    {Type: "apiKey", In: "header", Name: opts.CSRFHeader, Description: "不安全方法须携带 XSRF-TOKEN Cookie 的值"},
		},
	}
	return &Built{Document: doc, basePath: opts.BasePath}
}

// Operation returns the operation of a registered route (full Gin path).
func (b *Built) Operation(method, fullPath string) *Operation {
	p, _ := convertPath(stripBase(fullPath, b.basePath))
	return b.Paths[p][strings.ToLower(method)]
}

var methodOrder = map[string]int{
	http.MethodGet: 0, http.MethodPost: 1, http.MethodPut: 2, http.MethodPatch: 3, http.MethodDelete: 4,
}

func sortRoutes(routes []module.Route) []module.Route {
	out := slices.Clone(routes)
	slices.SortStableFunc(out, func(a, b module.Route) int {
		if c := cmp.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		ma, oka := methodOrder[a.Method]
		mb, okb := methodOrder[b.Method]
		if !oka {
			ma = len(methodOrder)
		}
		if !okb {
			mb = len(methodOrder)
		}
		if c := cmp.Compare(ma, mb); c != 0 {
			return c
		}
		return cmp.Compare(a.Method, b.Method)
	})
	return out
}

func stripBase(p, base string) string {
	if base != "" && (p == base || strings.HasPrefix(p, base+"/")) {
		p = strings.TrimPrefix(p, base)
	}
	if p == "" {
		return "/"
	}
	return p
}

// convertPath turns Gin parameters (":id", "*path") into OpenAPI templates
// and returns the matching path parameters.
func convertPath(p string) (string, []Parameter) {
	segs := strings.Split(p, "/")
	var params []Parameter
	for i, s := range segs {
		if len(s) < 2 || (s[0] != ':' && s[0] != '*') {
			continue
		}
		name := s[1:]
		segs[i] = "{" + name + "}"
		params = append(params, Parameter{Name: name, In: "path", Required: true, Schema: pathParamSchema(name)})
	}
	return strings.Join(segs, "/"), params
}

func pathParamSchema(name string) *Schema {
	if name == "id" || strings.HasSuffix(name, "Id") || strings.HasSuffix(name, "ID") {
		return &Schema{Type: "string", Pattern: IDPattern}
	}
	return &Schema{Type: "string"}
}

func uniqueOperationID(seen map[string]bool, method, p string) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(method))
	for _, seg := range strings.Split(p, "/") {
		if strings.HasPrefix(seg, "{") {
			b.WriteString("By")
			seg = strings.Trim(seg, "{}")
		}
		b.WriteString(pascal(seg))
	}
	id := b.String()
	for i := 2; seen[id]; i++ {
		id = b.String() + strconv.Itoa(i)
	}
	seen[id] = true
	return id
}

// pascal converts "api-routes" or "openapi.json" to "ApiRoutes"/"OpenapiJson".
func pascal(s string) string {
	var b strings.Builder
	upper := true
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			upper = true
			continue
		}
		if upper {
			r = unicode.ToUpper(r)
			upper = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (b *schemaBuilder) operation(r module.Route) *Operation {
	codes := r.Access.Codes()
	if codes == nil {
		codes = []string{}
	}
	op := &Operation{
		Tags:        []string{r.Module},
		Summary:     r.Doc.Summary,
		Description: r.Doc.Description,
		Responses:   map[string]Response{},
		Security:    security(r),
		Access:      r.Access.Kind().String(),
		Permissions: codes,
		Module:      r.Module,
		Sensitive:   r.Doc.Sensitive,
	}
	if isPage(r.Doc.Response) {
		op.Parameters = append(op.Parameters, pageParams()...)
	}
	op.Parameters = mergeParams(op.Parameters, b.queryParams(r.Doc.Query))
	if body := b.valueSchema(r.Doc.Body); body != nil {
		op.RequestBody = &RequestBody{Required: true, Content: map[string]MediaType{jsonType: {Schema: body}}}
	}
	op.Responses["200"] = b.successResponse(r)
	op.Responses["default"] = Response{Ref: "#/components/responses/" + ErrorComponent}
	return op
}

func security(r module.Route) []map[string][]string {
	if r.Access.Kind() == access.KindPublic {
		return []map[string][]string{}
	}
	req := map[string][]string{SchemeSession: {}}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
	default:
		req[SchemeCSRF] = []string{}
	}
	return []map[string][]string{req}
}

func (b *schemaBuilder) successResponse(r module.Route) Response {
	if raw, ok := r.Doc.Response.(Raw); ok {
		return rawResponse(raw)
	}
	if r.Doc.Response == nil && r.Method == http.MethodGet && strings.HasSuffix(r.Path, "/export") {
		return rawResponse(Raw{ContentType: "text/csv", Description: "CSV 文件（UTF-8 BOM）"})
	}
	return Response{
		Description: "成功",
		Content:     map[string]MediaType{jsonType: {Schema: Envelope(b.valueSchema(r.Doc.Response))}},
	}
}

func rawResponse(raw Raw) Response {
	ct := cmp.Or(raw.ContentType, "application/octet-stream")
	s := &Schema{Type: "string", Format: "binary"}
	if ct == jsonType {
		s = &Schema{Type: "object"}
	}
	return Response{
		Description: cmp.Or(raw.Description, "原始响应（不带信封）"),
		Content:     map[string]MediaType{ct: {Schema: s}},
	}
}

// Envelope wraps a data schema in the response envelope (docs/api-p0.md).
// A nil data schema documents "data": null.
func Envelope(data *Schema) *Schema {
	if data == nil {
		data = &Schema{Nullable: true, Description: "固定为 null"}
	}
	return &Schema{
		Type:     "object",
		Required: []string{"code", "message", "data", "requestId"},
		Properties: Properties{
			{Name: "code", Schema: &Schema{Type: "integer", Format: "int32", Description: "0 表示成功"}},
			{Name: "message", Schema: &Schema{Type: "string"}},
			{Name: "data", Schema: data},
			{Name: "requestId", Schema: &Schema{Type: "string"}},
		},
	}
}

func errorEnvelope() *Schema {
	s := Envelope(&Schema{
		Nullable:    true,
		Description: "参数校验失败（10002）时为 {fields: [{field, message}]}，其他错误通常为 null",
	})
	code, _ := s.Properties.Get("code")
	code.Description = "业务错误码，如 10002 参数校验失败、20001 未登录、20002 无权限"
	return s
}

var pageQueryType = reflect.TypeFor[httpx.PageQuery]()

// isPage reports whether v is an httpx.Page[T].
func isPage(v any) bool {
	if v == nil {
		return false
	}
	t := reflect.TypeOf(v)
	return t.PkgPath() == pageQueryType.PkgPath() && strings.HasPrefix(t.Name(), "Page[")
}

func pageParams() []Parameter {
	return []Parameter{
		{Name: "page", In: "query", Description: "页码，从 1 开始", Schema: &Schema{Type: "integer", Format: "int32", Minimum: floatPtr(1)}},
		{Name: "pageSize", In: "query", Description: "每页条数，默认 20", Schema: &Schema{
			Type: "integer", Format: "int32", Minimum: floatPtr(1), Maximum: floatPtr(httpx.MaxPageSize),
		}},
	}
}

func mergeParams(base, extra []Parameter) []Parameter {
	for _, p := range extra {
		if !slices.ContainsFunc(base, func(q Parameter) bool { return q.Name == p.Name && q.In == p.In }) {
			base = append(base, p)
		}
	}
	return base
}

// queryParams reflects a query struct (gin `form` tags) into parameters.
func (b *schemaBuilder) queryParams(v any) []Parameter {
	if v == nil {
		return nil
	}
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var out []Parameter
	b.collectQuery(t, &out, map[reflect.Type]bool{})
	return out
}

func (b *schemaBuilder) collectQuery(t reflect.Type, out *[]Parameter, visiting map[reflect.Type]bool) {
	if visiting[t] {
		return
	}
	visiting[t] = true
	for i := range t.NumField() {
		sf := t.Field(i)
		tag, _, _ := strings.Cut(sf.Tag.Get("form"), ",")
		if tag == "-" {
			continue
		}
		ft := sf.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if sf.Anonymous && tag == "" && ft.Kind() == reflect.Struct {
			b.collectQuery(ft, out, visiting)
			continue
		}
		if !sf.IsExported() {
			continue
		}
		s := b.schemaFor(sf.Type)
		if s.Ref != "" || (s.Type == "object" && s.AdditionalProperties == nil) {
			continue // nested structs are not bound from query strings
		}
		c := parseBinding(sf.Tag.Get("binding"))
		s = applyConstraints(s, c, sf.Type)
		*out = append(*out, Parameter{
			Name: cmp.Or(tag, sf.Name), In: "query", Required: c.required,
			Description: fieldDescription(sf), Schema: s,
		})
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
