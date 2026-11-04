// Package codegen generates a CRUD module (backend and frontend) from a
// PostgreSQL or MySQL table, following the patterns of modules/iam and the
// system/position views (docs/p1-p3-plan.md, devtools).
package codegen

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"base-admin/server/internal/platform/database"

	"gorm.io/gorm"
)

// Column is one table column as read from the catalog.
type Column struct {
	Name string
	// UDT is the type in PostgreSQL's names (int8, varchar, timestamptz);
	// MySQL types are read under the same names (bigint is int8, datetime
	// is timestamptz) and unsupported ones keep their own (blob).
	UDT       string
	MaxLength int
	Nullable  bool
	Default   string
	Identity  bool
	Generated bool
	Comment   string
	Primary   bool
	Unique    bool
	// Ref is set when the column is a single-column foreign key to the id
	// of another table.
	Ref *Ref
}

// Ref is the table a foreign key refers to.
type Ref struct {
	Table string
	// Labels are the text columns that can name a row, best first
	// (name, title, ...); empty when there is none.
	Labels []string
	// SoftDelete: the table has deleted_at, so deleted rows are hidden.
	SoftDelete bool
}

// HasDefault reports whether inserts may omit the column.
func (c Column) HasDefault() bool { return c.Default != "" || c.Identity || c.Generated }

// Table is a table definition.
type Table struct {
	Name    string
	Comment string
	Columns []Column
}

// Column returns the named column.
func (t Table) Column(name string) (Column, bool) {
	i := slices.IndexFunc(t.Columns, func(c Column) bool { return c.Name == name })
	if i < 0 {
		return Column{}, false
	}
	return t.Columns[i], true
}

// ErrTableNotFound is returned for unknown tables.
var ErrTableNotFound = errors.New("table not found")

// TableInfo is one entry of ListTables.
type TableInfo struct {
	Name    string `json:"name"`
	Comment string `json:"comment"`
	// Supported is false when the table cannot be generated; Reason says
	// why in Chinese.
	Supported bool   `json:"supported"`
	Reason    string `json:"reason"`
	Columns   int    `json:"columns"`
}

// excludedTable reports tables owned by the core (sys_*) or the migrator.
func excludedTable(name string) bool {
	return strings.HasPrefix(name, "sys_") || name == "schema_migrations" || name == "app_schema_migrations"
}

// ListTables lists the generator candidates of the current schema.
func ListTables(ctx context.Context, db *gorm.DB) ([]TableInfo, error) {
	db = db.WithContext(ctx)
	var rows []tableRow
	var err error
	if database.IsMySQL(db) {
		rows, err = mysqlTables(db)
	} else {
		rows, err = postgresTables(db)
	}
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	out := make([]TableInfo, 0, len(rows))
	for _, r := range rows {
		if excludedTable(r.Name) {
			continue
		}
		t, err := Introspect(ctx, db, r.Name)
		if err != nil {
			return nil, err
		}
		info := TableInfo{Name: r.Name, Comment: r.Comment, Supported: true, Columns: len(t.Columns)}
		if err := CheckTable(t); err != nil {
			info.Supported, info.Reason = false, err.Error()
		}
		out = append(out, info)
	}
	return out, nil
}

// tableRow is a table of the current schema with its comment.
type tableRow struct {
	Name    string
	Comment string
}

// Introspect reads a table of the current schema.
func Introspect(ctx context.Context, db *gorm.DB, table string) (Table, error) {
	db = db.WithContext(ctx)
	if database.IsMySQL(db) {
		return introspectMySQL(db, table)
	}
	return introspectPostgres(db, table)
}

// labelPreference orders the columns that usually name a row.
var labelPreference = []string{"name", "title", "nickname", "username", "label", "code"}

// refColumn is a column of a referenced table.
type refColumn struct{ Name, UDT string }

// refOf describes a referenced table from its columns.
func refOf(table string, cols []refColumn) *Ref {
	ref := &Ref{Table: table}
	var preferred, others []string
	for _, c := range cols {
		switch {
		case c.Name == "deleted_at":
			ref.SoftDelete = true
		case (c.UDT != "varchar" && c.UDT != "text" && c.UDT != "bpchar") || sensitiveColumn(c.Name):
		case slices.Contains(labelPreference, c.Name):
			preferred = append(preferred, c.Name)
		default:
			others = append(others, c.Name)
		}
	}
	slices.SortFunc(preferred, func(a, b string) int {
		return slices.Index(labelPreference, a) - slices.Index(labelPreference, b)
	})
	ref.Labels = append(preferred, others...)
	return ref
}

// CheckTable verifies the generator's requirements: a bigint identity (or
// bigserial) primary key named id, and only supported required columns.
func CheckTable(t Table) error {
	if !identifierPattern.MatchString(t.Name) || reservedWords[t.Name] {
		return fmt.Errorf("表名 %s 须为小写字母、数字或下划线，且不能是 SQL 关键字", t.Name)
	}
	for _, c := range t.Columns {
		if !identifierPattern.MatchString(c.Name) {
			return fmt.Errorf("列名 %s 须为小写字母、数字或下划线", c.Name)
		}
	}
	id, ok := t.Column("id")
	if !ok || !id.Primary || id.UDT != "int8" {
		return errors.New("表必须以 bigint 类型的 id 列为主键")
	}
	pkCols := 0
	for _, c := range t.Columns {
		if c.Primary {
			pkCols++
		}
	}
	if pkCols != 1 {
		return errors.New("不支持复合主键")
	}
	if !id.Identity && !strings.HasPrefix(id.Default, "nextval(") {
		return errors.New("id 列必须是 GENERATED ... AS IDENTITY（或 bigserial）")
	}
	for _, c := range t.Columns {
		if _, ok := mapType(c); !ok && !c.Nullable && !c.HasDefault() {
			return fmt.Errorf("列 %s 的类型 %s 不受支持，且为必填列", c.Name, c.UDT)
		}
	}
	return nil
}
