package codegen

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Search modes of a field in the list page's search form.
const (
	SearchNone  = "none"
	SearchLike  = "like"  // text contains the input
	SearchEq    = "eq"    // equals the input
	SearchRange = "range" // between two values (numbers, dates, times)
)

// dictCodePattern matches data dictionary codes (system module rule).
var dictCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)

// searchModeNames are shown in problems; the UI has its own labels.
var searchModeNames = map[string]string{SearchNone: "不搜索", SearchLike: "模糊", SearchEq: "精确", SearchRange: "范围"}

// FieldOption customizes one column. Zero values keep the generator's
// default, so a spec without options generates the same as before.
type FieldOption struct {
	Column string `json:"column"`
	// Label is the Chinese field name (default: the column comment).
	Label string `json:"label,omitempty"`
	// List shows the column in the list page.
	List *bool `json:"list,omitempty"`
	// Export puts the column in the CSV export; nil keeps the default (the
	// key and the list columns).
	Export *bool `json:"export,omitempty"`
	// Form lets users set the column in the create/edit form.
	Form *bool `json:"form,omitempty"`
	// Search is SearchNone, SearchLike, SearchEq or SearchRange.
	Search string `json:"search,omitempty"`
	// Dict is where the column's options come from: a data dictionary code
	// (saving checks the value against it), OptionsFromComment (the options
	// written in the column comment), or "" for none. nil keeps the default:
	// the comment options when the comment has them.
	Dict *string `json:"dict,omitempty"`
	// Upload is UploadImage, UploadFiles or "" (none); nil keeps the
	// default picked from the column name and comment.
	Upload *string `json:"upload,omitempty"`
	// Relation is the label column shown for a foreign key column ("" for
	// a plain ID input); nil keeps the default (the best label column).
	Relation *string `json:"relation,omitempty"`
	// RefTable declares a relation without a foreign key: the bigint column
	// refers to RefTable.RefKey (default id), and Relation names the label
	// column, e.g. user_id → lg_user.nickname. Load reads the table.
	RefTable string `json:"refTable,omitempty"`
	RefKey   string `json:"refKey,omitempty"`
	// CreateOnly: the column is set in the create form and read-only when
	// editing (a game code, a family chosen once).
	CreateOnly bool `json:"createOnly,omitempty"`
	// Hint is a help text shown under the form input (up to 200
	// characters), for what does not fit in the label.
	Hint string `json:"hint,omitempty"`
	// Format displays the list column: FormatCoins, FormatSigned,
	// FormatCents, FormatPermille, FormatMilli (with Suffix) or FormatImage;
	// the export keeps the stored value.
	Format string `json:"format,omitempty"`
	Suffix string `json:"suffix,omitempty"`
	// RelationExtra is a second column of the referenced table shown with
	// the relation's label, e.g. short_id with nickname.
	RelationExtra string `json:"relationExtra,omitempty"`
	// SearchDefault fills the search field initially and on reset.
	SearchDefault *Scalar `json:"searchDefault,omitempty"`
	// Default is the value of the field in a new form.
	Default *Scalar `json:"default,omitempty"`
	// Min, Max and Step bound a number input (and the request).
	Min  *float64 `json:"min,omitempty"`
	Max  *float64 `json:"max,omitempty"`
	Step *float64 `json:"step,omitempty"`
	// Placeholder is shown in the empty form input.
	Placeholder string `json:"placeholder,omitempty"`
	// ShowWhen shows the form field only for some values of another one.
	ShowWhen *ShowWhen `json:"showWhen,omitempty"`
	// Required false lets a NOT NULL text column be saved empty.
	Required *bool `json:"required,omitempty"`
	// ShowIf "feature:<name>" shows the field (list and form) only while
	// the module's featureEnabled(ctx, name) is true; while it is off an
	// update keeps the stored value.
	ShowIf string `json:"showIf,omitempty"`
}

// maxHint bounds FieldOption.Hint.
const maxHint = 200

