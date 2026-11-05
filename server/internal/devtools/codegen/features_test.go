package codegen

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gameTable refers to a table keyed by a text code (lg_game_family.family).
func gameTable() Table {
	return Table{Name: "lg_game", Comment: "游戏", Columns: []Column{
		{Name: "id", UDT: "int8", Primary: true},
		{Name: "game_code", UDT: "varchar", MaxLength: 32, Unique: true, Comment: "游戏编码"},
		{Name: "game_type", UDT: "varchar", MaxLength: 32, Comment: "玩法家族",
			Ref: &Ref{Table: "lg_game_family", Key: "family", Labels: []string{"name"}}},
		{Name: "title", UDT: "varchar", MaxLength: 64, Comment: "名称"},
		{Name: "start_at", UDT: "timestamptz", Nullable: true, Comment: "上线时间"},
	}}
}

func gameSpec() Spec {
	return Spec{Module: "game", Resource: "game", Name: "游戏", NewModule: true, NoDelete: true,
		Fields: []FieldOption{
			{Column: "game_code", CreateOnly: true, Hint: "建后不能修改"},
			{Column: "game_type", CreateOnly: true, Search: SearchEq},
			{Column: "start_at", Hint: "上线后不能再改"},
		}}
}

// ledgerTable is a large read-only table whose player is shown by an
// integer column (short_id) of a soft-delete table.
func ledgerTable() Table {
	return Table{Name: "lg_wallet_ledger", Comment: "钱包流水", Columns: []Column{
		{Name: "id", UDT: "int8", Primary: true, Comment: "流水号"},
		{Name: "user_id", UDT: "int8", Comment: "玩家",
			Ref: &Ref{Table: "lg_user", Labels: []string{"nickname"}, NumberLabels: []string{"short_id"}, SoftDelete: true}},
		{Name: "delta", UDT: "numeric", Comment: "变动"},
		{Name: "created_at", UDT: "timestamptz", Default: "now()"},
	}}
}

func ledgerSpec() Spec {
	return Spec{Module: "ledger", Resource: "wallet-ledger", Name: "钱包流水", NewModule: true, ReadOnly: true,
		OrderBy: "created_at DESC, id DESC",
		Fields: []FieldOption{
			{Column: "id", List: boolPtr(true), Search: SearchEq},
			{Column: "user_id", Relation: ptr("short_id"), Search: SearchEq},
		}}
}

func files(t *testing.T, res Result) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, f := range res.Files {
		out[f.Path[strings.LastIndex(f.Path, "/")+1:]] = f.Content
	}
	return out
}

func TestOrderByAndCountWithoutJoins(t *testing.T) {
	res, err := Generate(ledgerTable(), ledgerSpec())
	require.NoError(t, err)
	src := files(t, res)["wallet_ledger.go"]
	assert.Contains(t, src, `Order("t.created_at DESC, t.id DESC")`)
	assert.Contains(t, src, "func selectWalletLedgers(q *gorm.DB) *gorm.DB {\n\tq = q.Joins(`LEFT JOIN lg_user r1 ON r1.id = t.user_id`)")
	scoped := src[strings.Index(src, "func (m *Module) scopedWalletLedgers"):strings.Index(src, "func selectWalletLedgers")]
	assert.NotContains(t, scoped, "Joins", "the count runs on the table alone")
	assert.Contains(t, src, "q.Session(&gorm.Session{}).Count(&total)")

	for _, bad := range []string{"created_at DOWN", "created_at; DROP", "id, id", "a,b,c,d,e,f", "`x`"} {
		s := ledgerSpec()
		s.OrderBy = bad
		assert.NotEmpty(t, s.Problems(), bad)
	}
	s := ledgerSpec()
	s.OrderBy = "nope DESC"
	assert.Contains(t, s.FieldProblems(ledgerTable())[0].Message, "orderBy 的列 nope 不存在")
}

