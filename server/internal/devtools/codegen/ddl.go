package codegen

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"base-admin/server/internal/platform/database"
)

// Table creation for the AI agent API: a structured description of the
// business columns becomes a migration that follows docs/table-guide.md
// (identity id, standard columns, comments, unique indexes, foreign keys).
// No SQL is taken from the caller.

// Column types of a TableSpec.
const (
	ColString      = "string"      // varchar(length), length 1–2000, default 64
	ColText        = "text"        // text
	ColInt         = "int"         // integer
	ColBigint      = "bigint"      // bigint
	ColDecimal     = "decimal"     // numeric(precision, scale), default 12, 2
	ColBool        = "bool"        // boolean NOT NULL DEFAULT false
	ColDate        = "date"        // date
	ColDatetime    = "datetime"    // timestamptz
	ColJSON        = "json"        // jsonb
	ColImage       = "image"       // varchar(512): an uploaded image URL
	ColAttachments = "attachments" // jsonb list of uploaded files
	ColRef         = "ref"         // bigint REFERENCES <ref> (id)
)

var columnTypes = []string{ColString, ColText, ColInt, ColBigint, ColDecimal, ColBool, ColDate,
	ColDatetime, ColJSON, ColImage, ColAttachments, ColRef}

// TableSpec describes a business table. The standard columns (id, version,
// created_at, updated_at, created_by, updated_by) are always added.
type TableSpec struct {
	// Name is the table name, e.g. biz_orders (not sys_*).
	Name string `json:"name"`
	// Label is the Chinese table name, e.g. 订单.
	Label   string       `json:"label"`
	Columns []ColumnSpec `json:"columns"`
	// Status adds status (1 启用 / 2 停用); Sort adds sort; Dept adds dept_id
	// (department data scope); SoftDelete adds deleted_at.
	Status     bool `json:"status,omitempty"`
	Sort       bool `json:"sort,omitempty"`
	Dept       bool `json:"dept,omitempty"`
	SoftDelete bool `json:"softDelete,omitempty"`
}

// ColumnSpec describes one business column.
type ColumnSpec struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// Label is the Chinese field name, stored as the column comment.
	Label string `json:"label"`
	// Required makes the column NOT NULL (the form requires it).
	Required bool `json:"required,omitempty"`
	// Default is a literal default: a number, true/false, YYYY-MM-DD, a
	// string, or "now" for datetime.
	Default string `json:"default,omitempty"`
	Unique  bool   `json:"unique,omitempty"`
	// Length (string), Precision and Scale (decimal).
	Length    int `json:"length,omitempty"`
	Precision int `json:"precision,omitempty"`
	Scale     int `json:"scale,omitempty"`
	// Ref is the referenced table of a ref column (its id).
	Ref string `json:"ref,omitempty"`
	// Options are fixed choices (string/int/bigint): a select in the pages
	// and a CHECK constraint.
	Options []EnumOption `json:"options,omitempty"`
}

const (
	maxTableName  = 30
	maxColumnName = 28 // index names <table>_<column>_uq fit in 63 bytes
	maxColumns    = 60
	maxLabel      = 20
	maxString     = 2000
	// maxUniqueString is the longest indexable varchar of MySQL (utf8mb4).
	maxUniqueString = 768
)

var (
	ddlIdentPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	numberPattern   = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)
	integerPattern  = regexp.MustCompile(`^-?[0-9]+$`)
	datePattern     = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	optionPattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
)

// standardColumnNames are added by the generator; use the options instead.
var standardColumnNames = []string{"id", "version", "created_at", "updated_at", "created_by",
	"updated_by", "deleted_at", "status", "sort", "dept_id"}

// Problems checks the spec; field names are JSON paths such as
// "columns[2].default".
func (s TableSpec) Problems() []Problem {
	var out []Problem
	add := func(field, msg string) { out = append(out, Problem{Field: field, Message: msg}) }
	switch {
	case !ddlIdentPattern.MatchString(s.Name) || len(s.Name) > maxTableName:
		add("name", fmt.Sprintf("表名须为小写字母开头的小写字母、数字或下划线，最长 %d 个字符", maxTableName))
	case strings.HasPrefix(s.Name, "sys_"):
		add("name", "sys_ 前缀留给系统表，业务表请用 biz_ 或模块名做前缀")
	case reservedWords[s.Name]:
		add("name", "表名不能是 SQL 关键字")
	}
	if p := labelProblem(s.Label); p != "" {
		add("label", "表的中文名"+p)
	}
	if len(s.Columns) == 0 || len(s.Columns) > maxColumns {
		add("columns", fmt.Sprintf("需要 1–%d 个业务字段", maxColumns))
	}
	seen := map[string]bool{}
	for i, c := range s.Columns {
		path := fmt.Sprintf("columns[%d]", i)
		if seen[c.Name] {
			add(path+".name", "字段 "+c.Name+" 重复")
		}
		seen[c.Name] = true
		for _, p := range c.problems() {
			add(path+"."+p.Field, p.Message)
		}
	}
	return out
}