// hintProblem checks a hint; "" when it is fine.
func hintProblem(column, hint string) string {
	if strings.ContainsAny(hint, unsafeText) || utf8.RuneCountInString(hint) > maxHint {
		return fmt.Sprintf("字段 %s 的说明（hint）最长 %d 个字符，且不能包含引号、反斜杠、尖括号、花括号或换行", column, maxHint)
	}
	return ""
}

// OptionsFromComment as FieldOption.Dict uses the options parsed from the
// column comment, such as "类型：1=通知,2=公告".
const OptionsFromComment = "@comment"

// EnumOption is one option written in a column comment.
type EnumOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// FieldInfo describes a column and its defaults for the codegen page.
type FieldInfo struct {
	Column  string `json:"column"`
	Type    string `json:"type"`
	Comment string `json:"comment"`
	Label   string `json:"label"`
	List    bool   `json:"list"`
	// Export: in the CSV export by default (the key and list columns).
	Export bool `json:"export"`
	Form   bool `json:"form"`
	// FormLocked means the form flag cannot be changed; FormHint says why.
	FormLocked  bool     `json:"formLocked"`
	FormHint    string   `json:"formHint,omitempty"`
	Search      string   `json:"search"`
	SearchModes []string `json:"searchModes"`
	// DictAllowed: the column may have options (a dictionary or the
	// comment options).
	DictAllowed bool   `json:"dictAllowed"`
	Dict        string `json:"dict"`
	// CommentOptions are the options found in the comment, if any.
	CommentOptions []EnumOption `json:"commentOptions"`
	// Upload is the default upload widget; UploadModes the possible ones.
	Upload      string   `json:"upload"`
	UploadModes []string `json:"uploadModes"`
	// Relation is the default label column of a foreign key column;
	// RelationTable the referenced table, RelationLabels its label columns.
	Relation       string   `json:"relation"`
	RelationTable  string   `json:"relationTable"`
	RelationLabels []string `json:"relationLabels"`
	// CreateOnlyAllowed: the column may be set only on create
	// (FieldOption.CreateOnly) when it is in the form; CreateOnly and Hint
	// are the defaults (off, none).
	CreateOnlyAllowed bool   `json:"createOnlyAllowed"`
	CreateOnly        bool   `json:"createOnly"`
	Hint              string `json:"hint"`
	// TextRequired: a required text column, which may be allowed to be
	// empty (FieldOption.Required false).
	TextRequired bool `json:"textRequired"`
	// Numeric: a number column (formats, min, max, step).
	Numeric bool `json:"numeric"`
}

// kindNames are the column types shown on the codegen page.
var kindNames = map[kind]string{
	kindString: "文本", kindInt16: "整数", kindInt: "整数", kindInt64: "整数", kindID: "编号",
	kindFloat: "小数", kindDecimal: "金额/定点数", kindBool: "是/否", kindTime: "时间", kindDate: "日期",
	kindUUID: "UUID", kindJSON: "JSON", kindNet: "IP 地址",
}

// Describe lists the generated columns of t with their default options.
func Describe(t Table) ([]FieldInfo, error) {
	if err := CheckTable(t); err != nil {
		return nil, err
	}
	d := buildData(t, Spec{Module: "x", Resource: "x", Name: "x"})
	inExport := map[*Field]bool{}
	for _, f := range d.ExportFields {
		inExport[f] = true
	}
	out := make([]FieldInfo, 0, len(d.Fields))
	for _, f := range d.Fields {
		locked, hint := formLock(f)
		out = append(out, FieldInfo{
			Column: f.Column, Type: kindNames[f.Kind], Comment: f.comment, Label: f.Label,
			List: f.InTable, Export: inExport[f], Form: f.Writable, FormLocked: locked, FormHint: hint,
			Search: f.Search, SearchModes: searchModes(f), DictAllowed: dictAllowed(f),
			Dict: f.Dict, CommentOptions: append([]EnumOption{}, f.commentEnum...), // never null in JSON
			Upload: f.Upload, UploadModes: uploadModes(f),
			Relation: f.Relation, RelationTable: relationTable(f), RelationLabels: relationLabels(f),
			CreateOnlyAllowed: f.canWrite && f.Role != roleID && f.Upload == "",
			TextRequired:      f.Kind == kindString && f.Required && f.Role != roleID, Numeric: f.Kind.numeric() || f.Kind == kindDecimal,
		})
	}
	return out, nil
}

