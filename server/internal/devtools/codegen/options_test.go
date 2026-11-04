package codegen

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDescribeDefaults(t *testing.T) {
	infos, err := Describe(customerTable())
	require.NoError(t, err)
	by := map[string]FieldInfo{}
	for _, f := range infos {
		by[f.Column] = f
	}
	assert.Equal(t, "客户名称", by["name"].Label)
	assert.Equal(t, "客户名称，必填", by["name"].Comment)
	assert.Equal(t, "文本", by["name"].Type)
	assert.True(t, by["name"].FormLocked, "NOT NULL without a default must be in the form")
	assert.False(t, by["contact"].FormLocked)
	assert.True(t, by["version"].FormLocked)
	assert.False(t, by["version"].Form)
	assert.True(t, by["status"].FormLocked, "role columns stay in the form")

	assert.Equal(t, SearchLike, by["phone"].Search, "unique text is searched first")
	assert.Equal(t, SearchLike, by["name"].Search)
	assert.Equal(t, SearchNone, by["contact"].Search, "at most two text columns by default")
	assert.Equal(t, SearchEq, by["status"].Search)
	assert.Equal(t, []string{SearchNone, SearchLike, SearchEq}, by["name"].SearchModes)
	assert.Equal(t, []string{SearchNone, SearchEq}, by["dept_id"].SearchModes)
	assert.Equal(t, []string{SearchNone, SearchRange}, by["created_at"].SearchModes)
	assert.Equal(t, []string{SearchNone}, by["version"].SearchModes)
	_, deleted := by["deleted_at"]
	assert.False(t, deleted, "soft delete column is not listed")
	_, id := by["id"]
	assert.False(t, id, "the primary key is not listed")
}

func TestFieldProblems(t *testing.T) {
	yes, no := true, false
	spec := Spec{Fields: []FieldOption{
		{Column: "nope"},
		{Column: "name", Form: &no},
		{Column: "status", Search: SearchLike},
		{Column: "contact", Label: `联系"人`},
		{Column: "remark", List: &yes},
		{Column: "remark"},
	}}
	var msgs []string
	for _, p := range spec.FieldProblems(customerTable()) {
		msgs = append(msgs, p.Message)
	}
	assert.Equal(t, []string{
		"字段 nope 不存在或不会生成",
		"字段 name 的「表单」不能修改",
		"字段 status 不支持「模糊」搜索",
		"字段 contact 的中文名最长 20 个字符，且不能包含引号、反斜杠、尖括号或花括号",
		"字段 remark 重复配置",
	}, msgs)

	_, err := Generate(customerTable(), Spec{Module: "crm", Resource: "customer", Name: "客户", NewModule: true,
		Fields: []FieldOption{{Column: "nope"}}})
	assert.ErrorContains(t, err, "字段 nope 不存在")
}

func TestFieldOptionsApplied(t *testing.T) {
	yes, no := true, false
	spec := Spec{Module: "crm", Resource: "customer", Name: "客户", NewModule: true, Fields: []FieldOption{
		{Column: "contact", Label: "联系人姓名", Search: SearchEq, Form: &no},
		{Column: "phone", Search: SearchNone},
		{Column: "name", Search: SearchNone},
		{Column: "remark", List: &no},
		{Column: "dept_id", Search: SearchEq},
		{Column: "created_at", Search: SearchRange},
		{Column: "status", List: &yes},
	}}
	res, err := Generate(customerTable(), spec)
	require.NoError(t, err)
	src := fileByPath(t, res, "/customer.go").Content
	index := fileByPath(t, res, "/customer/index.vue").Content
	form := fileByPath(t, res, "Dialog.vue").Content
	api := fileByPath(t, res, "/api/crm/customer.ts").Content

	// Search: exact contact, department, created_at range; phone and name off.
	assert.Contains(t, src, `form:"contact" binding:"max=64" doc:"联系人姓名"`)
	assert.Contains(t, src, `q = q.Where("t.contact = ?", v)`)
	assert.Contains(t, src, `q = q.Where("t.dept_id = ?", f.DeptID)`)
	assert.Contains(t, src, `q = q.Where("t.created_at >= ?", d)`)
	assert.Contains(t, src, `q = q.Where("t.created_at < ?", d.AddDate(0, 0, 1))`)
	assert.NotContains(t, src, "LOWER(t.phone) LIKE")
	assert.Contains(t, api, "createdAtStart?: string;")
	assert.Contains(t, api, "deptId?: string | null;")
	assert.Contains(t, index, `<el-form-item label="联系人姓名">`)
	assert.Contains(t, index, `v-model="ranges.createdAt"`)
	assert.Contains(t, index, "createdAtStart: ranges.createdAt?.[0],")
	assert.Contains(t, index, "getDeptTree")

	// Form: contact is no longer written; list: remark hidden.
	assert.NotContains(t, src, `"contact":`)
	assert.NotContains(t, form, "form.contact")
	assert.NotContains(t, index, `label="备注"`)
	assert.Contains(t, index, `label="客户名称"`)
}

