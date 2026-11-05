package codegen

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// ChildTable is a child table of a spec, as Load read it.
type ChildTable struct {
	Spec  ChildSpec
	Table Table
}

// ChildSpec is a table edited together with each row of the main table,
// one child row per key: a translation table xxx_i18n (main_id, lang,
// title, ...) gets one row per language. The keys come from a data
// dictionary (KeyDict, 系统配置 → 数据字典) or from a table the business
// maintains (KeyTable, such as a language registry shared with another
// service), never from the code.
type ChildSpec struct {
	// Table is the child table.
	Table string `json:"table"`
	// ForeignKey is the child column holding the main row's key.
	ForeignKey string `json:"foreignKey"`
	// KeyColumn is the child column that tells rows of one main row apart,
	// such as lang.
	KeyColumn string `json:"keyColumn"`
	// KeyDict is the data dictionary listing the key values (lang codes).
	KeyDict string `json:"keyDict,omitempty"`
	// KeyTable lists the key values instead of a dictionary:
	// KeyValueColumn holds the value (code), KeyLabelColumn its name
	// (default: the value), KeyFilter a SQL condition on the table
	// ("enabled = 1"), KeySortColumn the order (default: the value) and
	// KeyRequiredColumn, when set, a boolean column: the rows of those keys
	// must be filled in.
	KeyTable          string `json:"keyTable,omitempty"`
	KeyValueColumn    string `json:"keyValueColumn,omitempty"`
	KeyLabelColumn    string `json:"keyLabelColumn,omitempty"`
	KeyFilter         string `json:"keyFilter,omitempty"`
	KeySortColumn     string `json:"keySortColumn,omitempty"`
	KeyRequiredColumn string `json:"keyRequiredColumn,omitempty"`
	// Label names the section in the form, e.g. 多语言.
	Label string `json:"label,omitempty"`
	// Fields customizes the child's columns like the main fields: label,
	// form (false leaves a nullable or defaulted column out), upload
	// ("image" for an image URL column) and hint.
	Fields []FieldOption `json:"fields,omitempty"`
	// ListKeys adds a list column with the keys whose rows are filled in
	// (已填语言).
	ListKeys bool `json:"listKeys,omitempty"`
}

// childData is the template model of a child table.
type childData struct {
	Spec  ChildSpec
	Table string // quoted table name
	// GoName, JSONName and TSName name the child list in the main view and
	// request (I18n, i18n) and its types.
	GoName, JSONName, Label string
	// FK and Key are the foreign key and key columns; Fields the edited
	// columns.
	FK, Key *Field
	Fields  []*Field
	// IDGenerated: the child has its own bigint id that the module assigns
	// (otherwise its key is (FK, Key) or a database id).
	IDGenerated bool
	IDColumn    string
	// Columns is the Go select list (db in scope); InsertCols, InsertVals
	// and InsertArgs the INSERT of one row r for main key "key".
	Columns, InsertCols, InsertVals string
	InsertArgs                      []string
	// KeyTSForm is the TypeScript type of the key in the form.
	KeyTSForm string
	// KeyFromTable: the keys come from Spec.KeyTable; KeySelect is the Go
	// select list of the key options (db in scope), KeyOrder their order.
	KeyFromTable bool
	KeySelect    string
	KeyOrder     string
	// ViewConvert: the row and the view have the same fields.
	ViewConvert bool
}

// childStandard are columns a child table keeps without the form.
var childStandard = []string{"created_at", "updated_at", "created_by", "updated_by", "version", "deleted_at"}

