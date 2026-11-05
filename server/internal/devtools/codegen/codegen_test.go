package codegen

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func customerSpec() Spec {
	return Spec{Module: "example", Resource: "customer", Name: "客户", ModuleTitle: "示例业务", NewModule: true}
}

func TestNaming(t *testing.T) {
	cases := []struct{ in, exported, unexported, lower, upper, plural string }{
		{"customer", "Customer", "customer", "customer", "Customer", "customers"},
		{"sales-order", "SalesOrder", "salesOrder", "salesOrder", "SalesOrder", "sales-orders"},
		{"dept_id", "DeptID", "deptID", "deptId", "DeptId", "dept_ids"},
		{"api-key", "APIKey", "apiKey", "apiKey", "ApiKey", "api-keys"},
		{"category", "Category", "category", "category", "Category", "categories"},
		{"box", "Box", "box", "box", "Box", "boxes"},
		{"day", "Day", "day", "day", "Day", "days"},
		{"client_ip", "ClientIP", "clientIP", "clientIp", "ClientIp", "client_ips"},
	}
	for _, c := range cases {
		assert.Equal(t, c.exported, goExported(c.in), c.in)
		assert.Equal(t, c.unexported, goUnexported(c.in), c.in)
		assert.Equal(t, c.lower, lowerCamel(c.in), c.in)
		assert.Equal(t, c.upper, upperCamel(c.in), c.in)
		assert.Equal(t, c.plural, plural(c.in), c.in)
	}
	assert.Equal(t, `"order"`, quoteIdent("order"))
	assert.Equal(t, "name", quoteIdent("name"))
	assert.Equal(t, "客户名称", label(Column{Name: "name", Comment: "客户名称，必填"}))
	assert.Equal(t, "名称", label(Column{Name: "name"}))
	assert.Equal(t, "foo_bar", label(Column{Name: "foo_bar"}))
	assert.Equal(t, `"a\"b"`, tsString(`a"b`))
	assert.Equal(t, "a'b c", structTagValue("a\"b\nc"))
}

