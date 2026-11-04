package codegen

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"go/format"
	"strconv"
	"strings"
	"text/template"
	"unicode/utf8"

	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/perm"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var templates = template.Must(template.New("codegen").Delims("[[", "]]").Funcs(template.FuncMap{
	"join":     strings.Join,
	"inc":      func(i int) int { return i + 1 },
	"goString": goString,
}).ParseFS(templateFS, "templates/*.tmpl"))

// File is one generated file; Path is relative to the repository root.
type File struct {
	Path     string `json:"path"`
	Language string `json:"language"`
	Content  string `json:"content"`
	// Keep marks a file written once for hand-written code (the extension
	// points of a resource): it is never overwritten, not even with force.
	Keep bool `json:"keep,omitempty"`
	// Refresh marks generated helpers shared by the module's resources
	// (common.go): always written, so resources generated later find the
	// helpers they call; never a conflict.
	Refresh bool `json:"refresh,omitempty"`
}

// Result is the output of Generate.
type Result struct {
	Files       []File     `json:"files"`
	Permissions []perm.Def `json:"-"`
	Menus       []menu.Def `json:"-"`
	// Wiring is the code to add to server/internal/app/modules.go.
	Wiring string `json:"wiring"`
	// Notes are warnings and manual follow-up steps.
	Notes []string `json:"notes"`
	// ManualWiring explains how to register a resource in an existing module
	// whose module.go has no codegen markers.
	ManualWiring string `json:"-"`
}

// Generate renders all files for table t. It never touches the disk.
func Generate(t Table, spec Spec) (Result, error) {
	if err := spec.Validate(); err != nil {
		return Result{}, err
	}
	if err := CheckTable(t); err != nil {
		return Result{}, err
	}
	if pk, _ := primaryKey(t); spec.KeyMode != "" {
		if msg := keyModeProblem(pk, spec.KeyMode); msg != "" {
			return Result{}, fmt.Errorf("keyMode: %s", msg)
		}
	}
	if problems := spec.FieldProblems(t); len(problems) > 0 {
		msgs := make([]string, len(problems))
		for i, p := range problems {
			msgs[i] = p.Message
		}
		return Result{}, fmt.Errorf("fields: %s", strings.Join(msgs, "；"))
	}
	d := buildData(t, spec)
	if d.childErr != nil {
		return Result{}, fmt.Errorf("children: %w", d.childErr)
	}
	if len(spec.Children) != len(t.Children) {
		return Result{}, errors.New("children: 子表未加载（用 codegen.Load 读取表结构）")
	}
	var res Result
	type job struct {
		tmpl, path, lang string
		keep             bool // extension points: written once (File.Keep)
		refresh          bool // shared helpers: always written (File.Refresh)
	}
	jobs := []job{
		{"common.go.tmpl", d.serverDir + "/common.go", "go", false, true},
		{"resource.go.tmpl", d.serverDir + "/" + d.FileBase + ".go", "go", false, false},
		{"resource_ext.go.tmpl", d.serverDir + "/" + d.FileBase + "_ext.go", "go", true, false},
		{"resource_integration_test.go.tmpl", d.serverDir + "/" + d.FileBase + "_integration_test.go", "go", false, false},
		{"api.ts.tmpl", "web/src/api/" + d.Pkg + "/" + d.Resource + ".ts", "typescript", false, false},
		{"index.vue.tmpl", d.viewDir + "/index.vue", "vue", false, false},
		{"extension.vue.tmpl", d.viewDir + "/components/" + d.TSUpper + "Extension.vue", "vue", true, false},
	}
	if !spec.ReadOnly {
		jobs = append(jobs, job{"form.vue.tmpl", d.viewDir + "/components/" + d.TSUpper + "FormDialog.vue", "vue", false, false})
	}
	if spec.NewModule {
		jobs = append([]job{{"module.go.tmpl", d.serverDir + "/module.go", "go", false, false}}, jobs...)
	}
	for _, j := range jobs {
		content, err := render(j.tmpl, d, j.lang == "go")
		if err != nil {
			return Result{}, err
		}
		res.Files = append(res.Files, File{Path: j.path, Language: j.lang, Content: content, Keep: j.keep, Refresh: j.refresh})
	}
	res.Permissions, res.Menus = d.permissions(), d.menus()
	res.Wiring = d.wiring()
	res.Notes = d.notes()
	res.ManualWiring = d.manualWiring()
	return res, nil
}

