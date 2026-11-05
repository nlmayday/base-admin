package codegen

import (
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
		{Column: "title", Search: SearchLike},
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