func TestTypeMapping(t *testing.T) {
	cases := []struct {
		col      Column
		kind     kind
		viewType string
		reqType  string
		binding  string
		ts       string
		widget   string
	}{
		{Column{Name: "name", UDT: "varchar", MaxLength: 64}, kindString, "string", "string", "required,max=64", "string", "input"},
		{Column{Name: "note", UDT: "text", Nullable: true}, kindString, "*string", "*string", "", "string | null", "textarea"},
		{Column{Name: "contact", UDT: "varchar", MaxLength: 32, Default: "''::character varying"}, kindString, "string", "string", "max=32", "string", "input"},
		{Column{Name: "owner_id", UDT: "int8", Nullable: true}, kindID, "*ids.ID", "*ids.ID", "", "string | null", "input"},
		{Column{Name: "customer_id", UDT: "int8"}, kindID, "ids.ID", "ids.ID", "required", "string", "input"},
		{Column{Name: "big", UDT: "int8"}, kindInt64, "int64", "int64", "", "number", "number"},
		{Column{Name: "qty", UDT: "int4", Nullable: true}, kindInt, "*int", "*int", "", "number | null", "number"},
		{Column{Name: "amount", UDT: "numeric"}, kindDecimal, "string", "string", "required,numeric", "string", "input"},
		{Column{Name: "paid", UDT: "bool", Default: "false"}, kindBool, "bool", "bool", "", "boolean", "switch"},
		{Column{Name: "due_on", UDT: "date", Nullable: true}, kindDate, "*string", "*string", "omitempty,datetime=2006-01-02", "string | null", "date"},
		{Column{Name: "at", UDT: "timestamptz"}, kindTime, "time.Time", "time.Time", "required", "string", "datetime"},
		{Column{Name: "ref", UDT: "uuid", Nullable: true}, kindUUID, "*string", "*string", "omitempty,uuid", "string | null", "input"},
		{Column{Name: "meta", UDT: "jsonb"}, kindJSON, "string", "string", "required,json", "string", "textarea"},
		{Column{Name: "status", UDT: "int2", Default: "1"}, kindInt16, "int16", "int16", "omitempty,oneof=1 2", "Status", "status"},
		{Column{Name: "dept_id", UDT: "int8", Nullable: true}, kindID, "*ids.ID", "*ids.ID", "", "string | null", "dept"},
	}
	for _, c := range cases {
		ti, ok := mapType(c.col)
		require.True(t, ok, c.col.Name)
		f := newField(c.col, ti)
		assert.Equal(t, c.kind, f.Kind, c.col.Name)
		assert.Equal(t, c.viewType, f.ViewType, c.col.Name)
		assert.Equal(t, c.reqType, f.ReqType, c.col.Name)
		assert.Equal(t, c.binding, f.Binding, c.col.Name)
		assert.Equal(t, c.ts, f.TSType, c.col.Name)
		assert.Equal(t, c.widget, f.Widget, c.col.Name)
		assert.True(t, f.Writable, c.col.Name)
	}
	_, ok := mapType(Column{Name: "tags", UDT: "_text"})
	assert.False(t, ok, "arrays are unsupported")

	uuid := newField(Column{Name: "code", UDT: "uuid", Default: "gen_random_uuid()"}, typeInfo{kind: kindUUID, goType: "string", castText: true})
	assert.False(t, uuid.Writable, "database-defaulted uuid is left to the database")
	stamp := newField(Column{Name: "paid_at", UDT: "timestamptz", Default: "now()"}, typeInfo{kind: kindTime, goType: "time.Time"})
	assert.False(t, stamp.Writable, "a computed default is left to the database")

	// A literal default keeps the column editable and pre-fills the form.
	amount := newField(Column{Name: "amount", UDT: "numeric", Default: "0"}, typeInfo{kind: kindDecimal, goType: "string", castText: true})
	assert.True(t, amount.Writable, "amount DEFAULT 0 is editable")
	assert.Equal(t, `"0"`, amount.TSDefault)
	meta := newField(Column{Name: "meta", UDT: "jsonb", Default: "'{}'::jsonb"}, typeInfo{kind: kindJSON, goType: "string", castText: true})
	assert.True(t, meta.Writable)
	assert.Equal(t, `"{}"`, meta.TSDefault)
	assert.Equal(t, castText, uuid.SelectCast)
	date := newField(Column{Name: "order", UDT: "date"}, typeInfo{kind: kindDate, goType: "string", castText: true})
	assert.Equal(t, castDate, date.SelectCast)
	assert.Equal(t, `t."order"`, date.SelectExpr)
	for _, role := range []string{"version", "created_at", "created_by", "deleted_at"} {
		c := Column{Name: role, UDT: map[string]string{"version": "int4", "created_at": "timestamptz", "created_by": "int8", "deleted_at": "timestamptz"}[role], Nullable: role == "deleted_at" || role == "created_by"}
		ti, _ := mapType(c)
		f := newField(c, ti)
		assert.Equal(t, role, f.Role)
		assert.False(t, f.Writable, role)
	}
}