func render(name string, d *tmplData, goSource bool) (string, error) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, name, d); err != nil {
		return "", fmt.Errorf("render %s: %w", name, err)
	}
	if !goSource {
		return buf.String(), nil
	}
	src, err := format.Source(buf.Bytes())
	if err != nil {
		return "", fmt.Errorf("format %s: %w\n%s", name, err, buf.String())
	}
	return string(src), nil
}

// tmplData is the template model.
type tmplData struct {
	Spec
	Pkg, Table, TableComment                  string
	Lower, Upper, PluralUpper                 string // Go identifiers
	TSUpper, TSPluralUpper                    string // TypeScript identifiers
	FileBase, URLPath, URLName                string
	PermPrefix, MenuCode, PagePath, Component string
	ViewName                                  string

	Fields, Writable, TableFields, ExportFields, SearchFields []*Field
	// UniqueFields are writable columns with a single-column unique index.
	UniqueFields                 []*Field
	RequiredFields, SampleFields []*Field
	// DateRanges are searched by a date range picker.
	DateRanges []*Field
	// DisplayField names a row in messages, e.g. the delete confirmation.
	DisplayField *Field
	// DictFields are written columns with options (form selects);
	// CheckDictFields are those backed by a dictionary (checked on save;
	// comment options are checked by the binding); IndexDicts are option
	// columns shown or searched in the list. The *DictImport lists name the
	// useDict.ts helpers each page imports.
	DictFields, CheckDictFields, IndexDicts []*Field
	IndexDictImport, FormDictImport         string
	// Upload widgets (uploads.go): UploadFields are written ones, checked on
	// save; HasFiles: some field is an attachment list (TypeScript type);
	// NeedsAttachment: the Go code uses internal/core/attachment.
	UploadFields []*Field
	// Relations are relation fields; RelationFields the written ones
	// (checked on save).
	Relations, RelationFields []*Field
	// RelationSearch are relation fields searched in the list page.
	RelationSearch                                  []*Field
	HasFiles, NeedsAttachment, FormImage, FormFiles bool
	// FormDateTime: the form has a date-time picker (UTC hint, toUTC).
	FormDateTime    bool
	DictNumericForm bool

	Version, Status, Sort, Dept, CreatedBy, UpdatedBy, CreatedAt, UpdatedAt, SoftDelete *Field

	// PK is the primary key field; KeyMode how rows get it (keys.go);
	// KeyGo, KeyBits and KeyZero its Go type, strconv bit size and zero.
	PK             *Field
	KeyMode, KeyGo string
	KeyZero        string
	KeyBits        int
	KeyAuto        bool
	KeyGenerated   bool
	KeyInput       bool
	// UpdateFields are the written fields an update sets (no create-only
	// key).
	UpdateFields []*Field
	// Children are the child tables edited with each row (children.go).
	Children []*childData
	childErr error
	// MissingKey is a key value no test row has (read-only test).
	MissingKey string
	// OptionRelations are the relations with an options route: all of them,
	// or only the searched ones of a read-only module. NeedsIDs: the code
	// uses ids.ID.
	OptionRelations []*Field
	NeedsIDs        bool
	// NeedsDatabase: the code calls the database package (writes, casts).
	NeedsDatabase bool
	// EnumExports are exported columns with comment options, exported as
	// their labels.
	EnumExports []*Field
	// ExportDicts are exported dictionary-backed columns, exported as the
	// labels of the dictionary.
	ExportDicts []*Field
	// DictBool: a yes/no column shows options (the list's dictBool helper).
	DictBool bool
	// ChildDictKeys: a child's keys come from a dictionary (form import).
	ChildDictKeys bool
	// OmittableDefaults: a NOT NULL column with a default may be omitted
	// on create (tested with the required fields only).
	OmittableDefaults bool

	DeptJoin, Scope, NeedsActor, NeedsUID, HasRequired, TableHasTime bool
	StatusSearch, DeptSearch, TextSearch, StatusInTable              bool
	ScopeDept, ScopeOwner, Order, Columns, StatusEnabled             string
	InsertCols, InsertVals, SoftDeleteSets                           string
	// Columns is a Go expression of the select list (db in scope).
	InsertArgs              []string
	FilterInit, FilterQuery string

	serverDir, viewDir string
	skipped            []string
}