func labelProblem(label string) string {
	l := strings.TrimSpace(label)
	switch {
	case l == "":
		return "不能为空"
	case utf8.RuneCountInString(l) > maxLabel:
		return fmt.Sprintf("最长 %d 个字符", maxLabel)
	case strings.ContainsAny(l, unsafeText+",，=：:（）()"):
		return "不能包含引号、反斜杠、尖括号、花括号、冒号、括号、逗号或等号"
	}
	return ""
}

func (c ColumnSpec) problems() []Problem {
	var out []Problem
	add := func(field, msg string) { out = append(out, Problem{Field: field, Message: msg}) }
	switch {
	case !ddlIdentPattern.MatchString(c.Name) || len(c.Name) > maxColumnName:
		add("name", fmt.Sprintf("字段名须为小写字母开头的小写字母、数字或下划线，最长 %d 个字符", maxColumnName))
	case slices.Contains(standardColumnNames, c.Name):
		add("name", c.Name+" 是标准字段，由生成器添加（status、sort、dept_id、deleted_at 用表级选项开启）")
	case reservedWords[c.Name]:
		add("name", "字段名不能是 SQL 关键字")
	}
	if p := labelProblem(c.Label); p != "" {
		add("label", "中文名"+p)
	}
	if !slices.Contains(columnTypes, c.Type) {
		add("type", "类型须为 "+strings.Join(columnTypes, "、")+" 之一")
		return out
	}
	switch c.Type {
	case ColString:
		if c.Length < 0 || c.Length > maxString {
			add("length", fmt.Sprintf("长度须为 1–%d", maxString))
		}
	case ColDecimal:
		p, sc := c.decimal()
		if p < 1 || p > 38 || sc < 0 || sc > p {
			add("precision", "精度须为 1–38，小数位须为 0 到精度之间")
		}
	case ColRef:
		if !ddlIdentPattern.MatchString(c.Ref) || reservedWords[c.Ref] {
			add("ref", "ref 字段须用 ref 指定被引用的表名")
		}
		if !strings.HasSuffix(c.Name, "_id") {
			add("name", "关联字段名须以 _id 结尾，如 customer_id")
		}
	}
	if c.Ref != "" && c.Type != ColRef {
		add("ref", "只有 ref 类型可以指定 ref")
	}
	if c.Unique && !slices.Contains([]string{ColString, ColInt, ColBigint, ColDate, ColRef}, c.Type) {
		add("unique", "只有 string、int、bigint、date、ref 字段可以唯一")
	}
	if c.Unique && c.Type == ColString && c.length() > maxUniqueString {
		add("length", fmt.Sprintf("唯一字段长度最长 %d（MySQL 索引的上限）", maxUniqueString))
	}
	out = append(out, c.optionProblems()...)
	if c.Default != "" {
		if msg := c.defaultProblem(); msg != "" {
			add("default", msg)
		}
	}
	return out
}

func (c ColumnSpec) optionProblems() []Problem {
	if len(c.Options) == 0 {
		return nil
	}
	var out []Problem
	if !slices.Contains([]string{ColString, ColInt, ColBigint}, c.Type) {
		return []Problem{{Field: "options", Message: "只有 string、int、bigint 字段可以有选项"}}
	}
	if len(c.Options) < 2 {
		out = append(out, Problem{Field: "options", Message: "选项至少 2 个"})
	}
	values := map[string]bool{}
	for i, o := range c.Options {
		path := fmt.Sprintf("options[%d]", i)
		switch {
		case !optionPattern.MatchString(o.Value):
			out = append(out, Problem{Field: path + ".value", Message: "选项值须为 1–32 位字母、数字、_ 或 -"})
		case c.Type != ColString && !integerPattern.MatchString(o.Value):
			out = append(out, Problem{Field: path + ".value", Message: "整数字段的选项值须为整数"})
		case values[o.Value]:
			out = append(out, Problem{Field: path + ".value", Message: "选项值重复"})
		case c.Type == ColString && len(o.Value) > c.length():
			out = append(out, Problem{Field: path + ".value", Message: "选项值超过字段长度"})
		}
		values[o.Value] = true
		if p := labelProblem(o.Label); p != "" {
			out = append(out, Problem{Field: path + ".label", Message: "选项文字" + p})
		}
	}
	if c.Default != "" && !values[c.Default] {
		out = append(out, Problem{Field: "default", Message: "默认值须为选项之一"})
	}
	return out
}