// formLock reports whether the form flag of f is fixed, and why.
func formLock(f *Field) (bool, string) {
	switch {
	case !f.canWrite:
		return true, "由系统维护"
	case f.Role != "" && f.Role != roleID:
		return true, "系统字段，固定在表单中"
	case f.mustWrite:
		return true, "不能为空且没有默认值，必须填写"
	}
	return false, ""
}

// dictAllowed reports whether f may show options (a data dictionary or
// the comment options): text, integer and yes/no columns without a
// standard role; options on status replace its 1 启用 / 2 停用. Yes/no
// columns take options only outside the form (FieldProblems).
func dictAllowed(f *Field) bool {
	switch f.Kind {
	case kindString, kindInt16, kindInt, kindInt64, kindBool:
		return f.Role == "" || f.Role == roleStatus
	}
	return false
}

// searchModes lists the search modes that suit f.
func searchModes(f *Field) []string {
	modes := []string{SearchNone}
	switch {
	case f.Role == roleStatus || f.Role == roleDept:
		modes = append(modes, SearchEq)
	case f.Role == roleCreatedAt || f.Role == roleUpdatedAt:
		modes = append(modes, SearchRange)
	case f.Role != "" && f.Role != roleID:
	case f.Kind == kindString:
		modes = append(modes, SearchLike, SearchEq)
	case f.Kind.numeric() || f.Kind == kindDecimal || f.Kind == kindDate:
		modes = append(modes, SearchEq, SearchRange)
	case f.Kind == kindTime:
		modes = append(modes, SearchRange)
	case f.Kind == kindBool || f.Kind == kindUUID || f.Kind == kindNet || f.Kind == kindID:
		modes = append(modes, SearchEq)
	}
	return modes
}

// defaultSearch picks the initial search fields: the status, and up to two
// short text columns (unique ones first), like the former keyword search.
func (d *tmplData) defaultSearch() {
	for _, f := range d.Fields {
		f.Search = SearchNone
		if f.Role == roleStatus {
			f.Search = SearchEq
		}
	}
	candidates := make([]*Field, 0, len(d.Fields))
	for _, f := range d.Fields {
		if f.Kind == kindString && f.Role == "" && f.MaxLen <= 255 && !longText(f.Column) {
			candidates = append(candidates, f)
		}
	}
	slices.SortStableFunc(candidates, func(a, b *Field) int {
		switch {
		case a.unique == b.unique:
			return 0
		case a.unique:
			return -1
		}
		return 1
	})
	for _, f := range candidates[:min(2, len(candidates))] {
		f.Search = SearchLike
	}
}

func longText(column string) bool {
	for _, s := range []string{"remark", "description", "content", "note", "memo"} {
		if strings.Contains(column, s) {
			return true
		}
	}
	return false
}

