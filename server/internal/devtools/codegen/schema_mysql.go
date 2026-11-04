package codegen

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"base-admin/server/internal/platform/database"
)

// MySQL tables are read from information_schema into the same Table as
// PostgreSQL ones: types are named the PostgreSQL way (mysqlUDT) and
// defaults are rewritten into SQL that parseDefault and the generated
// INSERT understand (mysqlDefault).

// mysqlTables lists the tables of the current database.
func mysqlTables(db *gorm.DB) ([]tableRow, error) {
	var rows []tableRow
	err := db.Raw(`SELECT TABLE_NAME AS name, TABLE_COMMENT AS comment FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE' ORDER BY TABLE_NAME`).Scan(&rows).Error
	return rows, err
}

type mysqlColumnRow struct {
	Name       string
	DataType   string
	ColumnType string
	MaxLength  int64
	Nullable   bool
	Def        *string
	Extra      string
	Comment    string
}

// introspectMySQL reads a table of the current database.
func introspectMySQL(db *gorm.DB, table string) (Table, error) {
	var meta []tableRow
	err := db.Raw(`SELECT TABLE_NAME AS name, TABLE_COMMENT AS comment FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND TABLE_TYPE = 'BASE TABLE'`, table).Scan(&meta).Error
	if err != nil {
		return Table{}, fmt.Errorf("read table %s: %w", table, err)
	}
	if len(meta) == 0 {
		return Table{}, fmt.Errorf("%w: %s", ErrTableNotFound, table)
	}
	cols, err := mysqlColumns(db, table)
	if err != nil {
		return Table{}, fmt.Errorf("read columns of %s: %w", table, err)
	}
	primary, unique, err := mysqlIndexColumns(db, table)
	if err != nil {
		return Table{}, fmt.Errorf("read indexes of %s: %w", table, err)
	}
	refs, err := mysqlForeignKeys(db, table)
	if err != nil {
		return Table{}, fmt.Errorf("read foreign keys of %s: %w", table, err)
	}
	t := Table{Name: table, Comment: meta[0].Comment}
	for _, c := range cols {
		udt := mysqlUDT(c.DataType, c.ColumnType)
		maxLen := 0
		if udt == "varchar" || udt == "bpchar" {
			maxLen = int(c.MaxLength)
		}
		extra := strings.ToLower(c.Extra)
		t.Columns = append(t.Columns, Column{
			Name: c.Name, UDT: udt, MaxLength: maxLen, Nullable: c.Nullable, Default: mysqlDefault(udt, c.Def, extra),
			Identity: strings.Contains(extra, "auto_increment"), Generated: strings.Contains(extra, " generated"),
			Comment: c.Comment, Primary: slices.Contains(primary, c.Name), Unique: slices.Contains(unique, c.Name),
			Ref: refs[c.Name],
		})
	}
	return t, nil
}

func mysqlColumns(db *gorm.DB, table string) ([]mysqlColumnRow, error) {
	var cols []mysqlColumnRow
	err := db.Raw(`SELECT COLUMN_NAME AS name, DATA_TYPE AS data_type, COLUMN_TYPE AS column_type,
			COALESCE(CHARACTER_MAXIMUM_LENGTH, 0) AS max_length, IS_NULLABLE = 'YES' AS nullable,
			COLUMN_DEFAULT AS def, EXTRA AS extra, COLUMN_COMMENT AS comment
		FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION`, table).Scan(&cols).Error
	return cols, err
}

// mysqlUDT names a MySQL column type the way PostgreSQL does, so that
// mapType applies; unsupported types keep their MySQL name.
func mysqlUDT(dataType, columnType string) string {
	switch dataType = strings.ToLower(dataType); dataType {
	case "bigint":
		return "int8"
	case "int", "integer", "mediumint":
		return "int4"
	case "smallint":
		return "int2"
	case "tinyint":
		if strings.HasPrefix(strings.ToLower(columnType), "tinyint(1)") {
			return "bool" // BOOLEAN
		}
		return "int2"
	case "float":
		return "float4"
	case "double", "real":
		return "float8"
	case "decimal", "numeric":
		return "numeric"
	case "varchar":
		return "varchar"
	case "char":
		return "bpchar"
	case "tinytext", "text", "mediumtext", "longtext":
		return "text"
	case "datetime", "timestamp":
		return "timestamptz"
	case "date":
		return "date"
	case "json":
		return "json"
	}
	return dataType
}

// mysqlCharsetLiteral is an expression default holding a string, which
// information_schema shows as _utf8mb4\'text\'.
var mysqlCharsetLiteral = regexp.MustCompile(`^_[a-z0-9]+\\'(.*)\\'$`)