func TestParseDefault(t *testing.T) {
	for in, want := range map[string]string{
		"'abc'::character varying": "abc",
		"'it''s'::text":            "it's",
		"0":                        "0",
		"(-1)":                     "-1",
		"true":                     "true",
		"1.5::numeric":             "1.5",
	} {
		got, ok := parseDefault(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{"", "now()", "gen_random_uuid()", "nextval('x'::regclass)"} {
		_, ok := parseDefault(in)
		assert.False(t, ok, in)
	}
}

func TestSpecValidation(t *testing.T) {
	require.NoError(t, customerSpec().Validate())
	bad := []Spec{
		{Module: "Example", Resource: "customer", Name: "客户"},
		{Module: "iam", Resource: "customer", Name: "客户"},
		{Module: "ex-ample", Resource: "customer", Name: "客户"},
		{Module: "example", Resource: "Customer", Name: "客户"},
		{Module: "example", Resource: "cust_omer", Name: "客户"},
		{Module: "example", Resource: "../x", Name: "客户"},
		{Module: "example", Resource: "customer", Name: ""},
		{Module: "example", Resource: "customer", Name: `客"户`},
		{Module: "example", Resource: "customer", Name: "客户", ParentMenu: "iam.system"},
	}
	for _, s := range bad {
		assert.Error(t, s.Validate(), "%+v", s)
	}
}

func TestCheckTable(t *testing.T) {
	require.NoError(t, CheckTable(customerTable()))
	require.NoError(t, CheckTable(minimalTable()), "bigserial ids are accepted")
	require.NoError(t, CheckTable(func() Table {
		tb := customerTable()
		tb.Columns = append([]Column(nil), tb.Columns...)
		tb.Columns[0].Identity = false
		return tb
	}()), "a bigint id the program generates")
	mutate := func(fn func(*Table)) Table {
		tb := customerTable()
		tb.Columns = append([]Column(nil), tb.Columns...)
		fn(&tb)
		return tb
	}
	cases := map[string]Table{
		"no id":         {Name: "x", Columns: []Column{{Name: "name", UDT: "text"}}},
		"no pk":         mutate(func(t *Table) { t.Columns[0].Primary = false }),
		"uuid pk":       mutate(func(t *Table) { t.Columns[0].UDT = "uuid" }),
		"composite pk":  mutate(func(t *Table) { t.Columns[1].Primary = true }),
		"bad table":     mutate(func(t *Table) { t.Name = "Bad-Table" }),
		"reserved name": mutate(func(t *Table) { t.Name = "order" }),
		"bad column":    mutate(func(t *Table) { t.Columns[1].Name = "Name" }),
		"required unsupported": mutate(func(t *Table) {
			t.Columns = append(t.Columns, Column{Name: "tags", UDT: "_text"})
		}),
	}
	for name, tb := range cases {
		assert.Error(t, CheckTable(tb), name)
	}
}

func fileByPath(t *testing.T, res Result, suffix string) File {
	t.Helper()
	for _, f := range res.Files {
		if strings.HasSuffix(f.Path, suffix) {
			return f
		}
	}
	t.Fatalf("no generated file ends with %s", suffix)
	return File{}
}

func TestGenerateCustomer(t *testing.T) {
	res, err := Generate(customerTable(), customerSpec())
	require.NoError(t, err)
	var paths []string
	for _, f := range res.Files {
		paths = append(paths, f.Path)
	}
	assert.Equal(t, []string{
		"server/internal/modules/example/module.go",
		"server/internal/modules/example/common.go",
		"server/internal/modules/example/customer.go",
		"server/internal/modules/example/customer_ext.go",
		"server/internal/modules/example/customer_integration_test.go",
		"web/src/api/example/customer.ts",
		"web/src/views/example/customer/index.vue",
		"web/src/views/example/customer/components/CustomerExtension.vue",
		"web/src/views/example/customer/components/CustomerFormDialog.vue",
	}, paths)
	for _, f := range res.Files {
		assert.Equal(t, strings.HasSuffix(f.Path, "_ext.go") || strings.HasSuffix(f.Path, "Extension.vue"), f.Keep, f.Path)
		assert.Equal(t, strings.HasSuffix(f.Path, "/common.go"), f.Refresh, f.Path)
	}

	ext := fileByPath(t, res, "/customer_ext.go").Content
	assert.Contains(t, ext, "func (m *Module) customerRestrict(ctx context.Context, q *gorm.DB) *gorm.DB {")
	assert.Contains(t, ext, "func (m *Module) customerAfterSave(ctx context.Context, tx *gorm.DB, row customerView, created bool) error {")
	assert.Contains(t, ext, "func (m *Module) customerAfterDelete(ctx context.Context, tx *gorm.DB, key int64) error {")

	src := fileByPath(t, res, "/customer.go").Content
	for _, want := range []string{
		`var customerScope = datascope.Spec{DeptColumn: "t.dept_id", OwnerColumn: "t.created_by"}`,
		`q = q.Where("t.deleted_at IS NULL")`,
		`q = q.Joins("LEFT JOIN sys_departments d ON d.id = t.dept_id")`,
		`res := tx.Table(customerTable).Where("id = ? AND version = ?", key, *req.Version).Updates(sets)`,
		// A duplicate is a field error on the unique column, not a 409.
		`{column: "phone", field: "phone", label: "手机号"},`,
		`return duplicateError(constraint, customerUniqueFields, "客户已存在")`,
		// The export shows the department name, not its ID.
		`csvValue(r.DeptName), // the department name instead of its ID`,
		`Name    string  ` + "`" + `json:"name" binding:"required,max=64" doc:"客户名称，必填"` + "`",
		`DeptID  *ids.ID ` + "`" + `json:"deptId" doc:"部门"` + "`",
		"UPDATE biz_customers SET deleted_at = CURRENT_TIMESTAMP(6), updated_by = ?, version = version + 1, updated_at = CURRENT_TIMESTAMP(6) WHERE id = ? AND deleted_at IS NULL",
		`INSERT INTO biz_customers (name, contact, phone, level, dept_id, owner_id, status, sort, remark, created_by, updated_by)`,
		// Default search: unique text first, then the next text column, and status.
		`q = q.Where("LOWER(t.phone) LIKE LOWER(?)", "%"+escapeLike(v)+"%")`,
		`q = q.Where("LOWER(t.name) LIKE LOWER(?)", "%"+escapeLike(v)+"%")`,
		`q = q.Where("t.status = ?", f.Status)`,
		`Order("t.sort, t.id DESC")`,
		`g := r.Group("/customers")`,
		`Response: httpx.Page[customerView]{}`,
		`Sensitive: true`,
		`customerStatusLabels[int(r.Status)]`,
		`fieldError("deptId", "所选部门不在您的数据范围内")`,
	} {
		assert.Contains(t, src, want)
	}
	assert.NotContains(t, src, "DeletedAt", "deleted_at is not exposed")

	module := fileByPath(t, res, "/module.go").Content
	assert.Contains(t, module, `{Code: "example.root", Type: menu.TypeDir, Title: "示例业务", Path: "/example"`)
	assert.Contains(t, module, "m.customerRoutes(r)")

	api := fileByPath(t, res, "/api/example/customer.ts").Content
	assert.Contains(t, api, "export type CustomerUpdatePayload = CustomerPayload & { version: number };")
	assert.Contains(t, api, `http.download("/customers/export", compactParams(filter), "customers.csv")`)
	index := fileByPath(t, res, "/customer/index.vue").Content
	assert.Contains(t, index, "确定删除客户「${row.phone}」吗？", "rows are named by a unique column")
	assert.Contains(t, index, `defineOptions({ name: "ExampleCustomer" });`)
	assert.Contains(t, index, `v-perm="'example:customer:export'"`)
	assert.Contains(t, index, `<ReDateTime v-else :value="row.createdAt" />`, "hover shows the other time zone")
	form := fileByPath(t, res, "CustomerFormDialog.vue").Content
	assert.Contains(t, form, `:validate-on-rule-change="false"`)
	assert.Contains(t, form, "version: props.row.version")
	assert.Contains(t, form, `name: [`+"\n"+`    { required: true, message: "请输入客户名称", trigger: "blur" }`)

	assert.Len(t, res.Permissions, 5)
	assert.Equal(t, "example:customer:export", res.Permissions[4].Code)
	require.Len(t, res.Menus, 2)
	assert.Equal(t, "example/customer/index", res.Menus[1].Component)
	assert.Contains(t, res.Wiring, "example.New(example.Deps{DB: d.db, Dict: systemModule}),")
}

func TestGenerateWithoutOptionalColumns(t *testing.T) {
	res, err := Generate(minimalTable(), Spec{Module: "notes", Resource: "note", Name: "笔记"})
	require.NoError(t, err)
	assert.Len(t, res.Files, 8, "an existing module gets no module.go")
	common := fileByPath(t, res, "/common.go")
	assert.True(t, common.Refresh, "common.go is brought up to date in an existing module")
	src := fileByPath(t, res, "/note.go").Content
	assert.NotContains(t, src, "datascope")
	assert.NotContains(t, src, "updateVersioned")
	assert.NotContains(t, src, "core/actor")
	assert.Contains(t, src, "DELETE FROM biz_notes WHERE id = ?")
	assert.Contains(t, src, `tx.Table(noteTable).Where("id = ?", key).Updates(sets)`)
	assert.Contains(t, src, `Order("t.id DESC")`)
	assert.Contains(t, src, `q = q.Where("LOWER(t.body) LIKE LOWER(?)", "%"+escapeLike(v)+"%")`)
	api := fileByPath(t, res, "/api/notes/note.ts").Content
	assert.NotContains(t, api, "UpdatePayload")
	assert.Contains(t, strings.Join(res.Notes, "\n"), "模块 notes 已存在")
}

func TestGenerateSkipsUnsupportedAndSensitiveColumns(t *testing.T) {
	res, err := Generate(mixedTable(), Spec{Module: "shop", Resource: "sales-order", Name: "订单", NewModule: true})
	require.NoError(t, err)
	src := fileByPath(t, res, "/sales_order.go").Content
	assert.NotContains(t, src, "Tags")
	assert.NotContains(t, src, "PasswordHash")
	assert.Contains(t, src, `t."order"`)
	assert.Contains(t, src, "int(actor.StatusEnabled)", "integer status columns convert the constant")
	assert.Contains(t, src, "VALUES (?, ?, ?, ?, COALESCE(?, true), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP(6))", "created_at without default is set")
	notes := strings.Join(res.Notes, "\n")
	assert.Contains(t, notes, "tags")
	assert.Contains(t, notes, "password_hash")
}

func TestGenerateRejectsInvalidInput(t *testing.T) {
	_, err := Generate(customerTable(), Spec{Module: "iam", Resource: "x", Name: "x"})
	assert.Error(t, err)
	_, err = Generate(Table{Name: "x"}, customerSpec())
	assert.Error(t, err)
}

func TestWriteRefusesToOverwrite(t *testing.T) {
	root := t.TempDir()
	files := []File{{Path: "a/b.txt", Content: "new"}, {Path: "c.txt", Content: "new"}}
	require.NoError(t, os.WriteFile(filepath.Join(root, "c.txt"), []byte("old"), 0o600))

	err := Write(root, files, false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrExists))
	assert.Contains(t, err.Error(), "c.txt")
	_, statErr := os.Stat(filepath.Join(root, "a", "b.txt"))
	assert.True(t, os.IsNotExist(statErr), "nothing is written when any file exists")

	require.NoError(t, Write(root, files, true))
	got, err := os.ReadFile(filepath.Join(root, "c.txt"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))

	existing, err := Existing(root, files)
	require.NoError(t, err)
	assert.Equal(t, []string{"a/b.txt", "c.txt"}, existing)

	assert.Error(t, Write(root, []File{{Path: "../escape.txt"}}, true))
}

// Extension files are written once and kept, with or without force.
func TestWriteKeepsExtensionFiles(t *testing.T) {
	root := t.TempDir()
	files := []File{{Path: "x.go", Content: "generated"}, {Path: "x_ext.go", Content: "stub", Keep: true}}
	require.NoError(t, Write(root, files, false))
	got, err := os.ReadFile(filepath.Join(root, "x_ext.go"))
	require.NoError(t, err)
	assert.Equal(t, "stub", string(got), "written when missing")

	require.NoError(t, os.WriteFile(filepath.Join(root, "x_ext.go"), []byte("hand-written"), 0o600))
	kept, err := Kept(root, files)
	require.NoError(t, err)
	assert.Equal(t, []string{"x_ext.go"}, kept)
	conflicts, err := Conflicts(root, files)
	require.NoError(t, err)
	assert.Equal(t, []string{"x.go"}, conflicts, "a kept file is no conflict")

	require.NoError(t, Write(root, files[1:], false), "only the kept file: nothing to refuse")

	// A Refresh file is rewritten, with or without force, and never a conflict.
	helpers := []File{{Path: "common.go", Content: "v1", Refresh: true}}
	require.NoError(t, Write(root, helpers, false))
	helpers[0].Content = "v2"
	require.NoError(t, Write(root, helpers, false))
	got, err = os.ReadFile(filepath.Join(root, "common.go"))
	require.NoError(t, err)
	assert.Equal(t, "v2", string(got))
	require.NoError(t, Write(root, files, true))
	got, err = os.ReadFile(filepath.Join(root, "x_ext.go"))
	require.NoError(t, err)
	assert.Equal(t, "hand-written", string(got), "never overwritten, even with force")
}

// TestWriteRollsBack: a failing write undoes the files written before it.
func TestWriteRollsBack(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "c.txt"), []byte("old"), 0o600))
	files := []File{{Path: "a/b.txt", Content: "new"}, {Path: "c.txt", Content: "new"}, {Path: "../escape.txt"}}

	require.Error(t, Write(root, files, true))
	assert.NoFileExists(t, filepath.Join(root, "a", "b.txt"), "new file removed")
	got, err := os.ReadFile(filepath.Join(root, "c.txt"))
	require.NoError(t, err)
	assert.Equal(t, "old", string(got), "overwritten file restored")
}

