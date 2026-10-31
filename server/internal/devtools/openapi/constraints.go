package openapi

import (
	"reflect"
	"strconv"
	"strings"
)

// constraints is the documentable subset of go-playground/validator rules
// found in a `binding` tag. Rules after "dive" apply to elements.
type constraints struct {
	required bool
	rules    map[string]string
	items    *constraints
}

// parseBinding parses a binding tag such as "required,max=64" or
// "omitempty,dive,oneof=a b".
func parseBinding(tag string) constraints {
	c := constraints{rules: map[string]string{}}
	if tag == "" {
		return c
	}
	parts := strings.Split(tag, ",")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "dive" {
			rest := parseBinding(strings.Join(parts[i+1:], ","))
			c.items = &rest
			return c
		}
		key, val, _ := strings.Cut(p, "=")
		switch key {
		case "":
		case "required":
			c.required = true
		default:
			c.rules[key] = val
		}
	}
	return c
}

// applyConstraints documents c on s. The Go type decides how bounds read:
// lengths for strings, item counts for lists and values for numbers.
func applyConstraints(s *Schema, c constraints, t reflect.Type) *Schema {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if len(c.rules) == 0 && c.items == nil {
		return s
	}
	target := decorable(s)
	kind := boundKind(s, t)
	for _, key := range []string{"len", "min", "max", "gt", "gte", "lt", "lte"} {
		if val, ok := c.rules[key]; ok {
			applyBound(target, kind, key, val)
		}
	}
	if val, ok := c.rules["oneof"]; ok {
		target.Enum = enumValues(strings.Fields(val), kind)
	}
	if format := formatRule(c.rules); format != "" {
		target.Format = format
	}
	if c.items != nil && s.Items != nil {
		s.Items = applyConstraints(s.Items, *c.items, t.Elem())
	}
	return target
}

// boundKind is "string", "number" or "array"; "" when bounds don't apply.
func boundKind(s *Schema, t reflect.Type) string {
	if t == typeID || t == typeTime {
		return ""
	}
	switch s.Type {
	case "string":
		return "string"
	case "integer", "number":
		return "number"
	case "array":
		return "array"
	}
	return ""
}

func applyBound(s *Schema, kind, rule, param string) {
	n, err := strconv.ParseFloat(param, 64)
	if err != nil {
		return
	}
	lower := rule == "min" || rule == "gte" || rule == "gt"
	exclusive := rule == "gt" || rule == "lt"
	switch kind {
	case "string", "array":
		v := int(n)
		if exclusive && lower {
			v++
		} else if exclusive {
			v--
		}
		setCount(s, kind, rule, lower, v)
	case "number":
		if rule == "len" {
			return
		}
		if lower {
			s.Minimum, s.ExclusiveMinimum = floatPtr(n), exclusive
		} else {
			s.Maximum, s.ExclusiveMaximum = floatPtr(n), exclusive
		}
	}
}

func setCount(s *Schema, kind, rule string, lower bool, v int) {
	setMin, setMax := lower, !lower
	if rule == "len" {
		setMin, setMax = true, true
	}
	if kind == "string" {
		if setMin {
			s.MinLength = intPtr(v)
		}
		if setMax {
			s.MaxLength = intPtr(v)
		}
		return
	}
	if setMin {
		s.MinItems = intPtr(v)
	}
	if setMax {
		s.MaxItems = intPtr(v)
	}
}

func enumValues(vals []string, kind string) []any {
	out := make([]any, 0, len(vals))
	for _, v := range vals {
		if kind == "number" {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				if n == float64(int64(n)) {
					out = append(out, int64(n))
				} else {
					out = append(out, n)
				}
				continue
			}
		}
		out = append(out, v)
	}
	return out
}

func formatRule(rules map[string]string) string {
	for _, f := range []struct{ rule, format string }{
		{"email", "email"}, {"url", "uri"}, {"uri", "uri"}, {"http_url", "uri"},
		{"uuid", "uuid"}, {"uuid4", "uuid"}, {"ip", "ipv4"}, {"ipv4", "ipv4"}, {"ipv6", "ipv6"},
	} {
		if _, ok := rules[f.rule]; ok {
			return f.format
		}
	}
	if layout, ok := rules["datetime"]; ok {
		if layout == "2006-01-02" {
			return "date"
		}
		return "date-time"
	}
	return ""
}
