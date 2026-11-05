//go:build integration && parity

package dbmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/testinfra"
)

// TestSchemaParity migrates a PostgreSQL and a MySQL database to the latest
// version and compares what the migrations built, not only their file
// names: tables, columns (type, nullability, auto keys), indexes
// (uniqueness and columns), foreign keys (target and ON DELETE) and the
// number of CHECK constraints. Normalized away, by design: PostgreSQL
// arrays are JSON arrays on MySQL; MySQL has no partial indexes (the WHERE
// is dropped); MySQL adds an index for every foreign key without one.
// Other intended differences are listed in parityExceptions with the reason.
//
//	ADMIN_TEST_PG_URL=postgres://… ADMIN_TEST_MYSQL_URL=mysql://… \
//	  go test -tags 'integration parity' -run TestSchemaParity ./internal/platform/dbmigrate/
func TestSchemaParity(t *testing.T) {
	pg := migratedSchema(t, requireParityEnv(t, "ADMIN_TEST_PG_URL"))
	my := migratedSchema(t, requireParityEnv(t, "ADMIN_TEST_MYSQL_URL"))
	require.NotEmpty(t, pg, "no tables on PostgreSQL")

	var diffs []string
	for _, line := range diffLines(pg, my) {
		if _, ok := parityExceptions[line]; !ok {
			diffs = append(diffs, line)
		}
	}
	assert.Empty(t, diffs, "schema differences (- PostgreSQL only, + MySQL only):\n%s", strings.Join(diffs, "\n"))

	// An exception that no longer matches anything is stale.
	all := map[string]bool{}
	for _, line := range diffLines(pg, my) {
		all[line] = true
	}
	for line := range parityExceptions {
		assert.True(t, all[line], "stale parity exception: %s", line)
	}
}

func requireParityEnv(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Fatalf("%s is not set; run `make test-schema-parity`", key)
	}
	return v
}

// schema is one line per fact, keyed by table: "table column name type …".
type schema map[string][]string

func migratedSchema(t *testing.T, adminURL string) schema {
	t.Helper()
	cfg := testinfra.DatabaseAt(t, adminURL)
	ctx := context.Background()
	for _, track := range TracksFor(cfg) {
		db, err := database.OpenSQL(ctx, cfg, "parity-migrate", database.ForMigrations())
		require.NoError(t, err)
		runner, err := New(db, track)
		require.NoError(t, err)
		require.NoError(t, runner.Up(), track.Name)
		require.NoError(t, runner.Close())
	}
	db, err := database.OpenSQL(ctx, cfg, "parity-read")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	versionTables := map[string]bool{cfg.MigrationsTable: true, cfg.AppMigrationsTable: true}
	s := schema{}
	add := func(table, fact string) {
		if !versionTables[table] {
			s[table] = append(s[table], fact)
		}
	}
	if database.SQLDialect(db) == database.MySQL {
		readMySQL(t, db, add)
	} else {
		readPostgres(t, db, add)
	}
	return s
}

func diffLines(pg, my schema) []string {
	tables := map[string]bool{}
	for k := range pg {
		tables[k] = true
	}
	for k := range my {
		tables[k] = true
	}
	var out []string
	for table := range tables {
		a, b := toSet(pg[table]), toSet(my[table])
		for f := range a {
			if !b[f] {
				out = append(out, "- "+table+" "+f)
			}
		}
		for f := range b {
			if !a[f] {
				out = append(out, "+ "+table+" "+f)
			}
		}
	}
	sort.Strings(out)
	return out
}

func toSet(facts []string) map[string]bool {
	m := make(map[string]bool, len(facts))
	for _, f := range facts {
		m[f] = true
	}
	return m
}

func rows(t *testing.T, db *sql.DB, query string, scan func(*sql.Rows) error) {
	t.Helper()
	r, err := db.Query(query)
	require.NoError(t, err)
	defer r.Close()
	for r.Next() {
		require.NoError(t, scan(r))
	}
	require.NoError(t, r.Err())
}

// ---- PostgreSQL ----