// FieldProblems checks spec.Fields against table t.
func (s Spec) FieldProblems(t Table) []Problem {
	var out []Problem
	add := func(msg string) { out = append(out, Problem{Field: "fields", Message: msg}) }
	d := buildData(t, s.probe())
	byColumn := map[string]*Field{}
	for _, f := range d.Fields {
		byColumn[f.Column] = f
	}
	seen := map[string]bool{}
	for _, o := range s.Fields {
		f, ok := byColumn[o.Column]
		switch {
		case !ok:
			add("字段 " + o.Column + " 不存在或不会生成")
			continue
		case seen[o.Column]:
			add("字段 " + o.Column + " 重复配置")
			continue
		}
		seen[o.Column] = true
		name := strings.TrimSpace(o.Label)
		if o.Label != "" && (name == "" || utf8.RuneCountInString(name) > 20 || strings.ContainsAny(name, unsafeText)) {
			add("字段 " + o.Column + " 的中文名最长 20 个字符，且不能包含引号、反斜杠、尖括号或花括号")
		}
		if locked, _ := formLock(f); o.Form != nil && locked && *o.Form != f.Writable {
			add("字段 " + o.Column + " 的「表单」不能修改")
		}
		if o.Search != "" && !slices.Contains(searchModes(f), o.Search) {
			add(fmt.Sprintf("字段 %s 不支持「%s」搜索", o.Column, searchModeNames[o.Search]))
		}
		if o.Dict != nil && *o.Dict != "" {
			switch {
			case !dictAllowed(f):
				add("字段 " + o.Column + " 不能使用下拉选项（只支持文本、整数或是/否字段）")
			case f.Kind == kindBool && !s.ReadOnly && formOn(f, o):
				add("字段 " + o.Column + " 是是/否字段，只有不在表单中（只读模块或关闭「表单」）时才能使用下拉选项")
			case *o.Dict == OptionsFromComment && len(f.commentEnum) == 0:
				add("字段 " + o.Column + " 的注释中没有选项（格式如 类型：1=通知,2=公告）")
			case *o.Dict != OptionsFromComment && !dictCodePattern.MatchString(*o.Dict):
				add("字段 " + o.Column + " 的字典编码无效")
			case o.Search == SearchLike || o.Search == SearchRange:
				add("字段 " + o.Column + " 使用下拉选项后只能「精确」搜索")
			}
		}
		if o.Relation != nil && *o.Relation != "" && !slices.Contains(relationLabels(f), *o.Relation) {
			add("字段 " + o.Column + " 的关联显示列无效")
		}
		if msg := hintProblem(o.Column, o.Hint); msg != "" {
			add(msg)
		}
		for _, msg := range extrasProblems(s, byColumn, f, o) {
			add(msg)
		}
		if o.CreateOnly {
			switch {
			case f.Role == roleID:
				add("字段 " + o.Column + " 是主键：手填的主键本来就只在新增时填写")
			case s.NoCreate:
				add("字段 " + o.Column + "：关闭新增（noCreate）后没有新增表单，不能设为「仅新增时填写」")
			case !formOn(f, o) || s.ReadOnly:
				add("字段 " + o.Column + " 不在表单中，不能设为「仅新增时填写」")
			case f.Upload != "" || (o.Upload != nil && *o.Upload != ""):
				add("字段 " + o.Column + " 是上传字段，不能设为「仅新增时填写」")
			}
		}
		if o.Upload != nil && *o.Upload != "" {
			switch {
			case !slices.Contains(uploadModes(f), *o.Upload):
				add("字段 " + o.Column + " 不能使用该上传方式（图片需要长度不小于 255 的文本字段，附件需要 JSON（jsonb 或 json）字段）")
			case o.Dict != nil && *o.Dict != "":
				add("字段 " + o.Column + " 不能同时使用上传和下拉选项")
			case o.Search != "" && o.Search != SearchNone:
				add("字段 " + o.Column + " 使用上传后不能搜索")
			}
		}
	}
	if msg := deleteConfirmProblem(s.DeleteConfirm, byColumn); msg != "" {
		out = append(out, Problem{Field: "deleteConfirm", Message: msg})
	}
	if terms, err := parseOrderBy(s.OrderBy); err == nil {
		for _, o := range terms {
			if _, ok := t.Column(o.Column); !ok {
				out = append(out, Problem{Field: "orderBy", Message: "orderBy 的列 " + o.Column + " 不存在"})
			}
		}
	}
	return out
}

// formOn reports whether f ends up in the form under option o.
func formOn(f *Field, o FieldOption) bool {
	if locked, _ := formLock(f); o.Form == nil || locked {
		return f.Writable
	}
	return *o.Form
}

// probe is a valid spec that gives t's columns the roles s gives them, to
// check s.Fields against.
func (s Spec) probe() Spec {
	p := Spec{Module: "x", Resource: "x", Name: "x", SoftDelete: s.SoftDelete, ReadOnly: s.ReadOnly}
	for _, o := range s.Fields {
		if o.Column == "status" && o.Dict != nil && *o.Dict != "" {
			p.Fields = append(p.Fields, FieldOption{Column: o.Column, Dict: o.Dict})
		}
	}
	return p
}

