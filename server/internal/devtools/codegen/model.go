package codegen

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Spec is what the developer asks for.
type Spec struct {
	// Module is the Go package and permission prefix, e.g. "example".
	Module string `json:"module"`
	// Resource is the kebab-case resource name, e.g. "customer".
	Resource string `json:"resource"`
	// Name is the Chinese display name, e.g. "客户".
	Name string `json:"name"`
	// ModuleTitle titles the directory menu of a new module.
	ModuleTitle string `json:"moduleTitle"`
	// ParentMenu is the menu the page goes under; default "<module>.root".
	ParentMenu string `json:"parentMenu"`
	// MenuTitle is the page's menu title; default "<Name>管理". MenuSort
	// orders it among its siblings (smaller first); default 1. Menus are
	// written to the database once, so set them before the first start.
	MenuTitle string `json:"menuTitle,omitempty"`
	MenuSort  *int   `json:"menuSort,omitempty"`
	// NewModule generates module.go and common.go as well.
	NewModule bool `json:"newModule"`
	// Fields customizes columns (label, list, form, search); columns not
	// listed keep the defaults.
	Fields []FieldOption `json:"fields,omitempty"`
	// KeyMode is how rows get their primary key (KeyAuto, KeyGenerated,
	// KeyInput); "" picks it from the column (keys.go).
	KeyMode string `json:"keyMode,omitempty"`
	// ReadOnly generates the list, detail and export only: no form, no
	// create, update or delete (logs, orders, ledgers).
	ReadOnly bool `json:"readOnly,omitempty"`
	// Children are tables edited together with each row (children.go).
	Children []ChildSpec `json:"children,omitempty"`
	// SoftDelete false makes deleted_at a plain time column (say, when an
	// account was closed): every row is listed and delete removes the row.
	// nil or true: a nullable deleted_at hides deleted rows.
	SoftDelete *bool `json:"softDelete,omitempty"`
	// OrderBy is the order of the list and the export: columns of the table
	// with ASC or DESC, such as "created_at DESC, id DESC" (index them for
	// large tables), also compared to a value: "status = 0 DESC". Default:
	// sort (when the table has it), then the key descending.
	OrderBy string `json:"orderBy,omitempty"`
	// DeleteConfirm is the delete confirmation, {column} standing for the
	// row's value: "删除分类「{code}」？大厅将不再显示该 Tab。"
	DeleteConfirm string `json:"deleteConfirm,omitempty"`
	// NoCreate and NoDelete leave out create and delete (route, permission
	// and button), e.g. games are taken off sale, never deleted.
	NoCreate bool `json:"noCreate,omitempty"`
	NoDelete bool `json:"noDelete,omitempty"`
}

// plainColumns are the columns that lose their standard role under s: a
// status with options of its own (FieldOption.Dict), deleted_at when soft
// delete is off.
func (s Spec) plainColumns() map[string]bool {
	plain := map[string]bool{}
	for _, o := range s.Fields {
		if o.Column == "status" && o.Dict != nil && *o.Dict != "" {
			plain["status"] = true
		}
	}
	if s.SoftDelete != nil && !*s.SoftDelete {
		plain["deleted_at"] = true
	}
	return plain
}

// maxMenuSort bounds Spec.MenuSort.
const maxMenuSort = 9999

// reservedModules are existing core module names.
var reservedModules = map[string]bool{"auth": true, "iam": true, "devtools": true, "core": true, "app": true}

// Problem is one invalid spec field; Message is Chinese for the UI.
type Problem struct {
	Field   string
	Message string
}

const unsafeText = "\"'`\\<>{}\n\r"

