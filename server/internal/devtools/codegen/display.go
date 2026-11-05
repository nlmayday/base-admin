package codegen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Display formats of list columns (FieldOption.Format); the export keeps
// the stored value.
const (
	FormatCoins    = "coins"    // 1,234,567
	FormatSigned   = "signed"   // +1,234 in green, -56 in red
	FormatCents    = "cents"    // 12345 → $123.45
	FormatPermille = "permille" // 55 → +5.5%
	FormatMilli    = "milli"    // 1500 → 1.50 (and FieldOption.Suffix, e.g. x)
	FormatImage    = "image"    // a thumbnail of an image URL, opened in a preview
)

var numberFormats = []string{FormatCoins, FormatSigned, FormatCents, FormatPermille, FormatMilli}

// Scalar is an option value written in JSON as a string, a number or a
// boolean; it is kept as text.
type Scalar string

// UnmarshalJSON accepts a string, a number or a boolean.
func (s *Scalar) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return err
	}
	switch x := v.(type) {
	case string:
		*s = Scalar(x)
	case json.Number:
		*s = Scalar(x.String())
	case bool:
		*s = Scalar(strconv.FormatBool(x))
	default:
		return errors.New("须为字符串、数字或布尔值")
	}
	return nil
}

// ShowWhen shows a form field only while another field has one of the
// values In, e.g. {"column": "fill_type", "in": [1]}.
type ShowWhen struct {
	Column string   `json:"column"`
	In     []Scalar `json:"in"`
}

var (
	featurePattern = regexp.MustCompile(`^feature:[a-z][a-z0-9_]{0,31}$`)
	safeValue      = regexp.MustCompile(`^[\p{L}\p{N} _.:/@+-]{0,64}$`)
)

// extrasProblems checks the display, form and search options of field
// option o (FieldProblems); messages are Chinese.
func extrasProblems(s Spec, byColumn map[string]*Field, f *Field, o FieldOption) []string {
	var out []string
	add := func(format string, args ...any) {
		out = append(out, fmt.Sprintf("字段 %s ", o.Column)+fmt.Sprintf(format, args...))
	}
	switch {
	case o.Format == "":
	case slices.Contains(numberFormats, o.Format):
		if !f.Kind.numeric() && f.Kind != kindDecimal {
			add("的显示格式 %s 只用于数字字段", o.Format)
		}
	case o.Format == FormatImage:
		if f.Kind != kindString {
			add("的显示格式 image 只用于文本字段（图片地址）")
		}
	default:
		add("的显示格式须为 coins、signed、cents、permille、milli、image 之一")
	}
	if o.Suffix != "" && (o.Format != FormatMilli || !safeValue.MatchString(o.Suffix) || utf8.RuneCountInString(o.Suffix) > 8) {
		add("的 suffix 只用于 milli 格式，最长 8 个字符")
	}
	relation := f.Relation
	if o.Relation != nil {
		relation = *o.Relation
	}
	if o.RelationExtra != "" && (relation == "" || o.RelationExtra == relation || !slices.Contains(relationLabels(f), o.RelationExtra)) {
		add("的 relationExtra 须为关联表中另一个可显示的列（%s）", strings.Join(relationLabels(f), "、"))
	}
	inForm := formOn(f, o) && !s.ReadOnly
	if o.Default != nil {
		switch {
		case !inForm:
			add("不在表单中，不能设默认值")
		case !valueFits(f, string(*o.Default)):
			add("的默认值 %q 与字段类型不符", string(*o.Default))
		}
	}
	if o.SearchDefault != nil {
		search := f.Search
		if o.Search != "" {
			search = o.Search
		}
		switch {
		case search != SearchEq && search != SearchLike:
			add("只有「精确」或「模糊」搜索才能设搜索默认值")
		case !valueFits(f, string(*o.SearchDefault)):
			add("的搜索默认值 %q 与字段类型不符", string(*o.SearchDefault))
		}
	}
	if o.Min != nil || o.Max != nil || o.Step != nil {
		switch {
		case !f.Kind.numeric() && f.Kind != kindDecimal:
			add("不是数字字段，不能设 min、max、step")
		case o.Min != nil && o.Max != nil && *o.Min > *o.Max:
			add("的 min 不能大于 max")
		case o.Step != nil && *o.Step <= 0:
			add("的 step 须大于 0")
		}
	}
	if strings.ContainsAny(o.Placeholder, unsafeText) || utf8.RuneCountInString(o.Placeholder) > 50 {
		add("的 placeholder 最长 50 个字符，且不能包含引号、反斜杠、尖括号或花括号")
	}
	if o.Required != nil {
		switch {
		case *o.Required:
			add("的 required 只能设为 false（允许保存空文本）")
		case f.Kind != kindString || !f.Required:
			add("不是必填的文本字段，不需要 required: false")
		}
	}
	if w := o.ShowWhen; w != nil {
		target, ok := byColumn[w.Column]
		switch {
		case !ok || w.Column == o.Column || !target.canWrite:
			add("的 showWhen 须指向表单中的另一个字段")
		case len(w.In) == 0 || len(w.In) > 20:
			add("的 showWhen.in 须列出 1–20 个值")
		case f.Required && (o.Required == nil || *o.Required):
			add("必填，不能按条件隐藏（可设 required: false）")
		}
		for _, v := range w.In {
			if ok && !valueFits(target, string(v)) {
				add("的 showWhen.in 中 %q 与字段 %s 的类型不符", string(v), w.Column)
			}
		}
	}
	if o.ShowIf != "" && !featurePattern.MatchString(o.ShowIf) {
		add("的 showIf 须为 feature:<开关名>，开关名为小写字母、数字或下划线")
	}
	return out
}

