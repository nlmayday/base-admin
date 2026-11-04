package database

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TextArray is a list of strings stored in one column: text[] on
// PostgreSQL, a JSON array on MySQL. GORM expands plain slices into "(?,?)"
// lists, so list parameters must implement driver.Valuer.
type TextArray []string

// GormValue encodes the list for the database of the statement.
func (a TextArray) GormValue(_ context.Context, db *gorm.DB) clause.Expr {
	if IsMySQL(db) {
		list := a
		if list == nil {
			list = TextArray{}
		}
		b, err := json.Marshal([]string(list))
		if err != nil {
			_ = db.AddError(fmt.Errorf("encode text list: %w", err))
		}
		return clause.Expr{SQL: "?", Vars: []any{string(b)}}
	}
	v, _ := a.Value()
	return clause.Expr{SQL: "?", Vars: []any{v}}
}

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
// text[], e.g. {a,"b c"}, and for a JSON array of strings (MySQL).
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
	if strings.HasPrefix(s, "[") {
		var out []string
		if err := json.Unmarshal([]byte(s), &out); err != nil {
			return fmt.Errorf("malformed json string list: %w", err)
		}
		if out == nil {
			out = []string{}
		}
		*a = out
		return nil
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