// Problems lists every invalid field of the spec.
func (s Spec) Problems() []Problem {
	var out []Problem
	add := func(field, msg string) { out = append(out, Problem{Field: field, Message: msg}) }
	switch {
	case !modulePattern.MatchString(s.Module):
		add("module", "模块名须为小写字母开头的 2–31 位小写字母或数字（Go 包名）")
	case reservedModules[s.Module]:
		add("module", "模块名 "+s.Module+" 是内置模块，不能用于生成")
	}
	if !resourcePattern.MatchString(s.Resource) || len(s.Resource) > 40 {
		add("resource", "资源名须为小写字母开头的 kebab-case，如 customer、sales-order")
	}
	name := strings.TrimSpace(s.Name)
	if name == "" || len([]rune(name)) > 20 || strings.ContainsAny(name, unsafeText) {
		add("name", "中文名不能为空，最长 20 个字符，且不能包含引号、反斜杠、尖括号或花括号")
	}
	if strings.ContainsAny(s.ModuleTitle, unsafeText) || len([]rune(s.ModuleTitle)) > 20 {
		add("moduleTitle", "模块标题最长 20 个字符，且不能包含引号、反斜杠、尖括号或花括号")
	}
	if t := strings.TrimSpace(s.MenuTitle); s.MenuTitle != "" && (t == "" || strings.ContainsAny(t, unsafeText) || len([]rune(t)) > 20) {
		add("menuTitle", "菜单标题最长 20 个字符，且不能包含引号、反斜杠、尖括号或花括号")
	}
	if s.MenuSort != nil && (*s.MenuSort < 0 || *s.MenuSort > maxMenuSort) {
		add("menuSort", fmt.Sprintf("菜单排序须在 0 到 %d 之间", maxMenuSort))
	}
	if s.ParentMenu != "" && (!strings.HasPrefix(s.ParentMenu, s.Module+".") || strings.ContainsAny(s.ParentMenu, unsafeText)) {
		add("parentMenu", "上级菜单必须是模块 "+s.Module+" 的菜单编码")
	}
	if s.KeyMode != "" && !slices.Contains(keyModes, s.KeyMode) {
		add("keyMode", "keyMode 须为 "+strings.Join(keyModes, "、")+" 之一")
	}
	if s.ReadOnly && len(s.Children) > 0 {
		add("children", "只读模块不能编辑子表")
	}
	if s.ReadOnly && (s.NoCreate || s.NoDelete) {
		add("readOnly", "只读模块本来就没有新增和删除，不用再设 noCreate、noDelete")
	}
	if _, err := parseOrderBy(s.OrderBy); err != nil {
		add("orderBy", err.Error())
	}
	return out
}

// Validate checks the spec.
func (s Spec) Validate() error {
	var errs []error
	for _, p := range s.Problems() {
		errs = append(errs, fmt.Errorf("%s: %s", p.Field, p.Message))
	}
	return errors.Join(errs...)
}

// Column roles the generator wires specially.
const (
	roleID        = "id"
	roleVersion   = "version"
	roleCreatedAt = "created_at"
	roleUpdatedAt = "updated_at"
	roleCreatedBy = "created_by"
	roleUpdatedBy = "updated_by"
	roleDeletedAt = "deleted_at"
	roleStatus    = "status"
	roleSort      = "sort"
	roleDept      = "dept_id"
)

func columnRole(c Column, k kind) string {
	switch {
	case c.plain:
		return ""
	case c.Primary:
		return roleID
	case c.Name == "version" && (k == kindInt || k == kindInt16 || k == kindInt64) && !c.Nullable:
		return roleVersion
	case (c.Name == "created_at" || c.Name == "updated_at") && k == kindTime:
		return c.Name
	case (c.Name == "created_by" || c.Name == "updated_by") && k == kindID:
		return c.Name
	case c.Name == "deleted_at" && k == kindTime && c.Nullable:
		return roleDeletedAt
	case c.Name == "status" && (k == kindInt16 || k == kindInt) && !c.Nullable && !strings.ContainsAny(c.Comment, "0123456789"):
		// A comment with values (状态：1=待支付,2=已支付, or 1正常 2冻结)
		// gives the column its own meaning; otherwise it is the standard
		// 1 启用 / 2 停用.
		return roleStatus
	case c.Name == "sort" && (k == kindInt16 || k == kindInt):
		return roleSort
	case c.Name == "dept_id" && k == kindID:
		return roleDept
	}
	return ""
}

// sensitiveColumn reports columns that must not be exposed by generated
// list/detail/export endpoints.
func sensitiveColumn(name string) bool {
	for _, s := range []string{"password", "secret", "token", "salt", "private_key"} {
		if strings.Contains(name, s) {
			return true
		}
	}
	return false
}