// valueFits reports whether v is a value of field f (a default, a search
// default, a showWhen value).
func valueFits(f *Field, v string) bool {
	switch {
	case f.Kind == kindBool:
		return v == "true" || v == "false" || v == "1" || v == "0"
	case f.Kind == kindID:
		_, err := strconv.ParseInt(v, 10, 64)
		return err == nil
	case f.Kind.numeric():
		_, err := strconv.ParseFloat(v, 64)
		return err == nil
	case f.Kind == kindDecimal:
		_, err := strconv.ParseFloat(v, 64)
		return err == nil
	case f.Kind == kindString:
		return safeValue.MatchString(v)
	case f.Kind == kindDate:
		return len(v) == 10 && safeValue.MatchString(v)
	}
	return false
}

// applyExtras applies the display, form and search options (validated).
func (f *Field) applyExtras(o FieldOption) {
	f.Format, f.Suffix = o.Format, o.Suffix
	f.RelationExtra = o.RelationExtra
	f.Placeholder = strings.TrimSpace(o.Placeholder)
	f.showWhen, f.searchDefault, f.formDefault = o.ShowWhen, o.SearchDefault, o.Default
	if o.Required != nil && !*o.Required {
		f.Required, f.emptyAllowed = false, true
		f.Binding = strings.TrimPrefix(strings.TrimPrefix(f.Binding, "required"), ",")
	}
	if name, ok := strings.CutPrefix(o.ShowIf, "feature:"); ok {
		f.Feature = name
	}
	var attrs, rules []string
	for _, b := range []struct {
		attr, rule string
		v          *float64
	}{{"min", "min", o.Min}, {"max", "max", o.Max}, {"step", "", o.Step}} {
		if b.v == nil {
			continue
		}
		n := strconv.FormatFloat(*b.v, 'f', -1, 64)
		attrs = append(attrs, fmt.Sprintf(` :%s="%s"`, b.attr, n))
		if b.rule != "" && f.Kind.numeric() {
			rules = append(rules, b.rule+"="+n)
		}
	}
	f.NumberAttrs = strings.Join(attrs, "")
	if len(rules) > 0 {
		f.Binding = withOmitempty(strings.Trim(f.Binding+","+strings.Join(rules, ","), ","))
		if f.Required {
			f.Binding = strings.Replace(f.Binding, "omitempty,", "", 1)
		}
	}
}

// tsLiteral writes v as a TypeScript value for a form or filter of type
// tsType ("number", "string", "boolean"...).
func tsLiteral(v, tsType string) string {
	switch {
	case strings.Contains(tsType, "number") || strings.Contains(tsType, "Status"):
		return v
	case strings.Contains(tsType, "boolean"):
		return strconv.FormatBool(v == "true" || v == "1")
	}
	return tsString(v)
}