func (c ColumnSpec) defaultProblem() string {
	d := c.Default
	switch c.Type {
	case ColInt, ColBigint:
		if !integerPattern.MatchString(d) {
			return "默认值须为整数"
		}
	case ColDecimal:
		if !numberPattern.MatchString(d) {
			return "默认值须为数字"
		}
	case ColBool:
		if d != "true" && d != "false" {
			return "默认值须为 true 或 false"
		}
	case ColDate:
		if !datePattern.MatchString(d) {
			return "默认值须为 YYYY-MM-DD"
		}
	case ColDatetime:
		if d != "now" {
			return "时间字段的默认值只能是 now"
		}
	case ColString, ColText:
		if c.Type == ColString && utf8.RuneCountInString(d) > c.length() {
			return "默认值超过字段长度"
		}
	default:
		return c.Type + " 字段不能设置默认值"
	}
	return ""
}

func (c ColumnSpec) length() int {
	if c.Length == 0 {
		return 64
	}
	return c.Length
}

func (c ColumnSpec) decimal() (int, int) {
	p, s := c.Precision, c.Scale
	if p == 0 {
		p, s = 12, 2
	}
	return p, s
}

// MigrationSQL returns the up and down migrations of a valid spec for the
// database dialect d.
func (s TableSpec) MigrationSQL(d database.Dialect) (up, down string) {
	if d == database.MySQL {
		return s.mysqlSQL()
	}
	return s.postgresSQL()
}

// postgresSQL is the PostgreSQL migration of a valid spec.
func (s TableSpec) postgresSQL() (up, down string) {
	t := quoteIdent(s.Name)
	var cols, comments, indexes []string
	cols = append(cols, "id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY")
	comment := func(column, text string) {
		comments = append(comments, fmt.Sprintf("COMMENT ON COLUMN %s.%s IS %s;", t, quoteIdent(column), sqlString(text)))
	}
	for _, c := range s.Columns {
		cols = append(cols, c.definition(database.Postgres))
		comment(c.Name, c.comment())
		switch {
		case c.Unique:
			idx := fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (%s)", quoteIdent(s.Name+"_"+c.Name+"_uq"), t, quoteIdent(c.Name))
			if s.SoftDelete {
				idx += " WHERE deleted_at IS NULL"
			}
			indexes = append(indexes, idx+";")
		case c.Type == ColRef:
			indexes = append(indexes, fmt.Sprintf("CREATE INDEX %s ON %s (%s);", quoteIdent(s.Name+"_"+c.Name+"_idx"), t, quoteIdent(c.Name)))
		}
	}
	if s.Status {
		cols = append(cols, "status smallint NOT NULL DEFAULT 1 CHECK (status IN (1, 2))")
		comment("status", "状态")
	}
	if s.Sort {
		cols = append(cols, "sort integer NOT NULL DEFAULT 0")
		comment("sort", "排序")
	}
	if s.Dept {
		cols = append(cols, "dept_id bigint REFERENCES sys_departments (id)")
		comment("dept_id", "所属部门")
		indexes = append(indexes, fmt.Sprintf("CREATE INDEX %s ON %s (dept_id);", quoteIdent(s.Name+"_dept_id_idx"), t))
	}
	cols = append(cols,
		"version integer NOT NULL DEFAULT 0",
		"created_at timestamptz NOT NULL DEFAULT now()",
		"updated_at timestamptz NOT NULL DEFAULT now()",
		"created_by bigint",
		"updated_by bigint")
	if s.SoftDelete {
		cols = append(cols, "deleted_at timestamptz")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "-- %s：%s。由 AI 接口按 docs/table-guide.md 生成。\n\n", s.Name, s.Label)
	fmt.Fprintf(&b, "CREATE TABLE %s (\n    %s\n);\n\n", t, strings.Join(cols, ",\n    "))
	fmt.Fprintf(&b, "COMMENT ON TABLE %s IS %s;\n", t, sqlString(strings.TrimSpace(s.Label)))
	b.WriteString(strings.Join(comments, "\n") + "\n")
	if len(indexes) > 0 {
		b.WriteString("\n" + strings.Join(indexes, "\n") + "\n")
	}
	return b.String(), fmt.Sprintf("DROP TABLE IF EXISTS %s;\n", t)
}