// applyOptions overrides the defaults with spec.Fields (already validated).
func (d *tmplData) applyOptions() {
	byColumn := map[string]FieldOption{}
	for _, o := range d.Spec.Fields {
		byColumn[o.Column] = o
	}
	for _, f := range d.Fields {
		o, ok := byColumn[f.Column]
		if !ok {
			continue
		}
		if l := strings.TrimSpace(o.Label); l != "" && l != f.Label {
			f.Label, f.Doc = l, structTagValue(l)
		}
		if o.Form != nil && f.canWrite && !f.mustWrite && f.Role == "" {
			f.Writable = *o.Form
		}
		if o.List != nil {
			f.listOverride = o.List
		}
		if o.Export != nil {
			f.exportOverride = o.Export
		}
		if o.Search != "" {
			f.Search = o.Search
		}
		f.Hint = strings.TrimSpace(o.Hint)
		f.applyExtras(o)
		if o.CreateOnly && f.Role != roleID {
			f.CreateOnly = true
		}
		if o.Upload != nil {
			f.setUpload(*o.Upload)
		}
		if o.Relation != nil {
			f.Relation = *o.Relation
		}
		switch {
		case o.Dict == nil:
		case *o.Dict != "":
			f.Upload = "" // options replace a default upload widget
			f.setChoices(*o.Dict)
		case f.Upload == "":
			f.setChoices("")
		}
	}
	if d.ReadOnly {
		for _, f := range d.Fields {
			f.Writable = false
		}
	}
	d.Writable = d.Writable[:0]
	d.UniqueFields = d.UniqueFields[:0]
	for _, f := range d.Fields {
		if f.Writable {
			d.Writable = append(d.Writable, f)
			if f.unique {
				d.UniqueFields = append(d.UniqueFields, f)
			}
		}
	}
}

// deriveSearch builds the filter code of every searched field.
func (d *tmplData) deriveSearch() {
	var init, query []string
	for _, f := range d.Fields {
		if f.Search == "" || f.Search == SearchNone {
			continue
		}
		f.searchCode()
		d.SearchFields = append(d.SearchFields, f)
		init = append(init, f.FilterInit...)
		query = append(query, f.FilterQuery...)
		if f.SearchWidget == "dateRange" {
			d.DateRanges = append(d.DateRanges, f)
		}
		if f.SearchWidget == "status" {
			d.StatusSearch = true
		}
		if f.SearchWidget == "dept" {
			d.DeptSearch = true
		}
		if f.Kind == kindString && f.Role == "" {
			d.TextSearch = true // strings.TrimSpace in the filter
		}
	}
	if len(init) > 0 {
		d.FilterInit = " " + strings.Join(init, ", ") + " "
	}
	d.FilterQuery = strings.Join(query, "&")
}

// searchCode fills the Go filter field, the WHERE code, the TypeScript
// filter properties and the search widget of f.
func (f *Field) searchCode() {
	col := "t." + f.Quoted
	param := f.JSONName
	switch {
	case f.Role == roleStatus:
		f.SearchWidget = "status"
		f.FilterGo = fmt.Sprintf("%s %s `form:%q binding:\"omitempty,oneof=1 2\" doc:\"%s：1 启用，2 停用\"`", f.GoName, f.RowType, param, f.Doc)
		f.FilterApply = fmt.Sprintf("if f.%s != 0 {\n\t\tq = q.Where(%q, f.%s)\n\t}", f.GoName, col+" = ?", f.GoName)
		f.FilterTS = []string{param + "?: Status | null;"}
		f.FilterInit = []string{param + ": null"}
		f.FilterQuery = []string{param + "=1"}
	case f.Search == SearchRange:
		f.rangeCode(col, param)
	default:
		f.eqCode(col, param)
	}
}