func buildData(t Table, spec Spec) *tmplData {
	spec.Name = strings.TrimSpace(spec.Name)
	d := &tmplData{
		Spec: spec, Pkg: spec.Module, Table: t.Name, TableComment: t.Comment,
		Lower: goUnexported(spec.Resource), Upper: goExported(spec.Resource),
		PluralUpper: goExported(plural(spec.Resource)),
		TSUpper:     upperCamel(spec.Resource), TSPluralUpper: upperCamel(plural(spec.Resource)),
		FileBase: strings.ReplaceAll(spec.Resource, "-", "_"),
		URLPath:  "/" + plural(spec.Resource), URLName: plural(spec.Resource),
		PermPrefix: spec.Module + ":" + spec.Resource, MenuCode: spec.Module + "." + spec.Resource,
		PagePath:  "/" + spec.Module + "/" + spec.Resource,
		Component: spec.Module + "/" + spec.Resource + "/index",
		ViewName:  upperCamel(spec.Module) + upperCamel(spec.Resource),
		serverDir: "server/internal/modules/" + spec.Module,
		viewDir:   "web/src/views/" + spec.Module + "/" + spec.Resource,
	}
	if d.ParentMenu == "" {
		d.ParentMenu = spec.Module + ".root"
	}
	if d.ModuleTitle == "" {
		d.ModuleTitle = spec.Module
	}
	d.collectFields(t)
	for _, ct := range t.Children {
		c, err := buildChild(ct, t.Name, d.PK)
		if err != nil {
			d.childErr = err
			continue
		}
		d.Children = append(d.Children, c)
	}
	d.defaultSearch()
	d.defaultChoices()
	d.defaultUploads()
	d.defaultRelations()
	d.applyOptions()
	d.derive()
	return d
}

func (d *tmplData) collectFields(t Table) {
	plain := d.plainColumns()
	for _, c := range t.Columns {
		c.plain = plain[c.Name]
		ti, ok := mapType(c)
		switch {
		case c.Primary:
			ti, ok = keyType(c)
		case c.Ref != nil && c.UDT == "int8":
			ti = typeInfo{kind: kindID, goType: "int64"} // a declared relation, whatever the name
		}
		switch {
		case !ok:
			d.skipped = append(d.skipped, fmt.Sprintf("列 %s（%s）类型不受支持，已跳过", c.Name, c.UDT))
			continue
		case sensitiveColumn(c.Name):
			d.skipped = append(d.skipped, fmt.Sprintf("列 %s 疑似敏感字段，未生成，请手动处理", c.Name))
			continue
		}
		f := newField(c, ti)
		if f.Role == roleDept && !f.Nullable {
			f.TSSubmit = "form." + f.JSONName + ` ?? ""`
		}
		d.assignRole(f)
		if f.Role == roleDeletedAt {
			continue // always NULL for visible rows
		}
		if c.Primary {
			d.PK = f
			d.KeyMode = d.Spec.KeyMode
			if d.KeyMode == "" {
				d.KeyMode = defaultKeyMode(c)
			}
			if d.KeyMode == KeyInput {
				// Typed in on create, never changed; duplicates are field
				// errors like other unique values.
				f.Writable, f.canWrite, f.mustWrite, f.CreateOnly, f.Required, f.unique = true, true, true, true, true, true
				f.binding(c)
				f.Binding = strings.TrimPrefix(strings.TrimPrefix(f.Binding, "required"), ",")
			}
		}
		d.Fields = append(d.Fields, f)
	}
}

func (d *tmplData) assignRole(f *Field) {
	targets := map[string]**Field{
		roleVersion: &d.Version, roleStatus: &d.Status, roleSort: &d.Sort, roleDept: &d.Dept,
		roleCreatedBy: &d.CreatedBy, roleUpdatedBy: &d.UpdatedBy, roleCreatedAt: &d.CreatedAt,
		roleUpdatedAt: &d.UpdatedAt, roleDeletedAt: &d.SoftDelete,
	}
	if p, ok := targets[f.Role]; ok {
		*p = f
	}
}

