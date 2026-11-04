package codegen

import (
	"fmt"
	"strings"

	"base-admin/server/internal/platform/database"
)

// mysqlSQL is the MySQL migration of a valid spec: one CREATE TABLE with
// the comments, indexes and foreign keys inline, so the statement is
// atomic (MySQL cannot roll back DDL) and follows migrations/mysql.
func (s TableSpec) mysqlSQL() (up, down string) {
	t := quoteIdent(s.Name)
	str := func(v string) string { return dialectString(database.MySQL, v) }
	cols := []string{"id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY"}
	var keys []string
	fk := func(column, ref string) {
		keys = append(keys, fmt.Sprintf("CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (id)",
			quoteIdent(s.Name+"_"+column+"_fk"), quoteIdent(column), quoteIdent(ref)))
	}
	for _, c := range s.Columns {
		cols = append(cols, c.definition(database.MySQL)+" COMMENT "+str(c.comment()))
		name := quoteIdent(c.Name)
		switch {
		case c.Unique && s.SoftDelete:
			// Unique among visible rows: deleted ones index NULL, which
			// does not collide (PostgreSQL: a partial unique index).
			keys = append(keys, fmt.Sprintf("UNIQUE KEY %s ((CASE WHEN deleted_at IS NULL THEN %s END))",
				quoteIdent(s.Name+"_"+c.Name+"_uq"), name))
		case c.Unique:
			keys = append(keys, fmt.Sprintf("UNIQUE KEY %s (%s)", quoteIdent(s.Name+"_"+c.Name+"_uq"), name))
		case c.Type == ColRef:
			keys = append(keys, fmt.Sprintf("KEY %s (%s)", quoteIdent(s.Name+"_"+c.Name+"_idx"), name))
		}
		if c.Type == ColRef {
			fk(c.Name, c.Ref)
		}
	}
	if s.Status {
		cols = append(cols, "status smallint NOT NULL DEFAULT 1 COMMENT '状态' CHECK (status IN (1, 2))")
	}
	if s.Sort {
		cols = append(cols, "sort integer NOT NULL DEFAULT 0 COMMENT '排序'")
	}
	if s.Dept {
		cols = append(cols, "dept_id bigint COMMENT '所属部门'")
		keys = append(keys, fmt.Sprintf("KEY %s (dept_id)", quoteIdent(s.Name+"_dept_id_idx")))
		fk("dept_id", "sys_departments")
	}
	cols = append(cols,
		"version integer NOT NULL DEFAULT 0",
		"created_at datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)",
		"updated_at datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)",
		"created_by bigint",
		"updated_by bigint")
	if s.SoftDelete {
		cols = append(cols, "deleted_at datetime(6)")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "-- %s：%s。由 AI 接口按 docs/table-guide.md 生成（MySQL）。\n\n", s.Name, s.Label)
	fmt.Fprintf(&b, "CREATE TABLE %s (\n    %s\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=%s COMMENT=%s;\n",
		t, strings.Join(append(cols, keys...), ",\n    "), database.MySQLCollation, str(strings.TrimSpace(s.Label)))
	return b.String(), fmt.Sprintf("DROP TABLE IF EXISTS %s;\n", t)
}