func TestKeyInListAndNumberLabel(t *testing.T) {
	res, err := Generate(ledgerTable(), ledgerSpec())
	require.NoError(t, err)
	f := files(t, res)
	src := f["wallet_ledger.go"]
	assert.Contains(t, src, `lookup.Target{Table: "lg_user", Label: "short_id", NumberLabel: true, SoftDelete: true}`)
	assert.Contains(t, src, `lookup.Query{Keyword: c.Query("keyword"), Deleted: c.Query("deleted") == "1"}`)
	assert.Contains(t, src, `q = q.Where("t.id = ?", f.ID)`, "the key is searched")
	index := f["index.vue"]
	assert.Contains(t, index, `label="流水号"`, "the key is listed")
	assert.Contains(t, index, `:fetch="(kw: string) => listWalletLedgerUserIdOptions(kw, true)"`, "the search bar finds deleted rows")
	assert.Contains(t, index, `placeholder="输入编号"`)

	// Integer labels are offered but never the default.
	infos, err := Describe(ledgerTable())
	require.NoError(t, err)
	for _, i := range infos {
		if i.Column == "user_id" {
			assert.Equal(t, "nickname", i.Relation)
			assert.Equal(t, []string{"nickname", "short_id"}, i.RelationLabels)
		}
	}
	onlyNumbers := ledgerTable()
	onlyNumbers.Columns[1].Ref = &Ref{Table: "lg_user", NumberLabels: []string{"short_id"}}
	res, err = Generate(onlyNumbers, Spec{Module: "ledger", Resource: "wallet-ledger", Name: "钱包流水", ReadOnly: true})
	require.NoError(t, err)
	assert.NotContains(t, files(t, res)["wallet_ledger.go"], "lookup.Target", "a plain id until a label is chosen")
}

func TestStringKeyRelationCreateOnlyHintNoDelete(t *testing.T) {
	res, err := Generate(gameTable(), gameSpec())
	require.NoError(t, err)
	f := files(t, res)
	src := f["game.go"]
	assert.Contains(t, src, `lookup.Target{Table: "lg_game_family", Key: "family", Label: "name", SoftDelete: false}`)
	assert.Contains(t, src, "q = q.Joins(`LEFT JOIN lg_game_family r1 ON r1.family = t.game_type`)")
	assert.Contains(t, src, `checkGameRelation(ctx, m.db, gameGameTypeLookup, "gameType", "玩法家族", req.GameType)`)
	assert.Contains(t, src, "target lookup.Target, field, label string, id any) error")
	update := src[strings.Index(src, "func (m *Module) updateGame"):]
	update = update[:strings.Index(update, "\n}\n")]
	assert.NotContains(t, update, "checkGameRelation", "a create-only relation is not checked on update")
	assert.NotContains(t, update, `"game_code":`, "create-only columns are not updated")
	assert.NotContains(t, update, `"game_type":`)
	assert.Contains(t, update, `"title":`)
	assert.Contains(t, update, "old, err := m.loadGame(ctx, tx, key)")
	assert.Contains(t, update, "m.gameBeforeUpdateHook(ctx, tx, old, &req)")

	// No delete: no route, permission, handler, button or API function.
	for _, absent := range []string{"g.DELETE(", "game:game:delete", "func (m *Module) deleteGame", "gameAfterDelete", "BeforeDelete"} {
		assert.NotContains(t, src, absent)
	}
	assert.Contains(t, src, "g.POST(")
	for _, p := range res.Permissions {
		assert.NotEqual(t, "game:game:delete", p.Code)
	}
	assert.NotContains(t, f["game.ts"], "deleteGame")
	assert.NotContains(t, f["index.vue"], "删除")
	assert.NotContains(t, f["index.vue"], "confirmAction")
	assert.NotContains(t, f["game_ext.go"], "gameAfterDelete")
	assert.Contains(t, f["game_ext.go"], "func (m *Module) gameBeforeUpdate(ctx context.Context, tx *gorm.DB, old gameView, req *gameRequest) error")
	assert.Contains(t, f["game_integration_test.go"], `"deleting is turned off: no route"`)
	assert.Contains(t, f["game_integration_test.go"], `payload["gameType"] = gameRelationSample(t, h, "gameType")`)
	assert.Contains(t, strings.Join(res.Notes, "\n"), "已关闭删除")

	form := f["GameFormDialog.vue"]
	assert.Contains(t, form, `<el-input v-model="form.gameCode" maxlength="32" :disabled="isEdit" />`)
	assert.Contains(t, form, `v-model="form.gameType" :disabled="isEdit"`)
	assert.Contains(t, form, "建后不能修改")
	assert.Contains(t, form, "上线后不能再改")
	index := f["index.vue"]
	assert.Contains(t, index, `:fetch="(kw: string) => listGameGameTypeOptions(kw, true)"`)
	assert.Contains(t, f["game.ts"], "gameType?: string | null;", "the select clears to null")
}