func TestFindRepoRoot(t *testing.T) {
	root, err := FindRepoRoot(".")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(root, "server", "go.mod"))
	assert.True(t, ModuleExists(root, "iam"))
	assert.False(t, ModuleExists(root, "no-such-module"))
	_, err = FindRepoRoot(t.TempDir())
	assert.Error(t, err)
}

func TestLabelStripsUnsafeCharacters(t *testing.T) {
	assert.Equal(t, "名称+evil", label(Column{Name: "name", Comment: "名称\"+evil()+\""}))
	assert.Equal(t, "备注", label(Column{Name: "x", Comment: "备注*/"}))
	assert.Equal(t, "ax", label(Column{Name: "x", Comment: "a`${x}\\"}))
	assert.Equal(t, "备注", label(Column{Name: "remark", Comment: `"'`}), "nothing left: standard label")
}

func TestFixedWidthFitsLabel(t *testing.T) {
	assert.Equal(t, 90, fixedWidth("状态"))
	assert.Equal(t, 104, fixedWidth("是否已付款"))
}

// TestCreateKeepsDefaults: an optional column whose default differs from
// the Go zero value is a pointer in the request, so a create that omits it
// gets the database default and an update keeps the current value.
func TestCreateKeepsDefaults(t *testing.T) {
	tb := ticketTable()
	tb.Columns = append(tb.Columns,
		Column{Name: "channel", UDT: "varchar", MaxLength: 16, Nullable: true, Default: "'web'::character varying"},
		Column{Name: "notify", UDT: "bool", Default: "true"},
		Column{Name: "muted", UDT: "bool", Default: "false"},
		Column{Name: "note", UDT: "varchar", MaxLength: 16, Default: "'a?b'::character varying"},
	)
	d := buildData(tb, Spec{Module: "desk", Resource: "ticket", Name: "工单"})
	got := map[string][3]string{}
	for _, f := range d.Writable {
		got[f.Column] = [3]string{f.ReqType, f.InsertVal, f.UpdateArg}
	}
	assert.Equal(t, [3]string{"*int16", "COALESCE(?, 1)", `gorm.Expr("COALESCE(?, kind)", req.Kind)`}, got["kind"])
	assert.Equal(t, [3]string{"*string", "COALESCE(?, 'web')", "req.Channel"}, got["channel"])
	assert.Equal(t, [3]string{"*bool", "COALESCE(?, true)", `gorm.Expr("COALESCE(?, notify)", req.Notify)`}, got["notify"])
	assert.Equal(t, [3]string{"bool", "?", "req.Muted"}, got["muted"], "zero value already is the default")
	assert.Equal(t, [3]string{"string", "?", "req.Note"}, got["note"], "? would be a GORM placeholder")
	assert.Equal(t, [3]string{"string", "?", "req.Title"}, got["title"])
}

