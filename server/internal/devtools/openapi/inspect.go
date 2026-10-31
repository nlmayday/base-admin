package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Field is one node of a schema tree shown by the API doc UI.
type Field struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Nullable    bool     `json:"nullable"`
	Constraints []string `json:"constraints"`
	Description string   `json:"description"`
	// Ref is the component name when the field's type is a named struct.
	Ref string `json:"ref,omitempty"`
	// Circular marks a reference back to an enclosing type; its children
	// are not expanded again.
	Circular bool    `json:"circular,omitempty"`
	Children []Field `json:"children,omitempty"`
}

// view is a schema with allOf/nullable wrappers and $refs resolved.
type view struct {
	s           *Schema
	ref         string
	nullable    bool
	description string
}

// resolve unwraps a schema: decorators (allOf with a single $ref, which
// is how nullable or described references are written) and the $ref.
func (d *Document) resolve(s *Schema) view {
	v := view{s: s, nullable: s.Nullable, description: s.Description}
	if len(s.AllOf) == 1 {
		s = s.AllOf[0]
		v.s = s
	}
	if s.Ref != "" {
		v.ref = RefName(s.Ref)
		v.s = d.Components.Schemas[v.ref]
		if v.s == nil {
			v.s = &Schema{}
		}
		if v.description == "" {
			v.description = v.s.Description
		}
	}
	return v
}

// Fields returns the tree of s: the properties of an object, the item
// properties of an array of objects, or a single "(value)" node.
func (d *Document) Fields(s *Schema) []Field {
	if s == nil {
		return []Field{}
	}
	v := d.resolve(s)
	visiting := map[string]bool{}
	if v.ref != "" {
		visiting[v.ref] = true
	}
	switch {
	case len(v.s.Properties) > 0:
		return d.properties(v.s, visiting)
	case v.s.Type == "array" && v.s.Items != nil:
		item := d.field("[]", v.s.Items, false, visiting)
		return []Field{item}
	default:
		return []Field{d.field("(value)", s, true, visiting)}
	}
}

func (d *Document) properties(s *Schema, visiting map[string]bool) []Field {
	out := make([]Field, 0, len(s.Properties))
	for _, p := range s.Properties {
		required := slices.Contains(s.Required, p.Name)
		out = append(out, d.field(p.Name, p.Schema, required, visiting))
	}
	return out
}

func (d *Document) field(name string, s *Schema, required bool, visiting map[string]bool) Field {
	v := d.resolve(s)
	f := Field{
		Name: name, Type: d.typeLabel(s), Required: required, Nullable: v.nullable,
		Constraints: constraintsOf(s, v.s), Description: v.description, Ref: v.ref,
	}
	if v.ref != "" && visiting[v.ref] {
		f.Circular = true
		return f
	}
	if v.ref != "" {
		visiting[v.ref] = true
		defer delete(visiting, v.ref)
	}
	f.Children = d.children(v.s, visiting)
	if v.s.Type == "array" && v.s.Items != nil {
		if item := d.resolve(v.s.Items); item.ref != "" && visiting[item.ref] {
			f.Circular = true
		}
	}
	return f
}

// children expands objects, arrays of objects and maps of objects.
func (d *Document) children(s *Schema, visiting map[string]bool) []Field {
	switch {
	case len(s.Properties) > 0:
		return d.properties(s, visiting)
	case s.Type == "array" && s.Items != nil:
		item := d.resolve(s.Items)
		if item.ref != "" && visiting[item.ref] {
			return nil
		}
		if len(item.s.Properties) > 0 || item.s.Type == "array" {
			if item.ref != "" {
				visiting[item.ref] = true
				defer delete(visiting, item.ref)
			}
			return d.children(item.s, visiting)
		}
	case s.AdditionalProperties != nil:
		val := d.resolve(s.AdditionalProperties)
		if len(val.s.Properties) > 0 && !visiting[val.ref] {
			return []Field{d.field("{key}", s.AdditionalProperties, false, visiting)}
		}
	}
	return nil
}

// typeLabel describes a schema for humans, e.g. "string(date-time)",
// "array<iam.userView>" or "integer(int64)".
func (d *Document) typeLabel(s *Schema) string {
	v := d.resolve(s)
	if v.ref != "" {
		return v.ref
	}
	t := v.s
	switch t.Type {
	case "":
		return "any"
	case "array":
		if t.Items == nil {
			return "array"
		}
		return "array<" + d.typeLabel(t.Items) + ">"
	case "object":
		if t.AdditionalProperties != nil {
			return "map<string, " + d.typeLabel(t.AdditionalProperties) + ">"
		}
		return "object"
	case "string":
		if t.Pattern == IDPattern {
			return "string(id)"
		}
	}
	if t.Format != "" {
		return t.Type + "(" + t.Format + ")"
	}
	return t.Type
}