func TestNoCreate(t *testing.T) {
	spec := Spec{Module: "game", Resource: "level", Name: "等级", NewModule: true, NoCreate: true, NoDelete: true}
	res, err := Generate(levelTable(), spec)
	require.NoError(t, err)
	f := files(t, res)
	for _, absent := range []string{"g.POST(", "g.DELETE(", "func (m *Module) createLevel", ":create"} {
		assert.NotContains(t, f["level.go"], absent)
	}
	assert.Contains(t, f["level.go"], "g.PUT(")
	assert.Len(t, res.Permissions, 3, "view, update, export")
	assert.NotContains(t, f["level.ts"], "createLevel")
	assert.NotContains(t, f["LevelFormDialog.vue"], "createLevel")
	assert.NotContains(t, f["index.vue"], "openCreate")
	assert.Contains(t, f["level_integration_test.go"], "func TestLevelNoCreate(t *testing.T)")
	assert.NotContains(t, f["level_integration_test.go"], "TestLevelValidation")

	spec.ReadOnly = true
	assert.NotEmpty(t, spec.Problems(), "read-only has neither anyway")
	spec = gameSpec()
	spec.NoCreate = true
	assert.NotEmpty(t, spec.FieldProblems(gameTable()), "create-only needs a create form")
}

func TestFieldOptionProblems(t *testing.T) {
	for _, o := range []FieldOption{
		{Column: "id", CreateOnly: true},
		{Column: "title", CreateOnly: true, Form: boolPtr(false)},
		{Column: "title", Hint: strings.Repeat("长", 201)},
		{Column: "title", Hint: "a <b>"},
	} {
		s := gameSpec()
		s.Fields = []FieldOption{o}
		assert.Len(t, s.FieldProblems(gameTable()), 1, "%+v", o)
	}
}

func TestChildOptionsListKeysAndMerge(t *testing.T) {
	child := bannerChild()
	child.ListKeys = true
	child.Fields = []FieldOption{{Column: "image_url", Label: "多语言图片", Upload: ptr(UploadImage), Hint: "不填时用主图"}}
	table := bannerTable()
	table.Children[0].Spec = child
	res, err := Generate(table, Spec{Module: "game", Resource: "banner", Name: "横幅", NewModule: true, Children: []ChildSpec{child}})
	require.NoError(t, err)
	f := files(t, res)
	src := f["banner.go"]
	for _, want := range []string{
		"I18nKeys []string `json:\"i18nKeys,omitempty\"",
		"if err := listBannerI18nKeys(c.Request.Context(), m.db, views); err != nil {",
		`Select("banner_id AS parent, lang AS value").`,
		"DELETE FROM lg_banner_i18n WHERE banner_id = ? AND lang IN ?",
		"attachment.URLProblem(*r.ImageURL)",
		`fieldError(fmt.Sprintf("i18n[%d].imageUrl", i), msg)`,
		`Pluck("lang", &stored)`,
		"ImageURL *string `json:\"imageUrl\" doc:\"多语言图片\"`",
	} {
		assert.Contains(t, src, want)
	}
	form := f["BannerFormDialog.vue"]
	assert.Contains(t, form, `<ReImageUpload`)
	assert.Contains(t, form, `@update:model-value="(v: string) => (row.imageUrl = v || null)"`)
	assert.Contains(t, form, "不填时用主图")
	assert.Contains(t, form, `label="多语言图片"`)
	index := f["index.vue"]
	assert.Contains(t, index, `label="已填多语言"`)
	assert.Contains(t, index, "listBannerI18nKeys()")
	assert.Contains(t, f["banner.ts"], "i18nKeys?: string[];")

	// Dictionary keys: the labels come from the dictionary.
	item := itemTable()
	item.Children[0].Spec.ListKeys = true
	spec := itemSpec("game")
	spec.Children = []ChildSpec{item.Children[0].Spec}
	res, err = Generate(item, spec)
	require.NoError(t, err)
	assert.Contains(t, files(t, res)["index.vue"], `const { labelOf: i18nKeyLabel } = useDict("lang");`)
	assert.Contains(t, files(t, res)["index.vue"], `import { useDict } from "@/composables/useDict";`)

	for _, bad := range []FieldOption{
		{Column: "title", Search: SearchEq},
		{Column: "title", Form: boolPtr(false)},
		{Column: "title", Upload: ptr(UploadFiles)},
		{Column: "lang", Label: "x"},
		{Column: "nope"},
	} {
		c := bannerChild()
		c.Fields = []FieldOption{bad}
		tb := bannerTable()
		tb.Children[0].Spec = c
		_, err := Generate(tb, Spec{Module: "game", Resource: "banner", Name: "横幅", Children: []ChildSpec{c}})
		assert.Error(t, err, "%+v", bad)
	}
}