func (d *tmplData) derive() {
	d.KeyGo, d.KeyBits = d.keyGo()
	d.KeyZero = d.keyZero()
	d.MissingKey = "999999999"
	switch d.KeyGo {
	case "string":
		d.MissingKey = "no-such-key"
	case "int16":
		d.MissingKey = "32000"
	}
	d.KeyAuto, d.KeyGenerated, d.KeyInput = d.KeyMode == KeyAuto, d.KeyMode == KeyGenerated, d.KeyMode == KeyInput
	for _, f := range d.Writable {
		if !f.CreateOnly {
			d.UpdateFields = append(d.UpdateFields, f)
		}
	}
	d.DeptJoin = d.Dept != nil && !d.hasColumn("dept_name")
	if d.Dept != nil {
		d.ScopeDept = "t.dept_id"
	}
	if d.CreatedBy != nil {
		d.ScopeOwner = "t.created_by"
	}
	d.Scope = d.ScopeDept != "" || d.ScopeOwner != ""
	d.NeedsUID = d.CreatedBy != nil || d.UpdatedBy != nil
	d.NeedsActor = d.Scope || d.NeedsUID || d.Dept != nil || d.Status != nil
	if d.ReadOnly {
		d.NeedsActor = d.Scope // the user id and defaults are for writes
	}
	for _, f := range d.Fields {
		d.NeedsIDs = d.NeedsIDs || f.Kind == kindID
	}
	d.StatusEnabled = "actor.StatusEnabled"
	if d.Status != nil && d.Status.Kind != kindInt16 {
		d.StatusEnabled = "int(actor.StatusEnabled)"
	}
	d.Order = "t." + d.PK.Quoted + " DESC"
	if d.Sort != nil {
		d.Order = "t.sort, t." + d.PK.Quoted + " DESC"
	}
	d.deriveUploads()
	for _, f := range d.Fields {
		d.FormDateTime = d.FormDateTime || (f.Writable && f.Widget == "datetime")
	}
	for _, c := range d.Children {
		d.ChildDictKeys = d.ChildDictKeys || !c.KeyFromTable
		for _, f := range c.Fields {
			d.FormDateTime = d.FormDateTime || f.Widget == "datetime"
		}
	}
	d.deriveRelations()
	d.deriveColumns()
	d.deriveDefaults()
	d.deriveInsert()
	d.deriveLists()
	d.deriveSearch()
	d.relationSearch()
	d.OptionRelations = d.Relations
	if d.ReadOnly {
		d.OptionRelations = d.RelationSearch
	}
	d.NeedsDatabase = !d.ReadOnly || strings.Contains(d.Columns, "database.")
	for _, f := range d.ExportFields {
		if f.Dict == "" || f.Role == roleStatus || f.Relation != "" || (f.Role == roleDept && d.DeptJoin) {
			continue
		}
		if f.Enum != nil {
			d.EnumExports = append(d.EnumExports, f)
		} else {
			d.ExportDicts = append(d.ExportDicts, f)
		}
	}
	d.deriveDisplay()
	for _, f := range d.Fields {
		if f.Dict == "" {
			continue
		}
		f.DictSource = dictSource(f)
		f.DictRowTS = "row." + f.JSONName
		if f.Kind == kindBool {
			// Options of a yes/no column have the values 1 and 0.
			f.DictRowTS = "dictBool(row." + f.JSONName + ")"
			d.DictBool = true
		}
		if f.Writable {
			d.DictFields = append(d.DictFields, f)
			if f.Enum == nil {
				d.CheckDictFields = append(d.CheckDictFields, f)
			} else {
				f.Binding = enumBinding(f)
			}
		}
		var parts []string
		if f.Search == SearchEq {
			parts = append(parts, "items: "+f.JSONName+"Options")
		}
		if f.InTable {
			parts = append(parts, "labelOf: "+f.JSONName+"Label", "tagTypeOf: "+f.JSONName+"TagType")
		}
		if len(parts) > 0 {
			f.DictBindings = strings.Join(parts, ", ")
			d.IndexDicts = append(d.IndexDicts, f)
		}
	}
	d.IndexDictImport, d.FormDictImport = dictImports(d.IndexDicts), dictImports(d.DictFields)
}