// mysqlDefault rewrites a column default from information_schema as SQL:
// literal defaults are shown unquoted (abc, 0), expression defaults
// (DEFAULT_GENERATED) as their expression. Strings become quoted literals
// and booleans true/false, as PostgreSQL shows them.
func mysqlDefault(udt string, def *string, extra string) string {
	if def == nil {
		return ""
	}
	v := *def
	if strings.Contains(extra, "default_generated") {
		m := mysqlCharsetLiteral.FindStringSubmatch(v)
		switch {
		case strings.EqualFold(v, "json_array()"):
			return "'[]'" // the default of attachment lists
		case strings.EqualFold(v, "json_object()"):
			return "'{}'"
		case m == nil:
			return v // CURRENT_TIMESTAMP(6), now(6), ...
		}
		s, ok := unescapeMySQL(strings.NewReplacer(`\\`, `\`, `\'`, `'`).Replace(m[1]))
		if !ok {
			return v
		}
		return dialectString(database.MySQL, undoLatin1(s))
	}
	switch udt {
	case "bool":
		switch v {
		case "0":
			return "false"
		case "1":
			return "true"
		}
		return v
	case "int2", "int4", "int8", "float4", "float8", "numeric":
		return v
	}
	return dialectString(database.MySQL, v)
}

// undoLatin1 repairs text that went through a latin1 to UTF-8 conversion
// once too often: MySQL 8.4 shows the string of an expression default
// (DEFAULT ('无')) that way in information_schema, though SHOW CREATE
// TABLE and the stored default are right. Text that is not of that shape
// is returned unchanged.
func undoLatin1(s string) string {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 0xff {
			return s
		}
		b = append(b, byte(r)) //nolint:gosec // G115: r <= 0xff, checked above
	}
	if !utf8.Valid(b) {
		return s
	}
	return string(b)
}

// unescapeMySQL reads the body of a MySQL string literal written with
// backslash escapes and doubled quotes ('it\'s' or 'it”s').
func unescapeMySQL(s string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\' && i+1 < len(s):
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '0':
				b.WriteByte(0)
			default:
				b.WriteByte(s[i])
			}
		case c == '\'' && i+1 < len(s) && s[i+1] == '\'':
			i++
			b.WriteByte('\'')
		case c == '\'' || c == '\\':
			return "", false // an unescaped quote: not one literal
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), true
}

// mysqlVisibleUnique is the functional key part of a unique index among
// the visible rows of a soft-delete table, as information_schema shows
// (CASE WHEN deleted_at IS NULL THEN code END); group 1 is the column.
var mysqlVisibleUnique = regexp.MustCompile("(?i)^\\(case when \\(`deleted_at` is null\\) then `([a-z0-9_]+)` end\\)$")

// mysqlIndexColumns returns the primary key columns and the columns that
// carry a single-column unique index, also one among the visible rows of a
// soft-delete table (the counterpart of PostgreSQL's partial index). Other
// functional key parts have no column and are not per column.
func mysqlIndexColumns(db *gorm.DB, table string) (primary, unique []string, err error) {
	var rows []struct {
		IndexName  string
		Name       string
		Expression string
		Unique     bool
	}
	err = db.Raw(`SELECT INDEX_NAME AS index_name, COALESCE(COLUMN_NAME, '') AS name,
			COALESCE(EXPRESSION, '') AS expression, NON_UNIQUE = 0 AS "unique"
		FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
		ORDER BY INDEX_NAME, SEQ_IN_INDEX`, table).Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	parts := map[string][]string{}
	for _, r := range rows {
		if !r.Unique {
			continue
		}
		name := r.Name
		if m := mysqlVisibleUnique.FindStringSubmatch(r.Expression); name == "" && m != nil {
			name = m[1]
		}
		parts[r.IndexName] = append(parts[r.IndexName], name)
	}
	for name, cols := range parts {
		switch {
		case name == "PRIMARY":
			primary = append(primary, cols...)
		case len(cols) == 1 && cols[0] != "":
			unique = append(unique, cols[0])
		}
	}
	return primary, unique, nil
}

// mysqlForeignKeys returns the single-column foreign keys to "id" columns
// of tables in the current database, by column.
func mysqlForeignKeys(db *gorm.DB, table string) (map[string]*Ref, error) {
	var rows []struct {
		ConstraintName, Name, RefTable, RefColumn string
	}
	err := db.Raw(`SELECT CONSTRAINT_NAME AS constraint_name, COLUMN_NAME AS name,
			REFERENCED_TABLE_NAME AS ref_table, REFERENCED_COLUMN_NAME AS ref_column
		FROM information_schema.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND REFERENCED_TABLE_SCHEMA = DATABASE()`,
		table).Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	count := map[string]int{}
	for _, r := range rows {
		count[r.ConstraintName]++
	}
	out := map[string]*Ref{}
	for _, r := range rows {
		if count[r.ConstraintName] != 1 || r.RefColumn != "id" {
			continue
		}
		var cols []mysqlColumnRow
		if cols, err = mysqlColumns(db, r.RefTable); err != nil {
			return nil, err
		}
		refCols := make([]refColumn, len(cols))
		for i, c := range cols {
			refCols[i] = refColumn{Name: c.Name, UDT: mysqlUDT(c.DataType, c.ColumnType)}
		}
		out[r.Name] = refOf(r.RefTable, refCols)
	}
	return out, nil
}