// constraintsOf lists validation rules in Chinese, reading the wrapper
// (where field constraints live) and the resolved schema.
func constraintsOf(schemas ...*Schema) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range schemas {
		if s == nil {
			continue
		}
		for _, c := range describeConstraints(s) {
			add(c)
		}
	}
	return out
}

func describeConstraints(s *Schema) []string {
	var out []string
	if s.MinLength != nil && s.MaxLength != nil && *s.MinLength == *s.MaxLength {
		out = append(out, fmt.Sprintf("长度 = %d", *s.MinLength))
	} else {
		if s.MinLength != nil {
			out = append(out, fmt.Sprintf("长度 ≥ %d", *s.MinLength))
		}
		if s.MaxLength != nil {
			out = append(out, fmt.Sprintf("长度 ≤ %d", *s.MaxLength))
		}
	}
	if s.Minimum != nil {
		out = append(out, bound(">", "≥", s.ExclusiveMinimum, *s.Minimum))
	}
	if s.Maximum != nil {
		out = append(out, bound("<", "≤", s.ExclusiveMaximum, *s.Maximum))
	}
	if s.MinItems != nil {
		out = append(out, fmt.Sprintf("至少 %d 项", *s.MinItems))
	}
	if s.MaxItems != nil {
		out = append(out, fmt.Sprintf("最多 %d 项", *s.MaxItems))
	}
	if len(s.Enum) > 0 {
		vals := make([]string, len(s.Enum))
		for i, e := range s.Enum {
			vals[i] = fmt.Sprint(e)
		}
		out = append(out, "可选值："+strings.Join(vals, " | "))
	}
	if s.Pattern != "" && s.Pattern != IDPattern {
		out = append(out, "格式："+s.Pattern)
	}
	switch s.Format {
	case "email", "uri", "uuid", "ipv4", "ipv6", "date":
		out = append(out, "格式："+s.Format)
	}
	return out
}

func bound(excl, incl string, exclusive bool, n float64) string {
	op := incl
	if exclusive {
		op = excl
	}
	return op + " " + strconv.FormatFloat(n, 'f', -1, 64)
}

// Object is a JSON object that keeps its key order.
type Object []KV

// KV is one member of an Object.
type KV struct {
	Key   string
	Value any
}

// MarshalJSON encodes the members in order.
func (o Object) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, kv := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(kv.Key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(kv.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Example generates a representative value for s. Recursive references
// stop at the first repetition.
func (d *Document) Example(s *Schema) any {
	if s == nil {
		return nil
	}
	return d.example(s, map[string]bool{})
}

// EnvelopeExample wraps a data example in the success envelope.
func EnvelopeExample(data any) Object {
	return Object{{"code", 0}, {"message", "success"}, {"data", data}, {"requestId", "req_0123456789abcdef"}}
}

func (d *Document) example(s *Schema, visiting map[string]bool) any {
	v := d.resolve(s)
	if v.ref != "" {
		if visiting[v.ref] {
			return nil
		}
		visiting[v.ref] = true
		defer delete(visiting, v.ref)
	}
	t := v.s
	// Constraints may sit on the wrapper; prefer its enum.
	if len(s.Enum) > 0 {
		return s.Enum[0]
	}
	if len(t.Enum) > 0 {
		return t.Enum[0]
	}
	switch t.Type {
	case "string":
		return stringExample(t, s)
	case "integer", "number":
		return numberExample(t, s)
	case "boolean":
		return false
	case "array":
		if t.Items == nil {
			return []any{}
		}
		return []any{d.example(t.Items, visiting)}
	case "object":
		if len(t.Properties) == 0 && t.AdditionalProperties != nil {
			return Object{{"key", d.example(t.AdditionalProperties, visiting)}}
		}
		obj := make(Object, 0, len(t.Properties))
		for _, p := range t.Properties {
			obj = append(obj, KV{p.Name, d.example(p.Schema, visiting)})
		}
		return obj
	}
	return nil
}

func stringExample(t, wrapper *Schema) string {
	format := t.Format
	if wrapper.Format != "" {
		format = wrapper.Format
	}
	switch {
	case t.Pattern == IDPattern:
		return "1"
	case format == "date-time":
		return "2026-01-01T08:00:00+08:00"
	case format == "date":
		return "2026-01-01"
	case format == "email":
		return "user@example.com"
	case format == "uri":
		return "https://example.com"
	case format == "uuid":
		return "00000000-0000-0000-0000-000000000000"
	case format == "byte", format == "binary":
		return ""
	}
	return "string"
}

func numberExample(t, wrapper *Schema) any {
	for _, s := range []*Schema{wrapper, t} {
		if s.Minimum != nil {
			n := *s.Minimum
			if s.ExclusiveMinimum {
				n++
			}
			if t.Type == "integer" {
				return int64(n)
			}
			return n
		}
	}
	return 0
}