func (f *Field) eqCode(col, param string) {
	op := " = ?"
	value := "v"
	switch {
	case f.Kind == kindString:
		f.SearchWidget, f.FilterInit = "text", []string{param + `: ""`}
		if f.Dict != "" {
			f.SearchWidget = "dict"
		}
		f.FilterGo = fmt.Sprintf("%s string `form:%q binding:\"max=%d\" doc:%q`", f.GoName, param, max(f.MaxLen, 64), f.Doc+searchDoc(f.Search))
		f.FilterTS = []string{param + "?: string;"}
		f.FilterQuery = []string{param + "=x"}
		if f.Search == SearchLike {
			value = `"%"+escapeLike(v)+"%"`
		}
		where := col + op
		if f.Search == SearchLike {
			where = "LOWER(" + col + ") LIKE LOWER(?)"
		}
		f.FilterApply = fmt.Sprintf("if v := strings.TrimSpace(f.%s); v != \"\" {\n\t\tq = q.Where(%q, %s)\n\t}", f.GoName, where, value)
		return
	case f.Kind == kindID:
		f.SearchWidget = "id"
		if f.Role == roleDept {
			f.SearchWidget = "dept"
		}
		f.FilterGo = fmt.Sprintf("%s int64 `form:%q doc:%q`", f.GoName, param, f.Doc)
		f.FilterApply = fmt.Sprintf("if f.%s != 0 {\n\t\tq = q.Where(%q, f.%s)\n\t}", f.GoName, col+op, f.GoName)
		f.FilterTS, f.FilterInit, f.FilterQuery = []string{param + "?: string | null;"}, []string{param + ": null"}, []string{param + "=1"}
		return
	case f.Kind.numeric():
		f.SearchWidget = "number"
		f.FilterGo = fmt.Sprintf("%s *%s `form:%q doc:%q`", f.GoName, numericGoType(f.Kind), param, f.Doc)
		f.FilterTS, f.FilterInit, f.FilterQuery = []string{param + "?: number;"}, []string{param + ": undefined"}, []string{param + "=1"}
	case f.Kind == kindBool:
		f.SearchWidget = "bool"
		f.FilterGo = fmt.Sprintf("%s *bool `form:%q doc:%q`", f.GoName, param, f.Doc)
		f.FilterTS, f.FilterInit, f.FilterQuery = []string{param + "?: boolean | null;"}, []string{param + ": null"}, []string{param + "=true"}
	default: // decimal, date, uuid, inet: strings checked by binding
		f.SearchWidget, f.FilterInit = "text", []string{param + `: ""`}
		binding, sample := "max=64", "x"
		switch f.Kind {
		case kindDecimal:
			binding, sample = "omitempty,numeric", "1.5"
		case kindDate:
			f.SearchWidget = "date"
			binding, sample = "omitempty,datetime=2006-01-02", "2026-01-01"
		case kindUUID:
			binding, sample = "omitempty,uuid", "00000000-0000-0000-0000-000000000000"
		case kindNet:
			binding, sample = "omitempty,ip", "127.0.0.1"
		}
		f.FilterGo = fmt.Sprintf("%s string `form:%q binding:%q doc:%q`", f.GoName, param, binding, f.Doc)
		f.FilterApply = fmt.Sprintf("if f.%s != \"\" {\n\t\tq = q.Where(%q, f.%s)\n\t}", f.GoName, col+op, f.GoName)
		f.FilterTS, f.FilterQuery = []string{param + "?: string;"}, []string{param + "=" + sample}
		return
	}
	f.FilterApply = fmt.Sprintf("if f.%s != nil {\n\t\tq = q.Where(%q, *f.%s)\n\t}", f.GoName, col+op, f.GoName)
	if f.Dict != "" {
		// Picked from the dictionary select: a string in the query.
		f.SearchWidget, f.FilterTS, f.FilterInit = "dict", []string{param + "?: string;"}, []string{param + `: ""`}
	}
}