func TestMenuTitleAndSort(t *testing.T) {
	spec := customerSpec()
	res, err := Generate(customerTable(), spec)
	require.NoError(t, err)
	assert.Contains(t, fileByPath(t, res, "/customer.go").Content, `Title: "客户管理"`)
	assert.Contains(t, fileByPath(t, res, "/customer.go").Content, "Sort: 1,")

	sort := 30
	spec.MenuTitle, spec.MenuSort = "客户查询", &sort
	res, err = Generate(customerTable(), spec)
	require.NoError(t, err)
	src := fileByPath(t, res, "/customer.go").Content
	assert.Contains(t, src, `Title: "客户查询"`)
	assert.Contains(t, src, "Sort: 30,")
	page := res.Menus[len(res.Menus)-1]
	assert.Equal(t, "客户查询", page.Title)
	assert.Equal(t, 30, page.Sort)

	bad := -1
	spec.MenuTitle, spec.MenuSort = `客户"`, &bad
	var fields []string
	for _, p := range spec.Problems() {
		fields = append(fields, p.Field)
	}
	assert.Equal(t, []string{"menuTitle", "menuSort"}, fields)
}

func TestAfterCommitHook(t *testing.T) {
	res, err := Generate(customerTable(), customerSpec())
	require.NoError(t, err)
	src := fileByPath(t, res, "/customer.go").Content
	assert.Contains(t, src, "m.customerCommitted(ctx, opCreate, key, &view)")
	assert.Contains(t, src, "m.customerCommitted(ctx, opUpdate, key, &view)")
	assert.Contains(t, src, "m.customerCommitted(ctx, opDelete, key, nil)")
	assert.Contains(t, src, "h, ok := any(m).(customerCommitter)", "optional: older extension files lack it")
	assert.Contains(t, fileByPath(t, res, "/customer_ext.go").Content,
		"func (m *Module) customerAfterCommit(ctx context.Context, op string, key int64, row *customerView) error {")
	assert.Contains(t, fileByPath(t, res, "/common.go").Content, "func afterCommitFailed(")

	ro, err := Generate(orderLogTable(), Spec{Module: "ledger", Resource: "order-log", Name: "订单流水", NewModule: true, ReadOnly: true})
	require.NoError(t, err)
	assert.NotContains(t, fileByPath(t, ro, "/order_log_ext.go").Content, "AfterCommit")
	assert.NotContains(t, fileByPath(t, ro, "/order_log.go").Content, "Committed")
}

// Generated writes fill 操作对象 of the operation log: create, update and
// delete call set<X>Target; an auto key is only known after the insert.
func TestGeneratedWritesSetAuditTarget(t *testing.T) {
	res, err := Generate(customerTable(), customerSpec())
	require.NoError(t, err)
	src := fileByPath(t, res, "/customer.go").Content
	assert.Contains(t, src, `"base-admin/server/internal/modules/audit"`)
	assert.Contains(t, src, "func setCustomerTarget(c *gin.Context, key int64) {\n\taudit.SetTarget(c, fmt.Sprintf(\"customer:%v\", key))\n}")
	for _, fn := range []string{"createCustomer", "updateCustomer", "deleteCustomer"} {
		body := src[strings.Index(src, "func (m *Module) "+fn+"("):]
		body = body[:strings.Index(body, "\n}\n")]
		assert.Equal(t, 1, strings.Count(body, "setCustomerTarget(c, key)"), fn)
	}

	ro, err := Generate(orderLogTable(), Spec{Module: "ledger", Resource: "order-log", Name: "订单流水", NewModule: true, ReadOnly: true})
	require.NoError(t, err)
	assert.NotContains(t, fileByPath(t, ro, "/order_log.go").Content, "audit")
}
