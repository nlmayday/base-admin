package codegen

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChildTables(t *testing.T) {
	res, err := Generate(itemTable(), itemSpec("game"))
	require.NoError(t, err)
	files := map[string]string{}
	for _, f := range res.Files {
		files[f.Path[strings.LastIndex(f.Path, "/")+1:]] = f.Content
	}
	src := files["item.go"]
	for _, want := range []string{
		"I18n []itemI18nView `json:\"i18n,omitempty\" doc:\"多语言\"`",
		"func loadItemI18n(ctx context.Context, db *gorm.DB, key int64) ([]itemI18nView, error)",
		`Table("lg_item_i18n AS t").Select(`,
		`Where("t.item_id = ?", key).Order("t.lang")`,
		"func (m *Module) saveItemI18n(ctx context.Context, tx *gorm.DB, key int64, rows []itemI18nRequest) error",
		`checkDict(ctx, m.dict, "lang", field, "语言", r.Lang)`,
		"DELETE FROM lg_item_i18n WHERE item_id = ?",
		"INSERT INTO lg_item_i18n (item_id, lang, name, description) VALUES (?, ?, ?, ?)",
		"return r.Name == \"\" && r.Description == nil",
		"if req.I18n != nil {",
	} {
		assert.Contains(t, src, want)
	}
	assert.Contains(t, files["item.ts"], "export type ItemI18n = {")
	assert.Contains(t, files["item.ts"], "i18n?: ItemI18n[];")
	form := files["ItemFormDialog.vue"]
	assert.Contains(t, form, `getDictOptions("lang")`)
	assert.Contains(t, form, `<el-tabs v-if="i18nRows.length" type="border-card">`)
	assert.Contains(t, form, "description: r.description || null")
	assert.Contains(t, strings.Join(res.Notes, "\n"), "子表 lg_item_i18n")

	bad := itemTable()
	bad.Children[0].Spec.ForeignKey = "nope"
	_, err = Generate(bad, Spec{Module: "game", Resource: "item", Name: "道具", Children: []ChildSpec{bad.Children[0].Spec}})
	assert.ErrorContains(t, err, "没有列 nope")

	bad = itemTable()
	bad.Children[0].Table.Columns[1].Primary = false
	_, err = Generate(bad, itemSpec("game"))
	assert.ErrorContains(t, err, "主键须为 (item_id, lang)")

	_, err = Generate(itemTable(), Spec{Module: "game", Resource: "item", Name: "道具", ReadOnly: true, Children: itemSpec("game").Children})
	assert.ErrorContains(t, err, "只读模块不能编辑子表")
}