// Field is one generated column with its Go, SQL and TypeScript forms.
type Field struct {
	Column   string
	GoName   string
	JSONName string
	Label    string
	Doc      string
	Kind     kind
	Role     string
	Nullable bool
	Required bool
	MaxLen   int
	Writable bool
	InTable  bool
	// Search is the search mode in the list page (SearchNone etc.).
	Search string
	// Dict is where the options come from: a data dictionary code or
	// OptionsFromComment (then Enum holds them); "" for none.
	Dict string
	Enum []EnumOption
	// Upload is UploadImage or UploadFiles for upload widgets, else "".
	Upload string
	// Relation is the label column of the referenced table for relation
	// fields (relations.go), else "". The Rel* and Label* fields hold the
	// join alias, quoted names, the label field and the lookup variable.
	Relation                                string
	RelAlias, RelTable, RelLabelSQL, RelVar string
	RelKey                                  string
	RelTarget, RelOptionsTS                 string
	// RelValue and RelValuePtr are the chosen key as lookup.Target takes it
	// (of a NOT NULL and a nullable request field); RelNumber: the label is
	// an integer column, matched exactly.
	RelValue, RelValuePtr string
	RelNumber             bool
	// RelLookup: the code uses the lookup variable; LabelDoc describes the
	// shown value in the API docs.
	RelLookup                       bool
	LabelDoc                        string
	LabelColumn, LabelGo, LabelJSON string
	// DictSource is the TypeScript expression that yields the options
	// (useDict or staticDict).
	DictSource string
	// DictNumeric: the column is an integer, so dictionary values (strings)
	// are converted for the form; DictBindings destructures useDict in the
	// list page (only what it uses).
	DictNumeric  bool
	DictBindings string

	RowType    string // scanned row field
	ViewType   string // JSON view field
	ReqType    string // request field
	ViewExpr   string // row → view
	ArgExpr    string // request → SQL argument
	InsertVal  string // INSERT value placeholder ("?" or COALESCE with the default)
	UpdateArg  string // UPDATE value (ArgExpr, or COALESCE keeping the current value)
	SelectExpr string // column expression in SELECT, e.g. t.name
	// SelectCast reads the column as text (castText) or as a YYYY-MM-DD
	// date (castDate) through the database package, so the generated query
	// runs on both PostgreSQL and MySQL; "" reads it as is.
	SelectCast string
	Binding    string
	Sample     string // Go literal for the integration test payload

	TSType     string // view property type
	TSForm     string // form field type
	TSDefault  string // form default expression
	TSSubmit   string // form → payload expression
	TSReqType  string // payload property type
	Widget     string
	RuleTrig   string
	Quoted     string // quoted column for SQL
	DefaultLit string // literal column default, if any
	TSFromView string // view → form expression
	MinWidth   int    // table column width
	FixedWidth int    // width of tag columns (status, booleans)
	DictRowTS  string // the value looked up in the options, in the list

	// Search form: filter struct field(s), WHERE code, TypeScript filter
	// properties and initial values, sample query and widget.
	SearchWidget string
	FilterGo     string
	FilterApply  string
	FilterTS     []string
	FilterInit   []string
	FilterQuery  []string

	colDefault string
	comment    string
	// commentEnum: options written in the comment; baseWidget/baseTrig
	// restore the plain widget when options are turned off.
	commentEnum          []EnumOption
	ref                  *Ref
	baseWidget, baseTrig string
	unique               bool
	// canWrite: clients may set the column at all; mustWrite: it is NOT
	// NULL without a default, so the form must include it.
	canWrite, mustWrite bool
	listOverride        *bool
	exportOverride      *bool
	// CreateOnly: set on create, never changed (a typed-in primary key,
	// or FieldOption.CreateOnly).
	CreateOnly bool
	// Hint is the help text under the form input (FieldOption.Hint).
	Hint string
	// Display and form options (display.go): Format and Suffix of the
	// list column, the relation's second shown column (Extra*), the input
	// Placeholder and NumberAttrs (:min, :max, :step), ShowWhenTS (the form
	// condition), Feature (FieldOption.ShowIf), Trim (text trimmed on save).
	Format, Suffix, RelationExtra             string
	ExtraColumn, ExtraGo, ExtraJSON, ExtraSQL string
	Placeholder, NumberAttrs, ShowWhenTS      string
	Feature                                   string
	Trim                                      bool
	showWhen                                  *ShowWhen
	searchDefault, formDefault                *Scalar
	emptyAllowed                              bool
}

func newField(c Column, ti typeInfo) *Field {
	f := &Field{
		Column: c.Name, GoName: goExported(c.Name), JSONName: lowerCamel(c.Name), Label: label(c),
		Doc: structTagValue(c.Comment), Kind: ti.kind, Role: columnRole(c, ti.kind), Nullable: c.Nullable,
		MaxLen: c.MaxLength, Quoted: quoteIdent(c.Name), colDefault: c.Default,
		comment: c.Comment, unique: c.Unique, commentEnum: parseCommentOptions(c.Comment), ref: c.Ref,
	}
	if f.Doc == "" {
		f.Doc = structTagValue(f.Label)
	}
	f.DefaultLit, _ = parseDefault(c.Default)
	f.goTypes(ti)
	f.SelectExpr = "t." + f.Quoted
	if ti.castText {
		f.SelectCast = castText
		if ti.kind == kindDate {
			f.SelectCast = castDate
		}
	}
	f.writable(c)
	f.canWrite = f.Writable
	f.mustWrite = f.Writable && !c.Nullable && !c.HasDefault()
	f.binding(c)
	f.tsTypes()
	return f
}

