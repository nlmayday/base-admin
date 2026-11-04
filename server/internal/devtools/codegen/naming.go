package codegen

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	modulePattern     = regexp.MustCompile(`^[a-z][a-z0-9]{1,30}$`)
	resourcePattern   = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	identifierPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
)

// initialisms are upper-cased in Go identifiers (Go naming conventions).
var initialisms = map[string]string{
	"id": "ID", "ids": "IDs", "url": "URL", "uri": "URI", "ip": "IP", "uuid": "UUID", "api": "API",
	"http": "HTTP", "json": "JSON", "sql": "SQL", "html": "HTML", "uid": "UID", "sku": "SKU",
}

// words splits snake_case or kebab-case into lower-case words.
func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == '_' || r == '-' })
}

// goExported converts "dept_id" to "DeptID".
func goExported(s string) string {
	var b strings.Builder
	for _, w := range words(s) {
		if up, ok := initialisms[w]; ok {
			b.WriteString(up)
			continue
		}
		b.WriteString(strings.ToUpper(w[:1]) + w[1:])
	}
	return b.String()
}

// lowerCamel converts "sales-order" to "salesOrder" (JSON and TS names).
func lowerCamel(s string) string {
	ws := words(s)
	var b strings.Builder
	for i, w := range ws {
		if i == 0 {
			b.WriteString(w)
			continue
		}
		b.WriteString(strings.ToUpper(w[:1]) + w[1:])
	}
	return b.String()
}

// upperCamel converts "sales-order" to "SalesOrder" without initialisms
// (TypeScript and Vue component names).
func upperCamel(s string) string {
	c := lowerCamel(s)
	if c == "" {
		return ""
	}
	return strings.ToUpper(c[:1]) + c[1:]
}

// goUnexported converts "sales-order" to "salesOrder" honouring Go
// initialisms in later words ("api-key" becomes "apiKey").
func goUnexported(s string) string {
	ws := words(s)
	if len(ws) == 0 {
		return ""
	}
	return ws[0] + goExported(strings.Join(ws[1:], "_"))
}

// plural applies simple English pluralization to the last word.
func plural(s string) string {
	switch {
	case strings.HasSuffix(s, "s"), strings.HasSuffix(s, "x"), strings.HasSuffix(s, "z"),
		strings.HasSuffix(s, "ch"), strings.HasSuffix(s, "sh"):
		return s + "es"
	case strings.HasSuffix(s, "y") && len(s) > 1 && !strings.ContainsRune("aeiou", rune(s[len(s)-2])):
		return s[:len(s)-1] + "ies"
	default:
		return s + "s"
	}
}

// reservedWords are PostgreSQL and MySQL 8.4 reserved words, which must be
// quoted as names (double quotes work in both: MySQL sessions run with
// ANSI_QUOTES).
var reservedWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`all analyse analyze and any array as asc asymmetric authorization binary both
		case cast check collate collation column concurrently constraint create cross current_catalog current_date
		current_role current_schema current_time current_timestamp current_user default deferrable desc distinct do
		else end except false fetch for foreign freeze from full grant group having ilike in initially inner intersect
		into is isnull join lateral leading left like limit localtime localtimestamp natural not notnull null offset on
		only or order outer overlaps placing primary references returning right select session_user similar some
		symmetric system_user table tablesample then to trailing true union unique user using variadic verbose when
		where window with`) {
		reservedWords[w] = true
	}
	for _, w := range strings.Fields(`accessible add alter asensitive before between bigint blob by call cascade change
		char character condition continue convert cube cume_dist cursor database databases day_hour day_microsecond
		day_minute day_second dec decimal declare delayed delete dense_rank describe deterministic distinctrow div
		double drop dual each elseif empty enclosed escaped exists exit explain first_value float float4 float8 force
		fulltext function generated get groups grouping high_priority hour_microsecond hour_minute hour_second if
		ignore index infile inout insensitive insert int int1 int2 int3 int4 int8 integer interval io_after_gtids
		io_before_gtids iterate json_table key keys kill lag last_value lead leave linear lines load lock long longblob
		longtext loop low_priority master_bind master_ssl_verify_server_cert match maxvalue mediumblob mediumint
		mediumtext middleint minute_microsecond minute_second mod modifies no_write_to_binlog nth_value ntile numeric
		of optimize optimizer_costs option optionally out outfile over partition percent_rank precision procedure
		purge range rank read read_write reads real recursive regexp release rename repeat replace require resignal
		restrict return revoke rlike row row_number rows schema schemas second_microsecond sensitive separator set
		show signal smallint spatial specific sql sql_big_result sql_calc_found_rows sql_small_result sqlexception
		sqlstate sqlwarning ssl starting stored straight_join system terminated tinyblob tinyint tinytext trigger undo
		unlock unsigned update usage use utc_date utc_time utc_timestamp values varbinary varchar varcharacter
		varying virtual while write xor year_month zerofill`) {
		reservedWords[w] = true
	}
}

// quoteIdent quotes reserved column names for SQL.
func quoteIdent(name string) string {
	if reservedWords[name] {
		return `"` + name + `"`
	}
	return name
}

// standardLabels name common columns that have no comment.
var standardLabels = map[string]string{
	"id": "ID", "name": "名称", "code": "编码", "title": "标题", "status": "状态", "sort": "排序", "remark": "备注",
	"description": "描述", "content": "内容", "type": "类型", "phone": "手机号", "mobile": "手机号", "email": "邮箱",
	"address": "地址", "contact": "联系人", "dept_id": "部门", "owner_id": "负责人", "user_id": "用户",
	"amount": "金额", "price": "价格", "quantity": "数量", "level": "级别", "version": "版本",
	"created_at": "创建时间", "updated_at": "更新时间", "created_by": "创建人", "updated_by": "更新人",
	"deleted_at": "删除时间", "start_at": "开始时间", "end_at": "结束时间", "enabled": "是否启用",
}

// labelUnsafe drops characters that could break out of the string, comment
// or template contexts a label is pasted into (Go, TypeScript, Vue).
var labelUnsafe = strings.NewReplacer(`"`, "", "'", "", "`", "", "\\", "", "<", "", ">", "",
	"{", "", "}", "", "*/", "", "/*", "", "\r", "", "\t", "", "$", "")

// label is the short display name of a column: the column comment up to
// the first punctuation mark, a standard label, or the column name.
func label(c Column) string {
	if c.Comment != "" {
		short := c.Comment
		if i := strings.IndexAny(short, "，,。.;；:：（(\n"); i > 0 {
			short = short[:i]
		}
		if len(parseCommentOptions(c.Comment)) > 0 {
			// "状态 1正常 2冻结": the name is what comes before the options.
			if i := strings.IndexAny(short, "0123456789"); i >= 0 {
				short = short[:i]
			}
		}
		if short = strings.TrimSpace(labelUnsafe.Replace(short)); short != "" {
			return short
		}
	}
	if l, ok := standardLabels[c.Name]; ok {
		return l
	}
	return c.Name
}

// goString quotes s as a Go string literal.
func goString(s string) string { return strconv.Quote(s) }

// tsString quotes s for TypeScript double-quoted strings.
func tsString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(s) + `"`
}

// structTagValue strips characters that would break a struct tag.
func structTagValue(s string) string {
	return strings.NewReplacer(`"`, "'", "`", "'", "\n", " ", "\r", " ", `\`, "/").Replace(s)
}