func TestViewConversionAndSharedHelpers(t *testing.T) {
	res, err := Generate(levelTable(), Spec{Module: "game", Resource: "level", Name: "等级", NewModule: true})
	require.NoError(t, err)
	f := files(t, res)
	assert.Contains(t, f["level.go"], "return levelView(r)", "staticcheck S1016")
	assert.Contains(t, f["common.go"], "var _ = []any{")
	assert.Contains(t, f["module.go"], "func (m *Module) Roles() []module.RoleDef { return nil }")

	res, err = Generate(customerTable(), customerSpec())
	require.NoError(t, err)
	assert.NotContains(t, files(t, res)["customer.go"], "return customerView(r)", "ids.ID fields are converted one by one")
}

func boolPtr(b bool) *bool { return &b }

// bannerWithOptions is bannerTable with child field options and list keys.
func bannerWithOptions() Table {
	t := bannerTable()
	c := bannerChild()
	c.ListKeys = true
	c.Fields = []FieldOption{{Column: "image_url", Label: "图片", Upload: ptr(UploadImage), Hint: "不填用主图"}}
	t.Children[0].Spec = c
	return t
}

func (c ChildSpec) asList() []ChildSpec { return []ChildSpec{c} }

func TestUnusedLookupAndLabelDocs(t *testing.T) {
	// Read-only, the relation not searched: no options route, no lookup.
	spec := ledgerSpec()
	spec.Fields = []FieldOption{{Column: "user_id", Relation: ptr("short_id")}}
	res, err := Generate(ledgerTable(), spec)
	require.NoError(t, err)
	src := files(t, res)["wallet_ledger.go"]
	assert.NotContains(t, src, "walletLedgerUserIDLookup")
	assert.NotContains(t, src, "core/lookup")
	assert.Contains(t, src, "LEFT JOIN lg_user r1", "the list still shows the label")
	assert.Contains(t, src, "`json:\"userName\" doc:\"玩家（lg_user.short_id）\"`")

	// The text key itself shown.
	game := gameTable()
	game.Columns[2].Ref = &Ref{Table: "lg_game_family", Key: "family", Labels: []string{"endpoint", "family"}}
	gs := gameSpec()
	gs.Fields = append(gs.Fields[:1], FieldOption{Column: "game_type", Relation: ptr("family")})
	res, err = Generate(game, gs)
	require.NoError(t, err)
	f := files(t, res)
	assert.Contains(t, f["game.go"], `lookup.Target{Table: "lg_game_family", Key: "family", Label: "family", SoftDelete: false}`)
	assert.Contains(t, f["game.go"], "doc:\"玩法家族（lg_game_family.family）\"")
	assert.Contains(t, f["game.ts"], "/** 玩法家族（lg_game_family.family） */")

	res, err = Generate(dealTable(), Spec{Module: "crm", Resource: "deal", Name: "商机"})
	require.NoError(t, err)
	assert.Contains(t, files(t, res)["deal.go"], "doc:\"客户名称\"")
}