func (f *Field) goTypes(ti typeInfo) {
	base := ti.goType
	ptr := ""
	if f.Nullable {
		ptr = "*"
	}
	f.RowType, f.ViewType, f.ReqType = ptr+base, ptr+base, ptr+base
	f.ViewExpr, f.ArgExpr = "r."+f.GoName, "req."+f.GoName
	f.InsertVal = "?"
	if ti.kind == kindID {
		f.ViewType, f.ReqType = ptr+"ids.ID", ptr+"ids.ID"
		if f.Nullable {
			f.ViewExpr, f.ArgExpr = "(*ids.ID)(r."+f.GoName+")", "(*int64)(req."+f.GoName+")"
		} else {
			f.ViewExpr, f.ArgExpr = "ids.ID(r."+f.GoName+")", "int64(req."+f.GoName+")"
		}
	}
}

// writable decides whether clients may set the column.
func (f *Field) writable(c Column) {
	switch {
	case f.Role != "" && f.Role != roleStatus && f.Role != roleSort && f.Role != roleDept:
		return
	case c.Identity || c.Generated:
		return
	case f.Kind.zeroIsInvalid() && !c.Nullable && c.HasDefault() && f.DefaultLit == "":
		// A computed default such as gen_random_uuid() or now() is left to
		// the database; a literal default (amount DEFAULT 0) stays editable
		// and pre-fills the form.
		return
	}
	f.Writable = true
	f.Required = !c.Nullable && !c.HasDefault() &&
		(f.Kind == kindString || f.Kind == kindID || f.Kind.zeroIsInvalid())
	if f.Kind.zeroIsInvalid() && !c.Nullable {
		f.Required = true
	}
}

func (f *Field) binding(c Column) {
	if !f.Writable {
		return
	}
	var rules []string
	switch {
	case f.Required:
		rules = append(rules, "required")
	case f.Nullable || f.Kind.zeroIsInvalid() || f.Role == roleStatus:
		rules = append(rules, "omitempty")
	}
	switch f.Kind {
	case kindString:
		if c.MaxLength > 0 {
			rules = append(rules, fmt.Sprintf("max=%d", c.MaxLength))
		}
	case kindDecimal:
		rules = append(rules, "numeric")
	case kindDate:
		rules = append(rules, "datetime=2006-01-02")
	case kindUUID:
		rules = append(rules, "uuid")
	case kindJSON:
		rules = append(rules, "json")
	}
	if f.Role == roleStatus {
		rules = append(rules, "oneof=1 2")
	}
	if len(rules) == 1 && rules[0] == "omitempty" {
		rules = nil
	}
	f.Binding = strings.Join(rules, ",")
}

func (f *Field) tsTypes() {
	f.TSType = f.Kind.tsType()
	if f.Role == roleStatus {
		f.TSType = "Status"
	}
	if f.Nullable {
		f.TSType += " | null"
	}
	f.TSReqType = f.TSType
	f.Widget, f.RuleTrig = "input", "blur"
	f.TSForm, f.TSSubmit = f.Kind.tsType(), "form."+f.JSONName
	switch {
	case f.Role == roleStatus:
		f.Widget, f.TSForm, f.TSDefault = "status", "Status", "STATUS_ENABLED"
	case f.Role == roleDept:
		f.Widget, f.RuleTrig, f.TSForm, f.TSDefault = "dept", "change", "string | null", "null"
	case f.Kind.numeric():
		f.Widget = "number"
		f.tsNumber()
	case f.Kind == kindBool:
		f.Widget, f.TSDefault = "switch", "false"
		if f.DefaultLit == "true" {
			f.TSDefault = "true"
		}
		if f.Nullable {
			f.TSSubmit = "form." + f.JSONName
		}
	case f.Kind == kindTime:
		f.Widget, f.RuleTrig = "datetime", "change"
		f.tsText()
		// The API takes UTC times (RFC 3339).
		f.TSSubmit = "toUTC(form." + f.JSONName + ")"
		if f.Nullable {
			f.TSSubmit += " || null"
		}
	case f.Kind == kindDate:
		f.Widget, f.RuleTrig = "date", "change"
		f.tsText()
	default:
		if f.Kind == kindJSON || (f.Kind == kindString && (f.MaxLen == 0 || f.MaxLen > 255)) {
			f.Widget = "textarea"
		}
		f.tsText()
	}
}