func (f *Field) rangeCode(col, param string) {
	lo, hi := f.GoName+"Min", f.GoName+"Max"
	loParam, hiParam := param+"Min", param+"Max"
	switch f.Kind {
	case kindDate, kindTime:
		lo, hi, loParam, hiParam = f.GoName+"Start", f.GoName+"End", param+"Start", param+"End"
		f.SearchWidget = "dateRange"
		utc := ""
		if f.Kind == kindTime {
			utc = "（UTC）"
		}
		f.FilterGo = fmt.Sprintf("%s string `form:%q binding:\"omitempty,datetime=2006-01-02\" doc:\"%s起始日期%s\"`\n\t%s string `form:%q binding:\"omitempty,datetime=2006-01-02\" doc:\"%s截止日期（含）%s\"`",
			lo, loParam, f.Doc, utc, hi, hiParam, f.Doc, utc)
		// UTC days, like the times of the API and the export; the end day
		// is included.
		f.FilterApply = fmt.Sprintf("if d, err := time.ParseInLocation(time.DateOnly, f.%s, time.UTC); err == nil {\n\t\tq = q.Where(%q, d)\n\t}\n"+
			"\tif d, err := time.ParseInLocation(time.DateOnly, f.%s, time.UTC); err == nil {\n\t\tq = q.Where(%q, d.AddDate(0, 0, 1))\n\t}",
			lo, col+" >= ?", hi, col+" < ?")
		f.FilterTS = []string{loParam + "?: string;", hiParam + "?: string;"}
		f.FilterQuery = []string{loParam + "=2026-01-01", hiParam + "=2026-12-31"}
		return
	case kindDecimal:
		f.SearchWidget = "decimalRange"
		f.FilterGo = fmt.Sprintf("%s string `form:%q binding:\"omitempty,numeric\" doc:\"%s最小值\"`\n\t%s string `form:%q binding:\"omitempty,numeric\" doc:\"%s最大值\"`",
			lo, loParam, f.Doc, hi, hiParam, f.Doc)
		f.FilterApply = fmt.Sprintf("if f.%s != \"\" {\n\t\tq = q.Where(%q, f.%s)\n\t}\n\tif f.%s != \"\" {\n\t\tq = q.Where(%q, f.%s)\n\t}",
			lo, col+" >= ?", lo, hi, col+" <= ?", hi)
		f.FilterTS = []string{loParam + "?: string;", hiParam + "?: string;"}
		f.FilterInit = []string{loParam + `: ""`, hiParam + `: ""`}
		f.FilterQuery = []string{loParam + "=1", hiParam + "=100.5"}
		return
	}
	f.SearchWidget = "numberRange"
	goType := numericGoType(f.Kind)
	f.FilterGo = fmt.Sprintf("%s *%s `form:%q doc:\"%s最小值\"`\n\t%s *%s `form:%q doc:\"%s最大值\"`",
		lo, goType, loParam, f.Doc, hi, goType, hiParam, f.Doc)
	f.FilterApply = fmt.Sprintf("if f.%s != nil {\n\t\tq = q.Where(%q, *f.%s)\n\t}\n\tif f.%s != nil {\n\t\tq = q.Where(%q, *f.%s)\n\t}",
		lo, col+" >= ?", lo, hi, col+" <= ?", hi)
	f.FilterTS = []string{loParam + "?: number;", hiParam + "?: number;"}
	f.FilterInit = []string{loParam + ": undefined", hiParam + ": undefined"}
	f.FilterQuery = []string{loParam + "=1", hiParam + "=100"}
}

func numericGoType(k kind) string {
	if k == kindFloat {
		return "float64"
	}
	return "int64"
}

func searchDoc(mode string) string {
	if mode == SearchLike {
		return "（模糊匹配）"
	}
	return ""
}

// commentOptionsPattern matches one "value=label" item of a comment.
var commentOptionsPattern = regexp.MustCompile(`^([A-Za-z0-9_-]{1,32})=([^=]{1,20})$`)

// numberedOptionsPattern matches one "1正常" item: a number followed by a
// label that starts with a letter (Chinese included).
var numberedOptionsPattern = regexp.MustCompile(`(\d{1,9})\s*(\pL[^\d\s,，;；、=:：()（）]{0,19})`)

// commentSeparators are the characters between options.
const commentSeparators = ",，;；、 \t"

