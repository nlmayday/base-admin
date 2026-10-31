// Package codegen generates a CRUD module (backend and frontend) from a
// PostgreSQL table, following the patterns of modules/iam and the
// system/position views (docs/p1-p3-plan.md, devtools).
package codegen

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"gorm.io/gorm"
)

// Column is one table column as read from the catalog.
type Column struct {
	Name string
	// UDT is the PostgreSQL type name, e.g. int8, varchar, timestamptz.
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
	return strings.HasPrefix(name, "sys_") || name == "schema_migrations"
}

// ListTables lists the generator candidates of the current schema.
func ListTables(ctx context.Context, db *gorm.DB) ([]TableInfo, error) {
	var rows []struct {
		Name    string
		Comment string
	}
	err := db.WithContext(ctx).Raw(`SELECT c.relname AS name, COALESCE(obj_description(c.oid, 'pg_class'), '') AS comment
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relkind IN ('r', 'p') AND NOT c.relispartition
		ORDER BY c.relname`).Scan(&rows).Error
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

type columnRow struct {
	Name      string
	UDT       string
	MaxLength int
	Nullable  bool
	Def       string
	Identity  bool
	Generated bool
	Comment   string
}

// Introspect reads a table of the current schema.
func Introspect(ctx context.Context, db *gorm.DB, table string) (Table, error) {
	db = db.WithContext(ctx)
	var meta []struct {
		RelID   int64
		Comment string
	}
	err := db.Raw(`SELECT c.oid::bigint AS rel_id, COALESCE(obj_description(c.oid, 'pg_class'), '') AS comment
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relname = ? AND c.relkind IN ('r', 'p')`, table).Scan(&meta).Error
	if err != nil {
		return Table{}, fmt.Errorf("read table %s: %w", table, err)
	}
	if len(meta) == 0 {
		return Table{}, fmt.Errorf("%w: %s", ErrTableNotFound, table)
	}
	oid := meta[0].RelID
	var cols []columnRow
	err = db.Raw(`SELECT c.column_name AS name, c.udt_name AS udt,
			COALESCE(c.character_maximum_length, 0) AS max_length,
			c.is_nullable = 'YES' AS nullable, COALESCE(c.column_default, '') AS def,
			c.is_identity = 'YES' AS identity, c.is_generated = 'ALWAYS' AS generated,
			COALESCE(col_description(?::oid, c.ordinal_position::int), '') AS comment
		FROM information_schema.columns c
		WHERE c.table_schema = current_schema() AND c.table_name = ?
		ORDER BY c.ordinal_position`, oid, table).Scan(&cols).Error
	if err != nil {
		return Table{}, fmt.Errorf("read columns of %s: %w", table, err)
	}
	primary, unique, err := indexColumns(db, oid)
	if err != nil {
		return Table{}, fmt.Errorf("read indexes of %s: %w", table, err)
	}
	refs, err := foreignKeys(db, oid)
	if err != nil {
		return Table{}, fmt.Errorf("read foreign keys of %s: %w", table, err)
	}
	t := Table{Name: table, Comment: meta[0].Comment}
	for _, c := range cols {
		t.Columns = append(t.Columns, Column{
			Name: c.Name, UDT: c.UDT, MaxLength: c.MaxLength, Nullable: c.Nullable, Default: c.Def,
			Identity: c.Identity, Generated: c.Generated, Comment: c.Comment,
			Primary: slices.Contains(primary, c.Name), Unique: slices.Contains(unique, c.Name),
			Ref: refs[c.Name],
		})
	}
	return t, nil
}

// labelPreference orders the columns that usually name a row.
var labelPreference = []string{"name", "title", "nickname", "username", "label", "code"}

// foreignKeys returns the single-column foreign keys to "id" columns of
// tables in the current schema, by column.
func foreignKeys(db *gorm.DB, oid int64) (map[string]*Ref, error) {
	var fks []struct{ Name, RefTable string }
	err := db.Raw(`SELECT a.attname AS name, rc.relname AS ref_table
		FROM pg_constraint con
		JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = con.conkey[1]
		JOIN pg_class rc ON rc.oid = con.confrelid
		JOIN pg_namespace n ON n.oid = rc.relnamespace AND n.nspname = current_schema()
		JOIN pg_attribute ra ON ra.attrelid = con.confrelid AND ra.attnum = con.confkey[1]
		WHERE con.conrelid = ?::oid AND con.contype = 'f' AND cardinality(con.conkey) = 1 AND ra.attname = 'id'`,
		oid).Scan(&fks).Error
	if err != nil || len(fks) == 0 {
		return nil, err
	}
	out := map[string]*Ref{}
	for _, fk := range fks {
		var cols []struct{ Name, UDT string }
		err := db.Raw(`SELECT column_name AS name, udt_name AS udt FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? ORDER BY ordinal_position`, fk.RefTable).Scan(&cols).Error
		if err != nil {
			return nil, err
		}
		out[fk.Name] = refOf(fk.RefTable, cols)
	}
	return out, nil
}

func refOf(table string, cols []struct{ Name, UDT string }) *Ref {
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

// indexColumns returns the primary key columns and the columns that carry
// a single-column unique index.
func indexColumns(db *gorm.DB, oid int64) (primary, unique []string, err error) {
	var rows []struct {
		Name      string
		IsPrimary bool
		Single    bool
	}
	err = db.Raw(`SELECT a.attname AS name, i.indisprimary AS is_primary, i.indnkeyatts = 1 AS single
		FROM pg_index i JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY (i.indkey)
		WHERE i.indrelid = ?::oid AND i.indisunique AND i.indpred IS NULL`, oid).Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	for _, r := range rows {
		switch {
		case r.IsPrimary:
			primary = append(primary, r.Name)
		case r.Single:
			unique = append(unique, r.Name)
		}
	}
	return primary, unique, nil
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