// dictSource is the TypeScript call that yields f's options; comment
// options are inlined with the same shape as dictionary items.
func dictSource(f *Field) string {
	if f.Enum == nil {
		return "useDict(" + tsString(f.Dict) + ")"
	}
	items := make([]string, len(f.Enum))
	for i, o := range f.Enum {
		items[i] = "{ value: " + tsString(o.Value) + ", label: " + tsString(o.Label) + " }"
	}
	return "staticDict([" + strings.Join(items, ", ") + "])"
}

// enumBinding adds the comment options to f's validation rules.
func enumBinding(f *Field) string {
	values := make([]string, len(f.Enum))
	for i, o := range f.Enum {
		values[i] = o.Value
	}
	rules := f.Binding
	switch {
	case rules == "":
		rules = "omitempty"
	case !strings.Contains(rules, "required") && !strings.Contains(rules, "omitempty"):
		rules = "omitempty," + rules
	}
	return rules + ",oneof=" + strings.Join(values, " ")
}

// dictImports lists the useDict.ts helpers that fields use.
func dictImports(fields []*Field) string {
	var dict, static bool
	for _, f := range fields {
		dict = dict || f.Enum == nil
		static = static || f.Enum != nil
	}
	var names []string
	if static {
		names = append(names, "staticDict")
	}
	if dict {
		names = append(names, "useDict")
	}
	return strings.Join(names, ", ")
}

func (d *tmplData) hasColumn(name string) bool {
	for _, f := range d.Fields {
		if f.Column == name {
			return true
		}
	}
	return false
}

func (d *tmplData) deriveColumns() {
	var cols selectList
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
	if d.DeptJoin {
		cols.add("COALESCE(d.name, '') AS dept_name")
	}
	for _, f := range d.Relations {
		cols.call("COALESCE(", "database.CastText(db, "+goString(f.RelAlias+"."+f.RelLabelSQL)+")", ", '') AS "+quoteIdent(f.LabelColumn))
	}
	d.Columns = cols.goExpr()
	sets := []string{"deleted_at = CURRENT_TIMESTAMP(6)"}
	if d.UpdatedBy != nil {
		sets = append(sets, "updated_by = ?")
	}
	if d.Version != nil {
		sets = append(sets, "version = version + 1")
	}
	if d.UpdatedAt != nil {
		sets = append(sets, "updated_at = CURRENT_TIMESTAMP(6)")
	}
	d.SoftDeleteSets = strings.Join(sets, ", ")
}

// deriveDefaults lets a create request omit an optional column with a
// database default: the INSERT falls back to the default instead of writing
// the Go zero value (kind 0 against DEFAULT 1 CHECK (kind IN (1, 2))) or
// NULL. A NOT NULL column becomes a pointer in the request so that omission
// is visible; an update that omits it keeps the current value. Defaults the
// zero value already matches are left alone.
func (d *tmplData) deriveDefaults() {
	for _, f := range d.Writable {
		if f.UpdateArg == "" {
			f.UpdateArg = f.ArgExpr
		}
		if !f.usesDefault() {
			continue
		}
		f.InsertVal = "COALESCE(?, " + f.defaultSQL() + ")"
		if !f.Nullable {
			d.OmittableDefaults = true
			f.ReqType = "*" + f.ReqType
			f.UpdateArg = "gorm.Expr(" + goString("COALESCE(?, "+f.Quoted+")") + ", " + f.ArgExpr + ")"
			// Omitting the column is allowed now: the other rules (max=3)
			// apply only to a value.
			f.Binding = withOmitempty(f.Binding)
		}
	}
}

// withOmitempty makes rules apply only to a given value.
func withOmitempty(rules string) string {
	if rules == "" || strings.HasPrefix(rules, "omitempty") || strings.HasPrefix(rules, "required") {
		return rules
	}
	return "omitempty," + rules
}