// parseCommentOptions reads options from a column comment written as
// "类型：1=通知,2=公告" (also with （…） instead of the colon, and with
// ，；、 or spaces between items) or as "1正常 2冻结 3注销" (with or
// without a "状态：" in front). It needs two or more well-formed items.
func parseCommentOptions(comment string) []EnumOption {
	body, prefixed := comment, false
	if i := strings.IndexAny(body, ":：（("); i >= 0 {
		body = body[i:]
		_, size := utf8.DecodeRuneInString(body)
		body, prefixed = strings.TrimRight(body[size:], ")） "), true
	}
	if strings.Contains(body, "=") {
		if !prefixed {
			return nil
		}
		return assignedOptions(body)
	}
	return numberedOptions(body)
}

// assignedOptions reads "1=通知,2=公告".
func assignedOptions(body string) []EnumOption {
	items := strings.FieldsFunc(body, func(r rune) bool { return strings.ContainsRune(commentSeparators, r) })
	if len(items) < 2 {
		return nil
	}
	out := make([]EnumOption, 0, len(items))
	for _, item := range items {
		m := commentOptionsPattern.FindStringSubmatch(item)
		if m == nil {
			return nil
		}
		if out = appendOption(out, m[1], m[2]); out == nil {
			return nil
		}
	}
	return out
}

// numberedOptions reads "1正常 2冻结 3注销"; anything else in body (other
// than separators) means it is not a list of options. Leading text such
// as "状态 " is allowed.
func numberedOptions(body string) []EnumOption {
	first := strings.IndexFunc(body, func(r rune) bool { return r >= '0' && r <= '9' })
	if first < 0 {
		return nil
	}
	if lead := strings.TrimSpace(body[:first]); lead != "" && strings.ContainsAny(lead, commentSeparators) {
		return nil // digits further into a sentence
	}
	rest := body[first:]
	matches := numberedOptionsPattern.FindAllStringSubmatchIndex(rest, -1)
	if len(matches) < 2 {
		return nil
	}
	out := make([]EnumOption, 0, len(matches))
	pos := 0
	for _, m := range matches {
		if strings.Trim(rest[pos:m[0]], commentSeparators) != "" {
			return nil
		}
		if out = appendOption(out, rest[m[2]:m[3]], rest[m[4]:m[5]]); out == nil {
			return nil
		}
		pos = m[1]
	}
	if strings.Trim(rest[pos:], commentSeparators) != "" {
		return nil
	}
	return out
}

// appendOption adds one option, or returns nil for a repeated value or an
// empty label.
func appendOption(out []EnumOption, value, label string) []EnumOption {
	label = strings.TrimSpace(labelUnsafe.Replace(label))
	if label == "" || slices.ContainsFunc(out, func(o EnumOption) bool { return o.Value == value }) {
		return nil
	}
	return append(out, EnumOption{Value: value, Label: label})
}

// defaultChoices gives columns whose comment lists options a select of
// those options, searched by default: such columns are what lists filter by.
func (d *tmplData) defaultChoices() {
	for _, f := range d.Fields {
		if len(f.commentEnum) > 0 && f.Role == "" && dictAllowed(f) && (f.Kind != kindBool || d.ReadOnly || !f.Writable) {
			f.setChoices(OptionsFromComment)
			f.Search = SearchEq
		}
	}
}

// setChoices sets where f's options come from (see FieldOption.Dict).
func (f *Field) setChoices(source string) {
	if f.baseWidget == "" {
		f.baseWidget, f.baseTrig = f.Widget, f.RuleTrig
	}
	f.Dict, f.Enum, f.DictNumeric = "", nil, false
	f.Widget, f.RuleTrig = f.baseWidget, f.baseTrig
	if source == "" {
		return
	}
	f.Dict, f.DictNumeric = source, f.Kind.numeric()
	if source == OptionsFromComment {
		f.Enum = f.commentEnum
	}
	f.Widget, f.RuleTrig = "dict", "change"
	if f.Search == SearchLike || f.Search == SearchRange {
		f.Search = SearchEq // options are picked from a list
	}
}