// deriveExtras turns the applied options into template code: the form
// default, the filter default, the showWhen condition, the trimmed text.
func (d *tmplData) deriveExtras() {
	byColumn := map[string]*Field{}
	for _, f := range d.Fields {
		byColumn[f.Column] = f
	}
	features := map[string]bool{}
	for _, f := range d.Fields {
		if f.formDefault != nil {
			f.TSDefault = tsLiteral(string(*f.formDefault), f.TSForm)
		}
		if f.searchDefault != nil && len(f.FilterInit) == 1 {
			name, typ, _ := strings.Cut(f.FilterTS[0], ":")
			f.FilterInit = []string{strings.TrimSuffix(name, "?") + ": " + tsLiteral(string(*f.searchDefault), typ)}
		}
		if w := f.showWhen; w != nil {
			target := byColumn[w.Column]
			vals := make([]string, len(w.In))
			for i, v := range w.In {
				vals[i] = tsLiteral(string(v), target.TSForm)
			}
			f.ShowWhenTS = "([" + strings.Join(vals, ", ") + "] as unknown[]).includes(form." + target.JSONName + ")"
		}
		if f.Writable && f.Kind == kindString && (f.Widget == "input" || f.Widget == "textarea") {
			f.Trim = true
			d.TrimFields = append(d.TrimFields, f)
		}
		if f.Feature != "" && !features[f.Feature] {
			features[f.Feature] = true
			d.Features = append(d.Features, f.Feature)
		}
		if f.Format != "" {
			d.Formats = true
		}
	}
	d.NeedsStrings = d.NeedsStrings || len(d.TrimFields) > 0
	var init, query []string
	for _, f := range d.SearchFields {
		init, query = append(init, f.FilterInit...), append(query, f.FilterQuery...)
	}
	if len(init) > 0 {
		d.FilterInit = " " + strings.Join(init, ", ") + " "
	}
	d.FilterQuery = strings.Join(query, "&")
}

// TrimStmt is the Go code that trims text field f of request r (and, for a
// required field, refuses what is left empty).
func (f *Field) TrimStmt(r, field string) string {
	v := r + "." + f.GoName
	var b strings.Builder
	if strings.HasPrefix(f.ReqType, "*") {
		fmt.Fprintf(&b, "if %s != nil {\n\t\tv := strings.TrimSpace(*%s)\n\t\t%s = &v\n\t}", v, v, v)
		return b.String()
	}
	fmt.Fprintf(&b, "%s = strings.TrimSpace(%s)", v, v)
	if f.Required {
		fmt.Fprintf(&b, "\n\tif %s == \"\" {\n\t\treturn fieldError(%s, \"不能为空\")\n\t}", v, field)
	}
	return b.String()
}

// deleteConfirmPattern matches the {column} placeholders of a delete
// confirmation.
var deleteConfirmPattern = regexp.MustCompile(`\{([a-z_][a-z0-9_]*)\}`)

// deleteConfirmProblem checks Spec.DeleteConfirm against the fields.
func deleteConfirmProblem(text string, byColumn map[string]*Field) string {
	if text == "" {
		return ""
	}
	if utf8.RuneCountInString(text) > 100 {
		return "deleteConfirm 最长 100 个字符"
	}
	rest := deleteConfirmPattern.ReplaceAllStringFunc(text, func(m string) string {
		if _, ok := byColumn[m[1:len(m)-1]]; !ok {
			return "\x00"
		}
		return ""
	})
	switch {
	case strings.Contains(rest, "\x00"):
		return "deleteConfirm 中的 {列名} 须为本表生成的列"
	case strings.ContainsAny(rest, unsafeText+"$"):
		return "deleteConfirm 不能包含引号、反斜杠、尖括号、$，花括号只用于 {列名}"
	}
	return ""
}

// deleteConfirmTS writes Spec.DeleteConfirm as a TypeScript template
// literal over row.
func deleteConfirmTS(text string, byColumn map[string]*Field) string {
	return "`" + deleteConfirmPattern.ReplaceAllStringFunc(text, func(m string) string {
		return "${row." + byColumn[m[1:len(m)-1]].JSONName + "}"
	}) + "`"
}

// childSearch adds the searched child text fields to the list's filters:
// a row matches when one of its child rows contains the text.
func (d *tmplData) childSearch() {
	for _, c := range d.Children {
		for _, cf := range c.Search {
			name := c.GoName + cf.GoName
			f := &Field{Column: cf.Column, GoName: name, JSONName: strings.ToLower(name[:1]) + name[1:], Label: cf.Label,
				Doc: cf.Doc, Kind: kindString, Search: SearchLike, SearchWidget: "text"}
			where := "EXISTS (SELECT 1 FROM " + c.Table + " c WHERE c." + c.FK.Quoted + " = t." + d.PK.Quoted +
				" AND LOWER(c." + cf.Quoted + ") LIKE LOWER(?))"
			f.FilterGo = fmt.Sprintf("%s string `form:%q binding:\"max=64\" doc:%q`", f.GoName, f.JSONName, c.Label+"："+cf.Doc+"（模糊匹配）")
			f.FilterApply = fmt.Sprintf("if v := strings.TrimSpace(f.%s); v != \"\" {\n\t\tq = q.Where(%q, \"%%\"+escapeLike(v)+\"%%\")\n\t}", f.GoName, where)
			f.FilterTS, f.FilterInit, f.FilterQuery = []string{f.JSONName + "?: string;"}, []string{f.JSONName + `: ""`}, []string{f.JSONName + "=x"}
			d.SearchFields = append(d.SearchFields, f)
			d.TextSearch = true
		}
		d.NeedsStrings = d.NeedsStrings || len(c.TrimFields) > 0
	}
}

