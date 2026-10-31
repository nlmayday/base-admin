package openapi

import (
	"encoding"
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"base-admin/server/internal/platform/ids"
)

// IDPattern is the pattern of entity IDs, which are decimal strings in JSON.
const IDPattern = "^[0-9]+$"

var (
	typeID          = reflect.TypeFor[ids.ID]()
	typeTime        = reflect.TypeFor[time.Time]()
	typeDuration    = reflect.TypeFor[time.Duration]()
	typeRawMessage  = reflect.TypeFor[json.RawMessage]()
	typeMarshaler   = reflect.TypeFor[json.Marshaler]()
	typeTextMarshal = reflect.TypeFor[encoding.TextMarshaler]()
)

// schemaBuilder reflects Go types into schemas, registering named structs
// as components. It is not safe for concurrent use.
type schemaBuilder struct {
	schemas map[string]*Schema
	names   map[reflect.Type]string
	owners  map[string]reflect.Type
}

func newSchemaBuilder() *schemaBuilder {
	return &schemaBuilder{
		schemas: map[string]*Schema{},
		names:   map[reflect.Type]string{},
		owners:  map[string]reflect.Type{},
	}
}

// valueSchema returns the schema of a Doc value (a zero value of a type), or
// nil when v is nil.
func (b *schemaBuilder) valueSchema(v any) *Schema {
	if v == nil {
		return nil
	}
	return b.schemaFor(reflect.TypeOf(v))
}

// schemaFor returns a fresh schema for t; callers may decorate the result.
func (b *schemaBuilder) schemaFor(t reflect.Type) *Schema {
	if s, ok := specialSchema(t); ok {
		return s
	}
	switch t.Kind() {
	case reflect.Pointer:
		return nullable(b.schemaFor(t.Elem()))
	case reflect.Bool:
		return &Schema{Type: "boolean"}
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Uint8, reflect.Uint16:
		return &Schema{Type: "integer", Format: "int32"}
	case reflect.Int, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return &Schema{Type: "integer", Format: "int64"}
	case reflect.Float32:
		return &Schema{Type: "number", Format: "float"}
	case reflect.Float64:
		return &Schema{Type: "number", Format: "double"}
	case reflect.String:
		return &Schema{Type: "string"}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return &Schema{Type: "string", Format: "byte"}
		}
		return &Schema{Type: "array", Items: b.schemaFor(t.Elem())}
	case reflect.Map:
		return &Schema{Type: "object", AdditionalProperties: b.schemaFor(t.Elem())}
	case reflect.Struct:
		if t.Name() == "" {
			return b.structSchema(t)
		}
		return &Schema{Ref: RefPrefix + b.component(t)}
	default:
		// interface{}, channels and functions: any value.
		return &Schema{}
	}
}

// specialSchema handles types whose JSON form differs from their kind.
func specialSchema(t reflect.Type) (*Schema, bool) {
	switch t {
	case typeID:
		return &Schema{Type: "string", Pattern: IDPattern}, true
	case typeTime:
		return &Schema{Type: "string", Format: "date-time"}, true
	case typeDuration:
		return &Schema{Type: "integer", Format: "int64", Description: "纳秒"}, true
	case typeRawMessage:
		return &Schema{}, true
	}
	if t.Kind() == reflect.Pointer {
		return nil, false
	}
	pt := reflect.PointerTo(t)
	if t.Implements(typeMarshaler) || pt.Implements(typeMarshaler) {
		return &Schema{}, true
	}
	if t.Implements(typeTextMarshal) || pt.Implements(typeTextMarshal) {
		return &Schema{Type: "string"}, true
	}
	return nil, false
}

// nullable marks s as accepting null. A $ref cannot carry siblings in
// OpenAPI 3.0, so it is wrapped in allOf.
func nullable(s *Schema) *Schema {
	if s.Ref != "" {
		return &Schema{AllOf: []*Schema{s}, Nullable: true}
	}
	s.Nullable = true
	return s
}

// decorable returns s, or an allOf wrapper when s is a bare $ref.
func decorable(s *Schema) *Schema {
	if s.Ref != "" {
		return &Schema{AllOf: []*Schema{s}}
	}
	return s
}

// component registers a named struct and returns its component name. The
// placeholder is stored before the fields are reflected so recursive types
// resolve to a $ref instead of looping.
func (b *schemaBuilder) component(t reflect.Type) string {
	if name, ok := b.names[t]; ok {
		return name
	}
	base := componentName(t)
	name := base
	for i := 2; ; i++ {
		owner, taken := b.owners[name]
		if !taken || owner == t {
			break
		}
		name = base + "_" + strconv.Itoa(i)
	}
	b.names[t] = name
	b.owners[name] = t
	placeholder := &Schema{}
	b.schemas[name] = placeholder
	*placeholder = *b.structSchema(t)
	return name
}

