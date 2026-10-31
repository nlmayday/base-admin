package database

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// TextArray is a PostgreSQL text[] parameter. GORM expands plain slices into
// "(?,?)" lists, so array parameters must implement driver.Valuer.
type TextArray []string

// Value renders the array literal with every element quoted.
func (a TextArray) Value() (driver.Value, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, s := range a {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		for _, r := range s {
			if r == '"' || r == '\\' {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String(), nil
}

// GormDataType tells GORM the column type, so the type can be used in
// scanned structs.
func (TextArray) GormDataType() string { return "text[]" }

// Scan implements sql.Scanner for the text form of a one-dimensional
// text[], e.g. {a,"b c"}.
func (a *TextArray) Scan(src any) error {
	var s string
	switch v := src.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	case nil:
		*a = nil
		return nil
	default:
		return fmt.Errorf("unsupported text[] source %T", src)
	}
	out, err := parseTextArray(s)
	if err != nil {
		return err
	}
	*a = out
	return nil
}

func parseTextArray(s string) ([]string, error) {
	if len(s) < 2 || s[0] != '{' || s[len(s)-1] != '}' {
		return nil, fmt.Errorf("malformed text[] %q", s)
	}
	body := s[1 : len(s)-1]
	out := []string{}
	if body == "" {
		return out, nil
	}
	var cur strings.Builder
	inQuotes, escaped := false, false
	for _, r := range body {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inQuotes = !inQuotes
		case r == ',' && !inQuotes:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	return append(out, cur.String()), nil
}