func TestListKeysLabel(t *testing.T) {
	tb := bannerWithOptions()
	c := tb.Children[0].Spec
	c.ListKeysLabel = "已填语言"
	tb.Children[0].Spec = c
	res, err := Generate(tb, Spec{Module: "game", Resource: "banner", Name: "横幅", Children: c.asList()})
	require.NoError(t, err)
	assert.Contains(t, files(t, res)["index.vue"], `label="已填语言"`)
	c.ListKeysLabel = strings.Repeat("长", 21)
	tb.Children[0].Spec = c
	_, err = Generate(tb, Spec{Module: "game", Resource: "banner", Name: "横幅", Children: c.asList()})
	assert.ErrorContains(t, err, "listKeysLabel")
}

// taskTable uses every display, form and search option (items 1–17).
func taskTable() Table {
	return Table{Name: "lg_task", Comment: "每日任务", Columns: []Column{
		{Name: "id", UDT: "int8", Primary: true},
		{Name: "title", UDT: "varchar", MaxLength: 64, Comment: "标题"},
		{Name: "fill_type", UDT: "int2", Default: "1", Comment: "填写方式：1=固定,2=随机"},
		{Name: "reward", UDT: "int4", Default: "0", Comment: "奖励"},
		{Name: "amount", UDT: "int8", Nullable: true, Comment: "变动"},
		{Name: "price", UDT: "int4", Default: "0", Comment: "价格"},
		{Name: "rate", UDT: "int4", Default: "0", Comment: "返还比例"},
		{Name: "multiplier", UDT: "int4", Default: "1000", Comment: "倍数"},
		{Name: "cover", UDT: "varchar", MaxLength: 255, Nullable: true, Comment: "封面地址"},
		{Name: "start_at", UDT: "timestamptz", Nullable: true, Comment: "开始时间"},
		{Name: "handled", UDT: "bool", Default: "false", Comment: "已处理"},
		{Name: "user_id", UDT: "int8", Nullable: true, Comment: "玩家",
			Ref: &Ref{Table: "lg_user", Labels: []string{"nickname"}, NumberLabels: []string{"short_id"}, SoftDelete: true}},
		{Name: "created_at", UDT: "timestamptz", Default: "now()"},
	}, Children: []ChildTable{{
		Spec: taskChild(),
		Table: Table{Name: "lg_task_i18n", Columns: []Column{
			{Name: "task_id", UDT: "int8", Primary: true},
			{Name: "lang", UDT: "varchar", MaxLength: 8, Primary: true, Comment: "语言"},
			{Name: "name", UDT: "varchar", MaxLength: 64, Comment: "名称"},
			{Name: "description", UDT: "text", Nullable: true, Comment: "描述"},
		}},
	}}}
}

func taskChild() ChildSpec {
	return ChildSpec{Table: "lg_task_i18n", ForeignKey: "task_id", KeyColumn: "lang", KeyDict: "lang", Label: "多语言",
		Hint: "英文必填", RequiredFields: []string{"name"}, ListKeys: true,
		Columns: []ChildColumn{{Key: "en", Field: "name", Label: "名称(英文)"}},
		Fields:  []FieldOption{{Column: "name", Search: SearchLike, Placeholder: "任务名称"}}}
}