func readPostgres(t *testing.T, db *sql.DB, add func(table, fact string)) {
	t.Helper()
	rows(t, db, `SELECT c.table_name, c.column_name, c.is_nullable = 'YES', c.data_type, c.udt_name,
			coalesce(c.character_maximum_length, 0), coalesce(c.numeric_precision, 0), coalesce(c.numeric_scale, 0),
			c.is_identity = 'YES' OR coalesce(c.column_default, '') LIKE 'nextval(%'
		FROM information_schema.columns c
		JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name
		WHERE c.table_schema = current_schema() AND t.table_type = 'BASE TABLE'`,
		func(r *sql.Rows) error {
			var table, col, dataType, udt string
			var nullable, auto bool
			var length, precision, scale int
			if err := r.Scan(&table, &col, &nullable, &dataType, &udt, &length, &precision, &scale, &auto); err != nil {
				return err
			}
			add(table, columnFact(col, pgType(dataType, udt, length, precision, scale), nullable, auto))
			return nil
		})
	rows(t, db, `SELECT t.relname, ix.indisunique,
			array_to_string(array(SELECT CASE WHEN k = 0 THEN 'expr' ELSE a.attname END
				FROM unnest(ix.indkey) WITH ORDINALITY AS u(k, n)
				LEFT JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = u.k ORDER BY u.n), ','),
			''
		FROM pg_index ix
		JOIN pg_class t ON t.oid = ix.indrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE n.nspname = current_schema() AND t.relkind = 'r'`,
		func(r *sql.Rows) error {
			var table, cols, unused string
			var unique bool
			if err := r.Scan(&table, &unique, &cols, &unused); err != nil {
				return err
			}
			add(table, indexFact(unique, cols))
			return nil
		})
	rows(t, db, `SELECT kcu.table_name, kcu.column_name, ccu.table_name, ccu.column_name, rc.delete_rule
		FROM information_schema.referential_constraints rc
		JOIN information_schema.key_column_usage kcu
			ON kcu.constraint_schema = rc.constraint_schema AND kcu.constraint_name = rc.constraint_name
		JOIN information_schema.constraint_column_usage ccu
			ON ccu.constraint_schema = rc.unique_constraint_schema AND ccu.constraint_name = rc.unique_constraint_name
		WHERE rc.constraint_schema = current_schema()`,
		func(r *sql.Rows) error {
			var table, col, refTable, refCol, rule string
			if err := r.Scan(&table, &col, &refTable, &refCol, &rule); err != nil {
				return err
			}
			add(table, fkFact(col, refTable, refCol, rule))
			return nil
		})
	countChecks(t, db, add, `SELECT tc.table_name, count(*) FROM information_schema.table_constraints tc
		JOIN information_schema.check_constraints cc
			ON cc.constraint_schema = tc.constraint_schema AND cc.constraint_name = tc.constraint_name
		WHERE tc.table_schema = current_schema() AND tc.constraint_type = 'CHECK'
			AND cc.check_clause NOT LIKE '% IS NOT NULL'
		GROUP BY tc.table_name`)
}

func pgType(dataType, udt string, length, precision, scale int) string {
	switch {
	case dataType == "ARRAY":
		return "json" // a JSON array on MySQL (database.Array)
	case udt == "int2":
		return "int16"
	case udt == "int4":
		return "int32"
	case udt == "int8":
		return "int64"
	case udt == "bool":
		return "bool"
	case udt == "text":
		return "text"
	case udt == "varchar":
		return fmt.Sprintf("varchar(%d)", length)
	case udt == "bpchar":
		return fmt.Sprintf("char(%d)", length)
	case udt == "timestamptz", udt == "timestamp":
		return "timestamp"
	case udt == "date":
		return "date"
	case udt == "jsonb", udt == "json":
		return "json"
	case udt == "numeric":
		return fmt.Sprintf("numeric(%d,%d)", precision, scale)
	case udt == "bytea":
		return "bytes"
	}
	return udt
}

// ---- MySQL ----

func readMySQL(t *testing.T, db *sql.DB, add func(table, fact string)) {
	t.Helper()
	rows(t, db, `SELECT c.table_name, c.column_name, c.is_nullable = 'YES', c.data_type, c.column_type,
			coalesce(c.character_maximum_length, 0), coalesce(c.numeric_precision, 0), coalesce(c.numeric_scale, 0),
			c.extra LIKE '%auto_increment%'
		FROM information_schema.columns c
		JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name
		WHERE c.table_schema = DATABASE() AND t.table_type = 'BASE TABLE'`,
		func(r *sql.Rows) error {
			var table, col, dataType, columnType string
			var nullable, auto bool
			var length, precision, scale int
			if err := r.Scan(&table, &col, &nullable, &dataType, &columnType, &length, &precision, &scale, &auto); err != nil {
				return err
			}
			add(table, columnFact(col, mysqlType(dataType, columnType, length, precision, scale), nullable, auto))
			return nil
		})
	rows(t, db, `SELECT table_name, non_unique = 0,
			group_concat(CASE WHEN column_name IS NULL THEN 'expr' ELSE column_name END ORDER BY seq_in_index SEPARATOR ',')
		FROM information_schema.statistics
		WHERE table_schema = DATABASE()
			-- the index MySQL creates for a foreign key is named after it
			AND index_name NOT IN (SELECT constraint_name FROM information_schema.referential_constraints rc
				WHERE rc.constraint_schema = DATABASE() AND rc.table_name = statistics.table_name)
		GROUP BY table_name, index_name, non_unique`,
		func(r *sql.Rows) error {
			var table, cols string
			var unique bool
			if err := r.Scan(&table, &unique, &cols); err != nil {
				return err
			}
			add(table, indexFact(unique, cols))
			return nil
		})
	rows(t, db, `SELECT kcu.table_name, kcu.column_name, kcu.referenced_table_name, kcu.referenced_column_name, rc.delete_rule
		FROM information_schema.referential_constraints rc
		JOIN information_schema.key_column_usage kcu
			ON kcu.constraint_schema = rc.constraint_schema AND kcu.constraint_name = rc.constraint_name
			AND kcu.table_name = rc.table_name
		WHERE rc.constraint_schema = DATABASE()`,
		func(r *sql.Rows) error {
			var table, col, refTable, refCol, rule string
			if err := r.Scan(&table, &col, &refTable, &refCol, &rule); err != nil {
				return err
			}
			add(table, fkFact(col, refTable, refCol, rule))
			return nil
		})
	countChecks(t, db, add, `SELECT table_name, count(*) FROM information_schema.table_constraints
		WHERE table_schema = DATABASE() AND constraint_type = 'CHECK'
		GROUP BY table_name`)
}