// usesDefault reports an optional plain column whose default the INSERT
// should apply when the request omits it.
func (f *Field) usesDefault() bool {
	if f.Required || f.colDefault == "" || f.Role != "" || f.Upload != "" || f.Kind == kindID {
		return false
	}
	// The expression is pasted into a raw string query: no backquote, and
	// no ? that GORM would take for a placeholder.
	if strings.ContainsAny(f.defaultSQL(), "`?") {
		return false
	}
	if f.Nullable {
		return true
	}
	lit, ok := parseDefault(f.colDefault)
	if !ok {
		return false
	}
	switch f.Kind {
	case kindBool:
		return lit != "false"
	case kindString:
		return lit != ""
	}
	if n, err := strconv.ParseFloat(lit, 64); err == nil {
		return n != 0
	}
	return true
}

// defaultSQL writes the column default for the generated INSERT in SQL
// that PostgreSQL and MySQL both accept: literals of plain kinds, dates
// and the current time. Other expressions (gen_random_uuid(), a jsonb
// literal) are kept as the database shows them.
func (f *Field) defaultSQL() string {
	lit, ok := parseDefault(f.colDefault)
	switch {
	case !ok:
		if f.Kind == kindTime && currentTime[strings.ToLower(strings.ReplaceAll(f.colDefault, " ", ""))] {
			return "CURRENT_TIMESTAMP(6)"
		}
	case strings.Contains(lit, `\`):
		// Read as an escape by MySQL only: keep the database's spelling.
	case f.Kind == kindString:
		return sqlString(lit)
	case f.Kind == kindBool, f.Kind.numeric():
		return lit
	case f.Kind == kindDecimal:
		if !strings.Contains(lit, ".") {
			lit += ".0" // a numeric literal: 0 would make the parameter an integer
		}
		return lit
	case f.Kind == kindDate:
		return "CAST(" + sqlString(lit) + " AS DATE)"
	}
	return f.colDefault
}

// currentTime are the defaults that mean "now", spaces removed.
var currentTime = map[string]bool{
	"now()": true, "current_timestamp": true, "current_timestamp(6)": true, "now(6)": true,
	"current_timestamp()": true, "localtimestamp": true, "transaction_timestamp()": true,
}

func (d *tmplData) deriveInsert() {
	var cols, vals []string
	if d.KeyGenerated {
		cols, vals = append(cols, d.PK.Quoted), append(vals, "?")
		d.InsertArgs = append(d.InsertArgs, "key")
	}
	for _, f := range d.Writable {
		cols, vals = append(cols, f.Quoted), append(vals, f.InsertVal)
		d.InsertArgs = append(d.InsertArgs, f.ArgExpr)
	}
	for _, f := range []*Field{d.CreatedBy, d.UpdatedBy} {
		if f != nil {
			cols, vals = append(cols, f.Column), append(vals, "?")
			d.InsertArgs = append(d.InsertArgs, "uid")
		}
	}
	for _, f := range []*Field{d.CreatedAt, d.UpdatedAt} {
		if f != nil && f.colDefault == "" {
			cols, vals = append(cols, f.Column), append(vals, "CURRENT_TIMESTAMP(6)")
		}
	}
	d.InsertCols, d.InsertVals = strings.Join(cols, ", "), strings.Join(vals, ", ")
}

func (d *tmplData) deriveLists() {
	shown := 0
	for _, f := range d.Fields {
		if f.Writable && f.Required {
			d.RequiredFields = append(d.RequiredFields, f)
			d.HasRequired = true
		}
		if f.Writable && (f.Required || !f.Nullable) && f.Role != roleDept {
			f.Sample = f.sample()
			d.SampleFields = append(d.SampleFields, f)
		}
		f.TSFromView = tsFromView(f)
		f.MinWidth = 120
		if f.Kind == kindString && (f.MaxLen == 0 || f.MaxLen > 64) {
			f.MinWidth = 180
		}
		f.FixedWidth = fixedWidth(f.Label)
		if f == d.CreatedAt {
			continue // appended last
		}
		f.InTable = tableColumn(f) && shown < 8
		if f.listOverride != nil {
			f.InTable = *f.listOverride && (f.Role != roleID || f.CreateOnly)
		}
		if f.InTable {
			shown++
			d.TableFields = append(d.TableFields, f)
		}
	}
	if d.CreatedAt != nil {
		d.CreatedAt.InTable = d.CreatedAt.listOverride == nil || *d.CreatedAt.listOverride
		if d.CreatedAt.InTable {
			d.TableFields = append(d.TableFields, d.CreatedAt)
		}
	}
	d.StatusInTable = d.Status != nil && d.Status.InTable
	for _, f := range d.Fields {
		if exported(f, f == d.PK) {
			d.ExportFields = append(d.ExportFields, f)
		}
	}
	for _, f := range d.TableFields {
		d.TableHasTime = d.TableHasTime || f.Kind == kindTime
	}
}

// exported reports whether the export has f: by default the key and the
// list columns; FieldOption.Export overrides it.
func exported(f *Field, key bool) bool {
	switch {
	case f.Role == roleVersion || f.Role == roleDeletedAt:
		return false
	case f.exportOverride != nil:
		return *f.exportOverride
	}
	return key || f.InTable
}

func tableColumn(f *Field) bool {
	if f.CreateOnly {
		return true // a typed-in key names the row
	}
	switch f.Role {
	case roleID, roleVersion, roleCreatedBy, roleUpdatedBy, roleDeletedAt, roleUpdatedAt, roleCreatedAt:
		return false
	}
	// Unbounded nullable text is usually a long note; NOT NULL text (a
	// PostgreSQL title or name) and text with comment options are shown.
	return f.Kind != kindJSON && (f.Kind != kindString || f.MaxLen > 0 || !f.Nullable || len(f.commentEnum) > 0)
}

func tsFromView(f *Field) string {
	if f.Upload == UploadFiles {
		return "[...r." + f.JSONName + "]"
	}
	v := "r." + f.JSONName
	switch {
	case !f.Nullable || f.Role == roleDept:
		return v
	case f.Widget == "number":
		return v + " ?? undefined"
	case f.Kind == kindBool:
		return v + " ?? false"
	default:
		return v + ` ?? ""`
	}
}

// deriveDisplay picks the column that names a row: a unique text column,
// else the first required text column.
func (d *tmplData) deriveDisplay() {
	for _, group := range [][]*Field{d.UniqueFields, d.RequiredFields} {
		for _, f := range group {
			if f.Kind == kindString && !f.Nullable {
				d.DisplayField = f
				return
			}
		}
	}
}

func (d *tmplData) permissions() []perm.Def {
	out := make([]perm.Def, 0, 5)
	actions := []struct{ action, verb string }{
		{"view", "查看"}, {"create", "新增"}, {"update", "编辑"}, {"delete", "删除"}, {"export", "导出"},
	}
	if d.ReadOnly {
		actions = []struct{ action, verb string }{{"view", "查看"}, {"export", "导出"}}
	}
	for _, a := range actions {
		out = append(out, perm.Def{Code: d.PermPrefix + ":" + a.action, Name: a.verb + d.Name, Menu: d.MenuCode})
	}
	return out
}

func (d *tmplData) menus() []menu.Def {
	var out []menu.Def
	if d.NewModule {
		out = append(out, menu.Def{Code: d.ParentMenu, Type: menu.TypeDir, Title: d.ModuleTitle, Path: "/" + d.Pkg, Sort: 100})
	}
	return append(out, menu.Def{
		Code: d.MenuCode, Parent: d.ParentMenu, Type: menu.TypeMenu, Title: d.Name + "管理", Path: d.PagePath,
		Component: d.Component, Permission: d.PermPrefix + ":view",
	})
}

func (d *tmplData) wiring() string {
	return fmt.Sprintf("// server/internal/app/modules.go\nimport \"%s/internal/modules/%s\"\n\n"+
		"mods := []module.Module{\n\t// ...\n\t%s.New(%s.Deps{DB: d.db, Dict: systemModule}),\n}", goModulePath, d.Pkg, d.Pkg, d.Pkg)
}

func (d *tmplData) notes() []string {
	notes := append([]string(nil), d.skipped...)
	if !d.NewModule {
		notes = append(notes, fmt.Sprintf("模块 %s 已存在：「生成到项目」会把新资源加到它的 module.go 中（该文件需含 codegen 标记）", d.Pkg))
	}
	notes = append(notes,
		"表结构必须由业务迁移创建（PostgreSQL：server/migrations/app，MySQL：server/migrations/app/mysql），集成测试才能通过",
		"前端路由由后端菜单动态下发，无需修改 web/src/router",
	)
	if d.Scope {
		notes = append(notes, "已按数据权限过滤（"+strings.Join(nonEmpty(d.ScopeDept, d.ScopeOwner), "、")+"）")
	}
	switch {
	case d.ReadOnly:
		notes = append(notes, "只读模块：只生成列表、详情和导出，没有表单和新增、编辑、删除接口")
	case d.KeyGenerated:
		notes = append(notes, "主键 "+d.PK.Column+" 由程序生成：新增时调用模块的 NextID（Deps.NewID，默认 ids.Next 雪花 ID）；"+
			"与其他服务共用该表时，在 modules.go 里传入同一个 ID 生成器")
	case d.KeyInput:
		notes = append(notes, "主键 "+d.PK.Column+" 在新增时填写，之后不能修改")
	}
	for _, c := range d.Children {
		notes = append(notes, fmt.Sprintf("子表 %s 随%s一起编辑：表单按%s的每一项列出一行，全部为空的行不保存；详情接口返回 %s",
			c.Table, d.Name, c.KeySourceLabel(), c.JSONName))
	}
	if missing := d.missingStandard(); len(missing) > 0 {
		notes = append(notes, "表缺少标准字段（见 docs/table-guide.md）："+strings.Join(missing, "；"))
	}
	return notes
}

// standardColumns are the columns every business table should have, with
// what is lost without them.
var standardColumns = []struct{ name, loss string }{
	{"version", "version（编辑时无法发现他人已修改）"},
	{"created_at", "created_at（无法按创建时间排序、追溯）"},
	{"updated_at", "updated_at（无法知道最后修改时间）"},
	{"created_by", "created_by（不记录创建人，数据权限「仅本人」不生效）"},
	{"updated_by", "updated_by（不记录修改人）"},
}

func (d *tmplData) missingStandard() []string {
	var out []string
	for _, c := range standardColumns {
		if !d.hasColumn(c.name) {
			out = append(out, c.loss)
		}
	}
	return out
}

func (d *tmplData) manualWiring() string {
	return fmt.Sprintf("请在 %s/module.go 中把 %sPermissions 加入 Permissions()、%sMenus 加入 Menus()，"+
		"并在 Routes() 中调用 m.%sRoutes(r)", d.serverDir, d.Lower, d.Lower, d.Lower)
}

func nonEmpty(vals ...string) []string {
	var out []string
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// fixedWidth is the width of a tag column (status, booleans): wide enough
// for its header on one line, at least 90px.
func fixedWidth(label string) int {
	return max(90, utf8.RuneCountInString(label)*14+34)
}

// selectList builds the Go expression of a select list: string literals
// joined with calls that the database package renders per dialect.
type selectList struct {
	segs []string // Go expressions
	lit  strings.Builder
	n    int
}

// add appends a column written in SQL.
func (l *selectList) add(sql string) { l.call(sql, "", "") }

// call appends prefix + the Go call's result + suffix as one column.
func (l *selectList) call(prefix, goCall, suffix string) {
	if l.n > 0 {
		l.lit.WriteString(", ")
	}
	l.n++
	l.lit.WriteString(prefix)
	if goCall != "" {
		l.flush()
		l.segs = append(l.segs, goCall)
	}
	l.lit.WriteString(suffix)
}

func (l *selectList) flush() {
	if l.lit.Len() > 0 {
		lit := l.lit.String()
		if strings.Contains(lit, "`") {
			lit = strconv.Quote(lit)
		} else {
			lit = "`" + lit + "`"
		}
		l.segs = append(l.segs, lit)
		l.lit.Reset()
	}
}

func (l *selectList) goExpr() string {
	l.flush()
	if len(l.segs) == 0 {
		return "``"
	}
	return strings.Join(l.segs, " + ")
}