// definition is the column definition inside CREATE TABLE. On MySQL the
// foreign key of a ref column is a table constraint (mysqlSQL) and the
// comment is added by the caller.
func (c ColumnSpec) definition(d database.Dialect) string {
	mysql := d == database.MySQL
	name := quoteIdent(c.Name)
	notNull := c.Required
	def := ""
	var typ, extra string
	switch c.Type {
	case ColString:
		typ = fmt.Sprintf("varchar(%d)", c.length())
	case ColText:
		typ = "text"
	case ColInt:
		typ = "integer"
	case ColBigint:
		typ = "bigint"
	case ColDecimal:
		p, s := c.decimal()
		typ = fmt.Sprintf("numeric(%d, %d)", p, s)
		if mysql {
			typ = fmt.Sprintf("decimal(%d, %d)", p, s)
		}
	case ColBool:
		typ, notNull, def = "boolean", true, "false"
	case ColDate:
		typ = "date"
	case ColDatetime:
		typ = pick(mysql, "datetime(6)", "timestamptz")
	case ColJSON:
		typ = pick(mysql, "json", "jsonb")
	case ColImage:
		typ = "varchar(512)"
		if !c.Required {
			notNull, def = true, "''"
		}
	case ColAttachments:
		typ, notNull, def = "jsonb", true, "'[]'"
		extra = " CHECK (jsonb_typeof(" + name + ") = 'array')"
		if mysql {
			typ, def = "json", "(JSON_ARRAY())"
			extra = " CHECK (JSON_TYPE(" + name + ") = 'ARRAY')"
		}
	case ColRef:
		typ = "bigint"
		if !mysql {
			extra = " REFERENCES " + quoteIdent(c.Ref) + " (id)"
		}
	}
	if c.Default != "" {
		def = c.defaultSQL(d)
	}
	if len(c.Options) > 0 {
		values := make([]string, len(c.Options))
		for i, o := range c.Options {
			values[i] = o.Value
			if c.Type == ColString {
				values[i] = dialectString(d, o.Value)
			}
		}
		extra += fmt.Sprintf(" CHECK (%s IN (%s))", name, strings.Join(values, ", "))
	}
	out := name + " " + typ
	if notNull {
		out += " NOT NULL"
	}
	if def != "" {
		out += " DEFAULT " + def
	}
	return out + extra
}

func (c ColumnSpec) defaultSQL(d database.Dialect) string {
	switch c.Type {
	case ColInt, ColBigint, ColDecimal, ColBool:
		return c.Default
	case ColDatetime:
		return pick(d == database.MySQL, "CURRENT_TIMESTAMP(6)", "now()")
	case ColText:
		if d == database.MySQL {
			return "(" + dialectString(d, c.Default) + ")" // text takes expression defaults only
		}
	}
	return dialectString(d, c.Default) // string, text, date
}

// pick returns a when cond holds, else b.
func pick(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

// comment is the column comment: the label, plus the options written the
// way the generator reads them (类型：1=普通,2=加急).
func (c ColumnSpec) comment() string {
	label := strings.TrimSpace(c.Label)
	if len(c.Options) == 0 {
		return label
	}
	items := make([]string, len(c.Options))
	for i, o := range c.Options {
		items[i] = o.Value + "=" + strings.TrimSpace(o.Label)
	}
	return label + "：" + strings.Join(items, ",")
}

// sqlString quotes s as a SQL string literal (standard SQL, as PostgreSQL
// reads it; MySQL also reads it so unless s holds a backslash).
func sqlString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// dialectString quotes s for d: MySQL takes a backslash for an escape.
func dialectString(d database.Dialect, s string) string {
	if d == database.MySQL {
		s = strings.ReplaceAll(s, `\`, `\\`)
	}
	return sqlString(s)
}

// MigrationName is the migration file stem for creating table, after the
// last migration number.
func MigrationName(last uint, table string) string {
	return fmt.Sprintf("%06d_create_%s", last+1, table)
}

// ParseMigrationNumber returns the number of a migration file name.
func ParseMigrationNumber(name string) (uint, bool) {
	stem, _, ok := strings.Cut(name, "_")
	if !ok || len(stem) != 6 {
		return 0, false
	}
	n, err := strconv.ParseUint(stem, 10, 32)
	return uint(n), err == nil
}