// buildChild checks a child table against the main key and builds its
// template model; the error is Chinese.
func buildChild(ct ChildTable, mainTable string, pk *Field) (*childData, error) {
	s, t := ct.Spec, ct.Table
	switch {
	case !identifierPattern.MatchString(s.ForeignKey) || !identifierPattern.MatchString(s.KeyColumn):
		return nil, fmt.Errorf("子表 %s：foreignKey 和 keyColumn 须为列名", t.Name)
	case (s.KeyDict == "") == (s.KeyTable == ""):
		return nil, fmt.Errorf("子表 %s：keyDict（数据字典）和 keyTable（键表）须指定且只指定一个", t.Name)
	case s.KeyDict != "" && !dictCodePattern.MatchString(s.KeyDict):
		return nil, fmt.Errorf("子表 %s：keyDict 须为数据字典编码", t.Name)
	case s.KeyTable != "":
		if msg := keyTableProblem(s); msg != "" {
			return nil, fmt.Errorf("子表 %s：%s", t.Name, msg)
		}
	case strings.ContainsAny(s.Label, unsafeText) || utf8.RuneCountInString(s.Label) > 20:
		return nil, fmt.Errorf("子表 %s：label 最长 20 个字符，且不能包含引号、反斜杠、尖括号或花括号", t.Name)
	}
	fkCol, ok := t.Column(s.ForeignKey)
	if !ok {
		return nil, fmt.Errorf("子表 %s 没有列 %s", t.Name, s.ForeignKey)
	}
	if fti, ok := keyType(fkCol); !ok || fti.kind != pk.Kind {
		return nil, fmt.Errorf("子表 %s 的 %s（%s）与主表主键 %s 的类型不一致", t.Name, s.ForeignKey, fkCol.UDT, pk.Column)
	}
	keyCol, ok := t.Column(s.KeyColumn)
	if !ok {
		return nil, fmt.Errorf("子表 %s 没有列 %s", t.Name, s.KeyColumn)
	}
	kti, ok := keyType(keyCol)
	if !ok || kti.kind == kindID {
		return nil, fmt.Errorf("子表 %s 的 %s 须为文本或整数（如 lang）", t.Name, s.KeyColumn)
	}
	d := &childData{Spec: s, Table: quoteIdent(t.Name), Label: strings.TrimSpace(s.Label)}
	rest := strings.TrimPrefix(t.Name, mainTable+"_")
	d.GoName, d.JSONName = goExported(rest), lowerCamel(rest)
	if d.Label == "" {
		d.Label = rest
	}
	if s.KeyTable != "" {
		d.KeyFromTable = true
		var sel selectList
		value := quoteIdent(s.KeyValueColumn)
		sel.call("", "database.CastText(db, "+goString(value)+")", ` AS "value"`)
		label := value
		if s.KeyLabelColumn != "" {
			label = quoteIdent(s.KeyLabelColumn)
		}
		sel.call("", "database.CastText(db, "+goString(label)+")", ` AS "label"`)
		if s.KeyRequiredColumn != "" {
			sel.add(quoteIdent(s.KeyRequiredColumn) + ` AS "required"`)
		} else {
			sel.add(`false AS "required"`)
		}
		d.KeySelect = sel.goExpr()
		d.KeyOrder = value
		if s.KeySortColumn != "" {
			d.KeyOrder = quoteIdent(s.KeySortColumn) + ", " + value
		}
	}
	d.FK = newField(fkCol, mustKeyType(fkCol))
	d.Key = newField(keyCol, kti)
	d.KeyTSForm = kti.kind.tsType()

	var pkCols []Column
	for _, c := range t.Columns {
		if c.Primary {
			pkCols = append(pkCols, c)
		}
	}
	switch {
	case len(pkCols) == 2 && slices.ContainsFunc(pkCols, func(c Column) bool { return c.Name == s.ForeignKey }) &&
		slices.ContainsFunc(pkCols, func(c Column) bool { return c.Name == s.KeyColumn }):
	case len(pkCols) == 1 && (pkCols[0].Name == s.ForeignKey || pkCols[0].Name == s.KeyColumn):
		return nil, fmt.Errorf("子表 %s 的主键须为 (%s, %s)，或一个自增 / bigint 的 id", t.Name, s.ForeignKey, s.KeyColumn)
	case len(pkCols) == 1 && autoKey(pkCols[0]):
		d.IDColumn = pkCols[0].Name
	case len(pkCols) == 1 && pkCols[0].UDT == "int8":
		d.IDColumn, d.IDGenerated = pkCols[0].Name, true
	default:
		return nil, fmt.Errorf("子表 %s 的主键须为 (%s, %s)，或一个自增 / bigint 的 id", t.Name, s.ForeignKey, s.KeyColumn)
	}
	opts, err := childOptions(t, s)
	if err != nil {
		return nil, err
	}
	for _, c := range t.Columns {
		switch {
		case c.Name == s.ForeignKey || c.Name == s.KeyColumn || c.Name == d.IDColumn || slices.Contains(childStandard, c.Name):
			continue
		case sensitiveColumn(c.Name):
			continue
		}
		ti, ok := mapType(c)
		f := newField(c, ti)
		o, hasOpt := opts[c.Name]
		switch {
		case !ok || !f.Writable:
			if !c.Nullable && !c.HasDefault() {
				return nil, fmt.Errorf("子表 %s 的列 %s 不能为空且没有默认值，生成器无法填写", t.Name, c.Name)
			}
			if hasOpt {
				return nil, fmt.Errorf("子表 %s 的列 %s 不能编辑，不能配置", t.Name, c.Name)
			}
			continue
		case hasOpt && o.Form != nil && !*o.Form:
			if !c.Nullable && !c.HasDefault() {
				return nil, fmt.Errorf("子表 %s 的列 %s 不能为空且没有默认值，必须在表单中", t.Name, c.Name)
			}
			continue
		}
		f.ArgExpr = strings.Replace(f.ArgExpr, "req.", "r.", 1)
		if !slices.Contains([]string{"input", "textarea", "number", "switch", "date", "datetime"}, f.Widget) {
			f.Widget = "input" // roles and pickers of the main form do not apply here
		}
		f.Binding = strings.TrimPrefix(strings.TrimPrefix(f.Binding, "required"), ",")
		if hasOpt {
			if l := strings.TrimSpace(o.Label); l != "" {
				f.Label, f.Doc = l, structTagValue(l)
			}
			f.Hint = strings.TrimSpace(o.Hint)
			if o.Upload != nil && *o.Upload == UploadImage {
				f.Upload, f.Widget = UploadImage, "image"
			}
		}
		d.Fields = append(d.Fields, f)
	}
	if len(d.Fields) == 0 {
		return nil, fmt.Errorf("子表 %s 没有可编辑的列", t.Name)
	}
	var cols selectList
	cols.add(d.Key.SelectExpr)
	for _, f := range d.Fields {
		switch f.SelectCast {
		case castText:
			cols.call("", "database.CastText(db, "+goString(f.SelectExpr)+")", " AS "+f.Quoted)
		case castDate:
			cols.call("", "database.FormatDate(db, "+goString(f.SelectExpr)+")", " AS "+f.Quoted)
		default:
			cols.add(f.SelectExpr)
		}
	}
	d.Columns = cols.goExpr()
	insertCols := []string{d.FK.Quoted, d.Key.Quoted}
	d.InsertArgs = []string{"key", "r." + d.Key.GoName}
	if d.IDGenerated {
		insertCols = append([]string{quoteIdent(d.IDColumn)}, insertCols...)
		d.InsertArgs = append([]string{"m.NextID()"}, d.InsertArgs...)
	}
	for _, f := range d.Fields {
		insertCols = append(insertCols, f.Quoted)
		d.InsertArgs = append(d.InsertArgs, f.ArgExpr)
	}
	d.InsertCols = strings.Join(insertCols, ", ")
	d.InsertVals = strings.TrimSuffix(strings.Repeat("?, ", len(insertCols)), ", ")
	return d, nil
}