func mysqlType(dataType, columnType string, length, precision, scale int) string {
	switch dataType {
	case "tinyint":
		if columnType == "tinyint(1)" {
			return "bool"
		}
		return "int16"
	case "smallint":
		return "int16"
	case "int", "mediumint":
		return "int32"
	case "bigint":
		return "int64"
	case "text", "mediumtext", "longtext":
		return "text"
	case "varchar":
		return fmt.Sprintf("varchar(%d)", length)
	case "char":
		return fmt.Sprintf("char(%d)", length)
	case "datetime", "timestamp":
		return "timestamp"
	case "date":
		return "date"
	case "json":
		return "json"
	case "decimal":
		return fmt.Sprintf("numeric(%d,%d)", precision, scale)
	case "blob", "mediumblob", "longblob", "varbinary", "binary":
		return "bytes"
	}
	return columnType
}

// ---- facts ----

func columnFact(col, typ string, nullable, auto bool) string {
	f := "column " + col + " " + typ
	if nullable {
		f += " null"
	}
	if auto {
		f += " auto"
	}
	return f
}

func indexFact(unique bool, cols string) string {
	if unique {
		return "unique index (" + cols + ")"
	}
	return "index (" + cols + ")"
}

func fkFact(col, refTable, refCol, rule string) string {
	return fmt.Sprintf("fk %s -> %s(%s) on delete %s", col, refTable, refCol, strings.ToLower(rule))
}

func countChecks(t *testing.T, db *sql.DB, add func(table, fact string), query string) {
	t.Helper()
	rows(t, db, query, func(r *sql.Rows) error {
		var table string
		var n int
		if err := r.Scan(&table, &n); err != nil {
			return err
		}
		add(table, fmt.Sprintf("checks %d", n))
		return nil
	})
}

// parityExceptions are the intended differences, as diffLines prints
// them, each with the reason.
var parityExceptions = map[string]string{
	// No partial indexes on MySQL: the WHERE condition becomes a leading
	// or trailing column.
	"- sys_job_runs index (expires_at)":           "PostgreSQL: WHERE status = 'running'",
	"+ sys_job_runs index (status,expires_at)":    "MySQL: status leads instead",
	"- sys_user_messages index (user_id)":         "PostgreSQL: WHERE read_at IS NULL",
	"+ sys_user_messages index (user_id,read_at)": "MySQL: read_at follows instead",
	// database.LockTx: pg_advisory_xact_lock on PostgreSQL, a locked row
	// here, released at COMMIT or ROLLBACK like the advisory lock.
	"+ sys_tx_locks column name varchar(128)": "MySQL-only lock rows",
	"+ sys_tx_locks unique index (name)":      "MySQL-only lock rows",
	// MySQL indexes text only by prefix; 768 characters fit the key limit.
	"- sys_departments column path text":         "materialized path",
	"+ sys_departments column path varchar(768)": "materialized path, indexed",
	// MySQL forbids CHECK on an AUTO_INCREMENT column: no CHECK (id <> parent_id).
	"- sys_departments checks 2": "PostgreSQL: status and id <> parent_id",
	"+ sys_departments checks 1": "MySQL: status",
	"- sys_menus checks 4":       "PostgreSQL: type, source, menu permission, id <> parent_id",
	"+ sys_menus checks 3":       "MySQL: without id <> parent_id",
	// text[] becomes json with CHECK (JSON_TYPE(requires) = 'ARRAY').
	"- sys_permissions checks 1": "PostgreSQL: status",
	"+ sys_permissions checks 2": "MySQL: status and the JSON array check",
}