func TestNoSearchFields(t *testing.T) {
	var opts []FieldOption
	for _, f := range mustDescribe(t, minimalTable()) {
		opts = append(opts, FieldOption{Column: f.Column, Search: SearchNone})
	}
	res, err := Generate(minimalTable(), Spec{Module: "notes", Resource: "note", Name: "笔记", NewModule: true, Fields: opts})
	require.NoError(t, err)
	index := fileByPath(t, res, "/note/index.vue").Content
	assert.NotContains(t, index, "resetFilter")
	assert.NotContains(t, index, "搜索")
	assert.Contains(t, fileByPath(t, res, "/api/notes/note.ts").Content, "export type NoteFilter = Record<never, never>;")
}

func mustDescribe(t *testing.T, tbl Table) []FieldInfo {
	t.Helper()
	infos, err := Describe(tbl)
	require.NoError(t, err)
	return infos
}

func TestDictBinding(t *testing.T) {
	yes := true
	infos := mustDescribe(t, mixedTable())
	allowed := map[string]bool{}
	for _, f := range infos {
		allowed[f.Column] = f.DictAllowed
	}
	assert.True(t, allowed["title"], "text")
	assert.True(t, allowed["order"], "integer")
	assert.True(t, allowed["status"], "options replace 1 启用 / 2 停用")
	assert.True(t, allowed["paid"], "yes/no, outside the form only (FieldProblems)")

	spec := Spec{Module: "crm", Resource: "customer", Name: "客户", NewModule: true, Fields: []FieldOption{
		{Column: "level", Dict: ptr("customer_level")}, // default search like becomes eq
		{Column: "sort", Dict: ptr("x")},               // system column: refused below
		{Column: "contact", Dict: ptr("Bad-Code")},     // invalid code
		{Column: "phone", Dict: ptr("phone_kind"), Search: SearchLike},
		{Column: "name", Dict: ptr(OptionsFromComment)}, // no options in the comment
	}}
	var msgs []string
	for _, p := range spec.FieldProblems(customerTable()) {
		msgs = append(msgs, p.Message)
	}
	assert.Equal(t, []string{
		"字段 sort 不能使用下拉选项（只支持文本、整数或是/否字段）",
		"字段 contact 的字典编码无效",
		"字段 phone 使用下拉选项后只能「精确」搜索",
		"字段 name 的注释中没有选项（格式如 类型：1=通知,2=公告）",
	}, msgs)

	spec.Fields = []FieldOption{{Column: "level", Dict: ptr("customer_level"), List: &yes, Search: SearchEq}}
	res, err := Generate(customerTable(), spec)
	require.NoError(t, err)
	src := fileByPath(t, res, "/customer.go").Content
	assert.Contains(t, src, `checkDict(ctx, m.dict, "customer_level", "level", "客户级别", req.Level)`)
	assert.Contains(t, src, `q = q.Where("t.level = ?", v)`)
	assert.Contains(t, fileByPath(t, res, "/module.go").Content, "Dict DictLookup")
	index := fileByPath(t, res, "/customer/index.vue").Content
	assert.Contains(t, index, `const { items: levelOptions, labelOf: levelLabel, tagTypeOf: levelTagType } = useDict("customer_level");`)
	assert.Contains(t, index, `v-for="o in levelOptions"`)
	assert.Contains(t, index, `{{ levelLabel(row.level) }}`)
	form := fileByPath(t, res, "Dialog.vue").Content
	assert.Contains(t, form, `const { items: levelOptions } = useDict("customer_level");`)
	assert.Contains(t, form, `:value="o.value"`)

	// Default search (like) turns into eq once a dictionary is bound.
	spec.Fields = []FieldOption{{Column: "phone", Dict: ptr("phone_kind")}}
	res, err = Generate(customerTable(), spec)
	require.NoError(t, err)
	assert.Contains(t, fileByPath(t, res, "/customer.go").Content, `q = q.Where("t.phone = ?", v)`)

	// Integer columns convert dictionary values for the form.
	res, err = Generate(mixedTable(), Spec{Module: "shop", Resource: "order", Name: "订单", NewModule: true,
		Fields: []FieldOption{{Column: "order", Dict: ptr("order_level")}}})
	require.NoError(t, err)
	assert.Contains(t, fileByPath(t, res, "Dialog.vue").Content, `:value="Number(o.value)"`)
}

func ptr(s string) *string { return &s }