func taskSpec(module string) Spec {
	f := func(v float64) *float64 { return &v }
	sc := func(v string) *Scalar { s := Scalar(v); return &s }
	return Spec{Module: module, Resource: "task", Name: "任务", NewModule: true,
		OrderBy: "handled = false DESC, id DESC", DeleteConfirm: "删除任务「{title}」？大厅将不再显示。",
		Children: []ChildSpec{taskChild()},
		Fields: []FieldOption{
			{Column: "title", Required: boolPtr(false), Placeholder: "可留空"},
			{Column: "reward", Format: FormatCoins, Min: f(0), Max: f(100000), Step: f(10), Default: sc("100"),
				ShowWhen: &ShowWhen{Column: "fill_type", In: []Scalar{"2"}}},
			{Column: "amount", Format: FormatSigned},
			{Column: "price", Format: FormatCents},
			{Column: "rate", Format: FormatPermille},
			{Column: "multiplier", Format: FormatMilli, Suffix: "x"},
			{Column: "cover", Format: FormatImage},
			{Column: "start_at", ShowIf: "feature:vip", Search: SearchRange},
			{Column: "handled", Search: SearchEq, SearchDefault: sc("false")},
			{Column: "user_id", Relation: ptr("short_id"), RelationExtra: "nickname", Search: SearchEq},
		}}
}

func TestDisplayFormAndSearchOptions(t *testing.T) {
	res, err := Generate(taskTable(), taskSpec("game"))
	require.NoError(t, err)
	f := files(t, res)
	src, index, form, api := f["task.go"], f["index.vue"], f["TaskFormDialog.vue"], f["task.ts"]

	// 1 formats (list only; the export keeps the value), 16 time display
	for _, want := range []string{
		"{{ formatCoins(row.reward) }}", `:class="signClass(row.amount)">{{ formatSigned(row.amount) }}`,
		"{{ formatCents(row.price) }}", "{{ formatPermille(row.rate) }}", `{{ formatMilli(row.multiplier, "x") }}`,
		`<ReDateTime v-else :value="row.createdAt" />`,
	} {
		assert.Contains(t, index, want)
	}
	assert.Contains(t, src, "csvValue(r.Reward),")
	// 2, 3, 14 extension: cells, header, expand, row buttons
	for _, want := range []string{
		`import TaskExtension, * as taskExtension from "./components/TaskExtension.vue";`,
		"const ext = extensionOptions<Task>(taskExtension);",
		`v-if="ext.cellColumns.includes('reward')"`, `place="cell"`, `v-if="ext.hasHeader"`,
		`<el-table-column v-if="ext.hasExpand" type="expand">`, `v-if="ext.canEdit(row as Task)"`, `v-if="ext.canDelete(row as Task)"`,
	} {
		assert.Contains(t, index, want)
	}
	for _, want := range []string{"export const cellColumns: string[] = [];", "export function canEdit(row: Task): boolean",
		"export const hasExpand = false;", `place: "toolbar" | "row" | "header" | "expand" | "cell";`} {
		assert.Contains(t, f["TaskExtension.vue"], want)
	}
	// 5 child columns and search
	assert.Contains(t, src, "i18nNameEn, err := childValues(ctx, db, `lg_task_i18n`, `task_id`, `lang`, \"en\", `name`, parents)")
	assert.Contains(t, src, `EXISTS (SELECT 1 FROM lg_task_i18n c WHERE c.task_id = t.id AND LOWER(c.name) LIKE LOWER(?))`)
	assert.Contains(t, index, `label="名称(英文)"`)
	assert.Contains(t, api, "i18nNameEn?: string;")
	// 6, 7 search default and route query
	assert.Contains(t, index, "handled: false")
	assert.Contains(t, index, `kinds: { title: "string", fillType: "string", handled: "boolean", userId: "string", i18nName: "string" },`)
	assert.Contains(t, index, "useRouteFilter({")
	// 8 order expression, 9 UTC days
	assert.Contains(t, src, `Order("t.handled = FALSE DESC, t.id DESC")`)
	assert.Contains(t, src, "time.ParseInLocation(time.DateOnly, f.StartAtStart, time.UTC)")
	assert.Contains(t, index, `label="开始时间(UTC)"`)
	// 10 default, min/max/step, placeholder; 11 showWhen
	assert.Contains(t, form, "reward: 100,")
	assert.Contains(t, form, `:min="0" :max="100000" :step="10"`)
	assert.Contains(t, src, "`json:\"reward\" binding:\"omitempty,min=0,max=100000\"")
	assert.Contains(t, form, `placeholder="可留空"`)
	assert.Contains(t, form, `<el-form-item v-if="([2] as unknown[]).includes(form.fillType)"`)
	// 12 empty text allowed, trimmed
	assert.Regexp(t, "Title +string +`json:\"title\" binding:\"max=64\"", src)
	assert.NotContains(t, form, `title: [`)
	assert.Contains(t, src, "r.Title = strings.TrimSpace(r.Title)")
	assert.Contains(t, src, "if err := req.normalize(); err != nil {")
	// 13 child required fields, hint, tab mark
	assert.Contains(t, src, `return fieldError(fmt.Sprintf("i18n[%d].name", i), "不能为空")`)
	assert.Contains(t, src, "rows[i].Name = strings.TrimSpace(rows[i].Name)")
	assert.Contains(t, form, "英文必填")
	assert.Contains(t, form, "return [row.name].some(blank);")
	assert.Contains(t, form, `placeholder="任务名称"`)
	// 15 delete confirmation
	assert.Contains(t, index, "confirm: `删除任务「${row.title}」？大厅将不再显示。`,")
	// 16 time input in the header's zone
	assert.Contains(t, form, "<ReDateTimePicker")
	// 17 feature switch
	assert.Contains(t, src, `"vip": m.featureOn(ctx, "vip"),`)
	assert.Contains(t, src, "if !m.featureOn(ctx, \"vip\") {\n\t\t\tdelete(sets, \"start_at\")")
	assert.Contains(t, form, `v-if="props.features?.vip === true"`)
	assert.Contains(t, index, `:features="features"`)
	assert.Contains(t, f["common.go"], "func (m *Module) featureOn(ctx context.Context, name string) bool {")
	// 4 relation with a second column
	assert.Contains(t, src, `lookup.Target{Table: "lg_user", Label: "short_id", Extra: "nickname", NumberLabel: true, SoftDelete: true}`)
	assert.Regexp(t, "UserNickname +string +`json:\"userNickname\"", src)
	assert.Contains(t, form, `relationLabel(props.row.userName, props.row.userNickname)`)
}