func mustKeyType(c Column) typeInfo {
	ti, _ := keyType(c)
	return ti
}

// EmptyExpr is the Go condition that field f of child request r is empty.
func (f *Field) EmptyExpr() string {
	v := "r." + f.GoName
	switch {
	case strings.HasPrefix(f.ReqType, "*"):
		return v + " == nil"
	case f.ReqType == "string":
		return v + ` == ""`
	case f.ReqType == "bool":
		return "!" + v
	case f.ReqType == "time.Time":
		return v + ".IsZero()"
	default:
		return v + " == 0"
	}
}

// SampleValue is a Go literal the request binding accepts (tests).
func (f *Field) SampleValue() string { return f.sample() }

// ChildTSDefault is the TypeScript value of an empty child row field.
func (f *Field) ChildTSDefault() string {
	switch {
	case f.Nullable:
		return "null"
	case f.Kind.numeric():
		if f.DefaultLit != "" {
			return f.DefaultLit
		}
		return "0"
	case f.Kind == kindBool:
		if f.DefaultLit == "true" {
			return "true"
		}
		return "false"
	case f.DefaultLit != "" && (f.Kind == kindString || f.Kind == kindDecimal):
		return tsString(f.DefaultLit)
	}
	return `""`
}

// ChildTSSubmit is the TypeScript value sent for child row field r.x: an
// empty text of a nullable column is null, a time is UTC.
func (f *Field) ChildTSSubmit() string {
	v := "r." + f.JSONName
	switch {
	case f.Kind == kindTime:
		v = "toUTC(" + v + ")"
		if f.Nullable {
			v += " || null"
		}
	case f.Nullable && f.Kind.tsType() == "string":
		v += " || null"
	}
	return v
}