func TestParseCommentOptions(t *testing.T) {
	for comment, want := range map[string][]EnumOption{
		"类型：1=通知,2=公告":            {{"1", "通知"}, {"2", "公告"}},
		"优先级(low=低 mid=中 high=高)": {{"low", "低"}, {"mid", "中"}, {"high", "高"}},
		"来源（web=网页；app=应用）":       {{"web", "网页"}, {"app", "应用"}},
		"状态: 1=待处理、2=已完成":         {{"1", "待处理"}, {"2", "已完成"}},
		"类型：1=通知":                 nil, // one option is not a list
		"类型":                      nil,
		"备注，可以写 a=b 这样的内容":        nil, // no colon or bracket
		"类型：1=通知,随便写":             nil, // a malformed item disables it
		"类型：1=通知,1=公告":            nil, // duplicate values
		"1正常 2冻结 3注销":             {{"1", "正常"}, {"2", "冻结"}, {"3", "注销"}},
		"状态：1正常，2冻结，3注销":          {{"1", "正常"}, {"2", "冻结"}, {"3", "注销"}},
		"状态 0 否 1 是":              {{"0", "否"}, {"1", "是"}},
		"状态(1启用 2停用)":             {{"1", "启用"}, {"2", "停用"}},
		"金额(分)":                   nil,
		"创建时间(UTC+8)":             nil,
		"最多重试 3 次 到 5 次":          nil, // words between the items
		"1正常":                     nil,
		"1正常 1冻结":                 nil,
		"版本 2026 起":               nil,
	} {
		assert.Equal(t, want, parseCommentOptions(comment), comment)
	}
}

func TestCommentOptions(t *testing.T) {
	infos := mustDescribe(t, ticketTable())
	byColumn := map[string]FieldInfo{}
	for _, f := range infos {
		byColumn[f.Column] = f
	}
	kind := byColumn["kind"]
	assert.Equal(t, OptionsFromComment, kind.Dict, "comment options are used by default")
	assert.Equal(t, "类型", kind.Label, "the label stops before the options")
	assert.Len(t, kind.CommentOptions, 2)
	assert.Equal(t, SearchEq, kind.Search, "columns with options are searched by default")
	assert.True(t, byColumn["source"].List, "nullable text with options is listed")
	assert.Equal(t, SearchEq, byColumn["source"].Search, "a default text search becomes a select")
	assert.Empty(t, byColumn["title"].Dict)
	assert.NotNil(t, byColumn["title"].CommentOptions, "an empty list, not null, for the page")

	spec := Spec{Module: "desk", Resource: "ticket", Name: "工单", NewModule: true,
		Fields: []FieldOption{{Column: "source", Search: SearchEq}}}
	res, err := Generate(ticketTable(), spec)
	require.NoError(t, err)
	src := fileByPath(t, res, "/ticket.go").Content
	assert.Contains(t, src, `binding:"omitempty,oneof=1 2"`)
	assert.Contains(t, src, `binding:"omitempty,max=16,oneof=web app"`)
	assert.NotContains(t, src, "checkDict(ctx", "comment options are checked by the binding")
	index := fileByPath(t, res, "/ticket/index.vue").Content
	assert.Contains(t, index, `import { staticDict } from "@/composables/useDict";`)
	assert.Contains(t, index, `= staticDict([{ value: "1", label: "通知" }, { value: "2", label: "公告" }]);`)
	assert.Contains(t, index, `v-for="o in sourceOptions"`)
	form := fileByPath(t, res, "Dialog.vue").Content
	assert.Contains(t, form, `const { items: kindOptions } = staticDict(`)
	assert.Contains(t, form, `:value="Number(o.value)"`)

	// Turned off: a plain input again, no option rule.
	spec.Fields = []FieldOption{{Column: "kind", Dict: ptr("")}, {Column: "source", Dict: ptr("")}}
	res, err = Generate(ticketTable(), spec)
	require.NoError(t, err)
	assert.NotContains(t, fileByPath(t, res, "/ticket.go").Content, "oneof=")
	assert.NotContains(t, fileByPath(t, res, "/ticket/index.vue").Content, "staticDict")

	// Mixed with a dictionary: both helpers are imported.
	spec.Fields = []FieldOption{{Column: "title", Dict: ptr("ticket_title"), Search: SearchEq}, {Column: "source", Search: SearchEq}}
	res, err = Generate(ticketTable(), spec)
	require.NoError(t, err)
	assert.Contains(t, fileByPath(t, res, "/ticket/index.vue").Content, `import { staticDict, useDict } from "@/composables/useDict";`)
}

func TestMissingStandardColumnsNoted(t *testing.T) {
	res, err := Generate(minimalTable(), Spec{Module: "notes", Resource: "note", Name: "笔记", NewModule: true})
	require.NoError(t, err)
	notes := strings.Join(res.Notes, "\n")
	assert.Contains(t, notes, "表缺少标准字段")
	assert.Contains(t, notes, "version（")
	res, err = Generate(customerTable(), customerSpec())
	require.NoError(t, err)
	assert.NotContains(t, strings.Join(res.Notes, "\n"), "表缺少标准字段", "the customer fixture is complete")
}