// componentName is "<package>.<Type>"; generic arguments are appended with
// underscores, e.g. httpx.Page[iam.userView] becomes httpx.Page_iam.userView.
func componentName(t reflect.Type) string {
	name := t.Name()
	if i := strings.IndexByte(name, '['); i >= 0 && strings.HasSuffix(name, "]") {
		args := splitTopLevel(name[i+1 : len(name)-1])
		parts := make([]string, len(args))
		for j, a := range args {
			parts[j] = sanitize(shortTypeString(a))
		}
		name = name[:i] + "_" + strings.Join(parts, "_")
	}
	if pkg := lastSegment(t.PkgPath()); pkg != "" {
		name = pkg + "." + name
	}
	return sanitize(name)
}

// splitTopLevel splits generic arguments on commas outside brackets.
func splitTopLevel(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

// shortTypeString drops import paths from a type string, so
// "base-admin/server/internal/modules/iam.userView" becomes "iam.userView".
func shortTypeString(s string) string {
	var b strings.Builder
	word := strings.Builder{}
	flush := func() {
		w := word.String()
		if i := strings.LastIndexByte(w, '/'); i >= 0 {
			w = w[i+1:]
		}
		b.WriteString(w)
		word.Reset()
	}
	for _, r := range s {
		if r == '[' || r == ']' || r == '*' || r == ',' || r == ' ' {
			flush()
			b.WriteRune(r)
			continue
		}
		word.WriteRune(r)
	}
	flush()
	return b.String()
}

func lastSegment(pkgPath string) string {
	if i := strings.LastIndexByte(pkgPath, '/'); i >= 0 {
		return pkgPath[i+1:]
	}
	return pkgPath
}

// sanitize keeps the characters OpenAPI allows in component names.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

// field is one JSON-visible struct field after embedding is resolved.
type field struct {
	name   string
	depth  int
	tagged bool
	sf     reflect.StructField
}

// jsonFields lists the fields encoding/json would encode, applying its
// embedding rules: shallower fields win, then tagged ones, then the first.
func jsonFields(t reflect.Type) []field {
	var all []field
	collectFields(t, 0, map[reflect.Type]bool{}, &all)
	best := map[string]int{}
	var order []string
	for i, f := range all {
		j, ok := best[f.name]
		if !ok {
			best[f.name] = i
			order = append(order, f.name)
			continue
		}
		prev := all[j]
		if f.depth < prev.depth || (f.depth == prev.depth && f.tagged && !prev.tagged) {
			best[f.name] = i
		}
	}
	out := make([]field, 0, len(order))
	for _, name := range order {
		out = append(out, all[best[name]])
	}
	return out
}

func collectFields(t reflect.Type, depth int, visiting map[reflect.Type]bool, out *[]field) {
	if visiting[t] {
		return
	}
	visiting[t] = true
	defer delete(visiting, t)
	for i := range t.NumField() {
		sf := t.Field(i)
		name, _, skip := jsonTag(sf)
		if skip {
			continue
		}
		if sf.Anonymous && name == "" {
			ft := sf.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				if _, special := specialSchema(ft); !special {
					collectFields(ft, depth+1, visiting, out)
					continue
				}
			}
		}
		if !sf.IsExported() {
			continue
		}
		tagged := name != ""
		if name == "" {
			name = sf.Name
		}
		*out = append(*out, field{name: name, depth: depth, tagged: tagged, sf: sf})
	}
}

// jsonTag parses the json struct tag.
func jsonTag(sf reflect.StructField) (name string, opts []string, skip bool) {
	tag := sf.Tag.Get("json")
	if tag == "-" {
		return "", nil, true
	}
	parts := strings.Split(tag, ",")
	return parts[0], parts[1:], false
}

func (b *schemaBuilder) structSchema(t reflect.Type) *Schema {
	s := &Schema{Type: "object"}
	for _, f := range jsonFields(t) {
		fs := b.schemaFor(f.sf.Type)
		if _, opts, _ := jsonTag(f.sf); slices.Contains(opts, "string") && isScalar(fs) {
			fs = &Schema{Type: "string", Nullable: fs.Nullable}
		}
		c := parseBinding(f.sf.Tag.Get("binding"))
		fs = applyConstraints(fs, c, f.sf.Type)
		if d := fieldDescription(f.sf); d != "" {
			fs = decorable(fs)
			fs.Description = d
		}
		if c.required {
			s.Required = append(s.Required, f.name)
		}
		s.Properties = append(s.Properties, Property{Name: f.name, Schema: fs})
	}
	return s
}

// fieldDescription reads the optional doc (or description) struct tag.
func fieldDescription(sf reflect.StructField) string {
	if d := sf.Tag.Get("doc"); d != "" {
		return d
	}
	return sf.Tag.Get("description")
}

func isScalar(s *Schema) bool {
	switch s.Type {
	case "boolean", "integer", "number", "string":
		return true
	}
	return false
}