// KeySample is a key value of the child sample row (tests).
func (c *childData) KeySample() string {
	if c.Key.Kind == kindString {
		return `"en"`
	}
	return "1"
}

// KeyFromDict converts a dictionary value (a string) to the key's type.
func (c *childData) KeyFromDict(v string) string {
	if c.Key.Kind == kindString {
		return v
	}
	return "Number(" + v + ")"
}

// keyFilterUnsafe are what a key filter may not contain: it is a fixed
// condition pasted into the generated query, not a statement.
var keyFilterUnsafe = []string{";", "?", "`", "--", "/*", "*/", "\\"}

// keyTableProblem checks the key table options; Load checks them against
// the database.
func keyTableProblem(s ChildSpec) string {
	for _, c := range []string{s.KeyTable, s.KeyValueColumn} {
		if !identifierPattern.MatchString(c) {
			return "keyTable 和 keyValueColumn 须为表名和列名"
		}
	}
	for _, c := range []string{s.KeyLabelColumn, s.KeySortColumn, s.KeyRequiredColumn} {
		if c != "" && !identifierPattern.MatchString(c) {
			return "keyLabelColumn、keySortColumn、keyRequiredColumn 须为列名"
		}
	}
	for _, bad := range keyFilterUnsafe {
		if strings.Contains(s.KeyFilter, bad) {
			return "keyFilter 是一个条件（如 enabled = 1），不能包含 " + bad
		}
	}
	if utf8.RuneCountInString(s.KeyFilter) > 200 {
		return "keyFilter 最长 200 个字符"
	}
	return ""
}

// KeySourceLabel describes where the keys come from, for notes.
func (c *childData) KeySourceLabel() string {
	if c.KeyFromTable {
		return "表 " + c.Spec.KeyTable
	}
	return "字典 " + c.Spec.KeyDict
}

// childOptions checks the field options of child spec s against table t;
// the error is Chinese.
func childOptions(t Table, s ChildSpec) (map[string]FieldOption, error) {
	out := map[string]FieldOption{}
	for _, o := range s.Fields {
		c, ok := t.Column(o.Column)
		name := strings.TrimSpace(o.Label)
		switch {
		case !ok || o.Column == s.ForeignKey || o.Column == s.KeyColumn:
			return nil, fmt.Errorf("子表 %s：字段 %s 不存在或不能配置", t.Name, o.Column)
		case out[o.Column].Column != "":
			return nil, fmt.Errorf("子表 %s：字段 %s 重复配置", t.Name, o.Column)
		case o.List != nil || o.Export != nil || o.Search != "" || o.Dict != nil || o.Relation != nil || o.RefTable != "" || o.CreateOnly:
			return nil, fmt.Errorf("子表 %s：字段 %s 只能配置 label、form、upload、hint", t.Name, o.Column)
		case o.Label != "" && (name == "" || utf8.RuneCountInString(name) > 20 || strings.ContainsAny(name, unsafeText)):
			return nil, fmt.Errorf("子表 %s：字段 %s 的中文名最长 20 个字符，且不能包含引号、反斜杠、尖括号或花括号", t.Name, o.Column)
		case hintProblem(o.Column, o.Hint) != "":
			return nil, fmt.Errorf("子表 %s：%s", t.Name, hintProblem(o.Column, o.Hint))
		case o.Upload != nil && *o.Upload != "" && *o.Upload != UploadImage:
			return nil, fmt.Errorf("子表 %s：字段 %s 只支持图片上传（upload 为 image）", t.Name, o.Column)
		case o.Upload != nil && *o.Upload == UploadImage && !imageColumn(c):
			return nil, fmt.Errorf("子表 %s：字段 %s 上传图片需要长度不小于 255 的文本字段", t.Name, o.Column)
		}
		out[o.Column] = o
	}
	return out, nil
}

// imageColumn reports a text column long enough for an image URL.
func imageColumn(c Column) bool {
	return c.UDT == "text" || (c.UDT == "varchar" && (c.MaxLength == 0 || c.MaxLength >= 255))
}