// A read-only player list: dictionaries on the status and a yes/no flag,
// deleted_at as a plain time (soft delete off), export like the list.
func TestReadOnlyDictsAndPlainDeletedAt(t *testing.T) {
	spec := accountSpec("game")
	require.Empty(t, spec.FieldProblems(accountTable()))
	res, err := Generate(accountTable(), spec)
	require.NoError(t, err)
	src := fileByPath(t, res, "/account.go").Content
	index := fileByPath(t, res, "/index.vue").Content

	// status: the dictionary instead of 1 启用 / 2 停用.
	assert.NotContains(t, src, `map[int]string{1: "启用", 2: "停用"}`)
	assert.Regexp(t, `Status\s+\*int64`, src, "filter by any value")
	assert.Contains(t, src, `"ID", "昵称", "状态", "测试号"`, "the name of the status column is not its options")
	assert.Contains(t, index, `useDict("user_status")`)
	// is_test: yes/no shown with the dictionary values 1 and 0.
	assert.Contains(t, index, `isTestLabel(dictBool(row.isTest))`)
	assert.Contains(t, index, "const dictBool = ")
	assert.Regexp(t, `IsTest\s+\*bool`, src)
	// deleted_at: listed, searched, never hides rows.
	assert.NotContains(t, src, "deleted_at IS NULL")
	assert.Regexp(t, `DeletedAt\s+\*time.Time`, src)
	assert.Contains(t, src, `"t.deleted_at >= ?"`)
	// Export: the list columns with labels; the note is not listed, so not exported.
	assert.Contains(t, src, `statusLabels, err := dictLabels(ctx, m.dict, "user_status")`)
	assert.Contains(t, src, `accountLabel(isTestLabels, optionKey(r.IsTest))`)
	assert.Contains(t, src, `export.TimeHeader("注销时间")`)
	assert.Contains(t, src, "export.Check(ctx, m.db, q)")
	assert.NotContains(t, src, "csvValue(r.InternalNote)")
	assert.NotContains(t, src, "ExportRows")

	// The export switch overrides the default.
	on, off := true, false
	spec.Fields = append(spec.Fields, FieldOption{Column: "nickname", Export: &off})
	spec.Fields[2].Export = &on
	res, err = Generate(accountTable(), spec)
	require.NoError(t, err)
	src = fileByPath(t, res, "/account.go").Content
	assert.Contains(t, src, "csvValue(r.InternalNote)")
	assert.NotContains(t, src, "csvValue(r.Nickname)")
}

func TestStatusWithOwnValues(t *testing.T) {
	no := false
	res, err := Generate(accountTable(), Spec{Module: "game", Resource: "account", Name: "账号", NewModule: true, SoftDelete: &no})
	require.NoError(t, err)
	src := fileByPath(t, res, "/account.go").Content
	assert.NotContains(t, src, `oneof=1 2"`, "1正常 2冻结 3注销 is not 启用/停用")
	assert.Contains(t, src, `oneof=1 2 3`)
	assert.Contains(t, src, `var accountStatusLabels = map[string]string{"1": "正常", "2": "冻结", "3": "注销"}`)
	assert.Contains(t, src, "DELETE FROM lg_account WHERE", "soft delete off: rows are removed")
	assert.NotContains(t, src, "deleted_at IS NULL")

	// A status comment with numbers that are not options: a plain integer.
	tbl := accountTable()
	tbl.Columns[2].Comment = "状态 见文档 v2"
	res, err = Generate(tbl, Spec{Module: "game", Resource: "account", Name: "账号", NewModule: true})
	require.NoError(t, err)
	src = fileByPath(t, res, "/account.go").Content
	assert.NotContains(t, src, "启用")
	assert.Contains(t, src, "deleted_at IS NULL", "soft delete stays on by default")
}

func TestYesNoDictOnlyOutsideTheForm(t *testing.T) {
	spec := Spec{Module: "game", Resource: "account", Name: "账号", Fields: []FieldOption{{Column: "is_test", Dict: ptr("yes_no")}}}
	var msgs []string
	for _, p := range spec.FieldProblems(accountTable()) {
		msgs = append(msgs, p.Message)
	}
	assert.Equal(t, []string{"字段 is_test 是是/否字段，只有不在表单中（只读模块或关闭「表单」）时才能使用下拉选项"}, msgs)
	off := false
	spec.Fields[0].Form = &off
	assert.Empty(t, spec.FieldProblems(accountTable()))
	spec.Fields[0].Form = nil
	spec.ReadOnly = true
	assert.Empty(t, spec.FieldProblems(accountTable()))
}