func TestDisplayOptionProblems(t *testing.T) {
	sc := func(v string) *Scalar { s := Scalar(v); return &s }
	for _, o := range []FieldOption{
		{Column: "title", Format: FormatCoins},
		{Column: "reward", Format: "money"},
		{Column: "reward", Suffix: "x"},
		{Column: "reward", Default: sc("abc")},
		{Column: "reward", Min: boolFloat(5), Max: boolFloat(1)},
		{Column: "reward", ShowWhen: &ShowWhen{Column: "nope", In: []Scalar{"1"}}},
		{Column: "reward", ShowWhen: &ShowWhen{Column: "fill_type", In: []Scalar{"x"}}},
		{Column: "reward", Required: boolPtr(false)},
		{Column: "title", ShowIf: "vip"},
		{Column: "title", Placeholder: "<b>"},
		{Column: "handled", SearchDefault: sc("1")},    // not searched
		{Column: "user_id", RelationExtra: "nickname"}, // the label itself
	} {
		s := taskSpec("game")
		s.Fields = []FieldOption{o}
		assert.NotEmpty(t, s.FieldProblems(taskTable()), "%+v", o)
	}
	s := taskSpec("game")
	s.DeleteConfirm = "删除「{nope}」"
	assert.NotEmpty(t, s.FieldProblems(taskTable()))
	s.DeleteConfirm = "删除 ${x}"
	assert.NotEmpty(t, s.FieldProblems(taskTable()))
	s.DeleteConfirm = "删除「{title}」"
	assert.Empty(t, s.FieldProblems(taskTable()))
	for _, bad := range []string{"status = 0; DROP", "status = x DESC", "status == 0"} {
		s.OrderBy = bad
		assert.NotEmpty(t, s.Problems(), bad)
	}

	var v struct {
		A, B, C Scalar
	}
	require.NoError(t, json.Unmarshal([]byte(`{"A": 1, "B": "x", "C": false}`), &v))
	assert.Equal(t, Scalar("1"), v.A)
	assert.Equal(t, Scalar("false"), v.C)
}

func boolFloat(v float64) *float64 { return &v }