// filterKinds lists the filter parameters with the kind of their route
// query value, as TypeScript object entries; date ranges are separate.
func filterKinds(fields []*Field) []string {
	var out []string
	for _, f := range fields {
		if f.SearchWidget == "dateRange" {
			continue
		}
		for _, ts := range f.FilterTS {
			name, typ, _ := strings.Cut(ts, ":")
			kind := "string"
			switch {
			case strings.Contains(typ, "number") || strings.Contains(typ, "Status"):
				kind = "number"
			case strings.Contains(typ, "boolean"):
				kind = "boolean"
			}
			out = append(out, strings.TrimSuffix(name, "?")+`: "`+kind+`"`)
		}
	}
	return out
}

// ColumnAttrs are the attributes of f's list column (el-table-column).
func (f *Field) ColumnAttrs() string {
	var a string
	switch {
	case f.Role == roleStatus:
		a = fmt.Sprintf(`width="%d" align="center"`, f.FixedWidth)
	case f.Upload == UploadImage || f.Format == FormatImage:
		a = `width="90" align="center"`
	case f.Upload == UploadFiles:
		a = `width="100" align="center"`
	case f.Dict != "":
		a = fmt.Sprintf(`min-width="%d"`, f.FixedWidth)
	case f.Relation != "" && f.ExtraGo != "":
		a = `min-width="160" show-overflow-tooltip`
	case f.Relation != "" || f.Role == roleDept:
		a = `min-width="120" show-overflow-tooltip`
	case f.Kind == kindTime:
		a = `width="215"`
	case f.Kind == kindBool:
		a = fmt.Sprintf(`width="%d" align="center"`, f.FixedWidth)
	case f.Format != "":
		a = fmt.Sprintf(`min-width="%d" align="right"`, f.MinWidth)
	default:
		a = fmt.Sprintf(`min-width="%d" show-overflow-tooltip`, f.MinWidth)
	}
	return a
}

// ColumnIf is the v-if of f's list column (its feature switch), first
// among the attributes.
func (f *Field) ColumnIf() string {
	if f.Feature == "" {
		return ""
	}
	return ` v-if="features.` + f.Feature + ` === true"`
}

// FormatCall is the TypeScript expression of f's formatted value in row.
func (f *Field) FormatCall() string {
	v := "row." + f.JSONName
	switch f.Format {
	case FormatCoins:
		return "formatCoins(" + v + ")"
	case FormatSigned:
		return "formatSigned(" + v + ")"
	case FormatCents:
		return "formatCents(" + v + ")"
	case FormatPermille:
		return "formatPermille(" + v + ")"
	case FormatMilli:
		if f.Suffix != "" {
			return "formatMilli(" + v + ", " + tsString(f.Suffix) + ")"
		}
		return "formatMilli(" + v + ")"
	}
	return v
}

// indexFormatImports lists the @/utils/format helpers the list page uses.
func (d *tmplData) indexFormatImports() string {
	names := map[string]bool{}
	for _, f := range d.TableFields {
		switch f.Format {
		case FormatCoins, FormatCents, FormatPermille, FormatMilli:
			names["format"+strings.ToUpper(f.Format[:1])+f.Format[1:]] = true
		case FormatSigned:
			names["formatSigned"], names["signClass"] = true, true
		}
		if f.ExtraGo != "" {
			names["relationLabel"] = true
		}
	}
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	slices.Sort(out)
	return strings.Join(out, ", ")
}

// FormItemIf is the v-if of f's form item: its showWhen condition and its
// feature switch.
func (f *Field) FormItemIf() string {
	var conds []string
	if f.ShowWhenTS != "" {
		conds = append(conds, f.ShowWhenTS)
	}
	if f.Feature != "" {
		conds = append(conds, "props.features?."+f.Feature+" === true")
	}
	if len(conds) == 0 {
		return ""
	}
	return ` v-if="` + strings.Join(conds, " && ") + `"`
}

// PlaceholderOr is f's placeholder, or def.
func (f *Field) PlaceholderOr(def string) string {
	if f.Placeholder != "" {
		return f.Placeholder
	}
	return def
}

// PlaceholderAttr is a placeholder attribute when f has one.
func (f *Field) PlaceholderAttr() string {
	if f.Placeholder == "" {
		return ""
	}
	return ` placeholder="` + f.Placeholder + `"`
}
