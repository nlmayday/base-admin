package codegen

import (
	"fmt"
	"slices"

	"gorm.io/gorm"
)

// postgresTables lists the tables of the current schema.
func postgresTables(db *gorm.DB) ([]tableRow, error) {
	var rows []tableRow
	err := db.Raw(`SELECT c.relname AS name, COALESCE(obj_description(c.oid, 'pg_class'), '') AS comment
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relkind IN ('r', 'p') AND NOT c.relispartition
		ORDER BY c.relname`).Scan(&rows).Error
	return rows, err
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

// introspectPostgres reads a table from the PostgreSQL catalog.
func introspectPostgres(db *gorm.DB, table string) (Table, error) {
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
	primary, unique, err := postgresIndexColumns(db, oid)
	if err != nil {
		return Table{}, fmt.Errorf("read indexes of %s: %w", table, err)
	}
	refs, err := postgresForeignKeys(db, oid)
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

// postgresForeignKeys returns the single-column foreign keys to "id" columns of
// tables in the current schema, by column.
func postgresForeignKeys(db *gorm.DB, oid int64) (map[string]*Ref, error) {
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
		var cols []refColumn
		err := db.Raw(`SELECT column_name AS name, udt_name AS udt FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? ORDER BY ordinal_position`, fk.RefTable).Scan(&cols).Error
		if err != nil {
			return nil, err
		}
		out[fk.Name] = refOf(fk.RefTable, cols)
	}
	return out, nil
}

// postgresIndexColumns returns the primary key columns and the columns that carry
// a single-column unique index, also one among the visible rows of a
// soft-delete table (WHERE deleted_at IS NULL); other partial indexes are
// not per column.
func postgresIndexColumns(db *gorm.DB, oid int64) (primary, unique []string, err error) {
	var rows []struct {
		Name      string
		IsPrimary bool
		Single    bool
	}
	err = db.Raw(`SELECT a.attname AS name, i.indisprimary AS is_primary, i.indnkeyatts = 1 AS single
		FROM pg_index i JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY (i.indkey)
		WHERE i.indrelid = ?::oid AND i.indisunique
			AND (i.indpred IS NULL OR pg_get_expr(i.indpred, i.indrelid) = '(deleted_at IS NULL)')`, oid).Scan(&rows).Error
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