func (f *Field) tsNumber() {
	f.TSDefault = "0"
	if f.DefaultLit != "" {
		f.TSDefault = f.DefaultLit
	}
	if f.Nullable {
		f.TSForm, f.TSDefault = "number | undefined", "undefined"
		f.TSSubmit = "form." + f.JSONName + " ?? null"
	}
}

func (f *Field) tsText() {
	f.TSForm, f.TSDefault = "string", `""`
	if (f.Kind == kindString || f.Kind == kindDecimal || f.Kind == kindJSON) && f.DefaultLit != "" {
		f.TSDefault = tsString(f.DefaultLit)
	}
	if f.Nullable {
		f.TSSubmit = "form." + f.JSONName + " || null"
	}
}

// sample returns a Go literal accepted by the request binding.
func (f *Field) sample() string {
	if f.Role == roleID && f.Kind == kindString {
		return goString("k1") // a path segment in the test
	}
	switch f.Upload {
	case UploadImage:
		return goString(sampleAttachmentURL)
	case UploadFiles:
		return "[]any{}"
	}
	switch f.Kind {
	case kindString:
		s := "示例" + f.Label
		if f.MaxLen > 0 && len([]rune(s)) > f.MaxLen {
			s = strings.Repeat("a", f.MaxLen)
		}
		return goString(s)
	case kindID:
		return `"1"`
	case kindBool:
		return "true"
	case kindTime:
		return `"2026-01-02T03:04:05Z"`
	case kindDate:
		return `"2026-01-02"`
	case kindUUID:
		return `"00000000-0000-0000-0000-000000000001"`
	case kindJSON:
		return `"{}"`
	case kindDecimal:
		return `"1.5"`
	case kindNet:
		return `"127.0.0.1"`
	}
	return "1"
}

// DisabledAttr disables the form input of a create-only field when
// editing.
func (f *Field) DisabledAttr() string {
	if f.CreateOnly {
		return ` :disabled="isEdit"`
	}
	return ""
}

// orderTerm is one column of Spec.OrderBy; Value, when set, is an SQL
// literal the column is compared to ("status = 0 DESC": rows with status 0
// first).
type orderTerm struct {
	Column, Value string
	Desc          bool
}

// orderTermPattern matches "col", "col DESC", "col = 0 DESC", "col = 'x'".
var orderTermPattern = regexp.MustCompile(`(?i)^([a-z_][a-z0-9_]*)(?:\s*=\s*(-?[0-9]{1,18}|'[A-Za-z0-9_.-]{0,32}'|(?i:true|false)))?(?:\s+(ASC|DESC))?$`)

// SQL is the ORDER BY term on the table alias t.
func (o orderTerm) SQL() string {
	out := "t." + quoteIdent(o.Column)
	if o.Value != "" {
		out += " = " + o.Value
	}
	if o.Desc {
		out += " DESC"
	}
	return out
}

// parseOrderBy reads "created_at DESC, id" or "status = 0 DESC, id"; the
// error is Chinese.
func parseOrderBy(s string) ([]orderTerm, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []orderTerm
	for _, part := range strings.Split(s, ",") {
		m := orderTermPattern.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil || strings.ToLower(m[1]) != m[1] {
			return nil, errors.New("orderBy 须为「列名 [= 值] [ASC|DESC]」，多个用逗号分隔，如 status = 0 DESC, id DESC（值为整数、'文本'、true 或 false）")
		}
		t := orderTerm{Column: m[1], Value: m[2], Desc: strings.EqualFold(m[3], "DESC")}
		if strings.EqualFold(t.Value, "true") || strings.EqualFold(t.Value, "false") {
			t.Value = strings.ToUpper(t.Value)
		}
		if slices.ContainsFunc(out, func(o orderTerm) bool { return o.Column == t.Column && o.Value == t.Value }) {
			return nil, errors.New("orderBy 中 " + strings.TrimSpace(part) + " 重复")
		}
		out = append(out, t)
	}
	if len(out) > 5 {
		return nil, errors.New("orderBy 最多 5 项")
	}
	return out, nil
}

// KeyOfView is the Go expression of key field f of view v as the key type.
func (f *Field) KeyOfView(v string) string {
	if f.Kind == kindID {
		return "int64(" + v + "." + f.GoName + ")"
	}
	return v + "." + f.GoName
}
