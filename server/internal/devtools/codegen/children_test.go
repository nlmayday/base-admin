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

func TestChildKeysFromTable(t *testing.T) {
	res, err := Generate(bannerTable(), Spec{Module: "game", Resource: "banner", Name: "公告横幅", NewModule: true,
		Children: []ChildSpec{bannerChild()}})
	require.NoError(t, err)
	files := map[string]string{}
	for _, f := range res.Files {
		files[f.Path[strings.LastIndex(f.Path, "/")+1:]] = f.Content
	}
	src := files["banner.go"]
	for _, want := range []string{
		`g.GET("/options/i18nKeys", access.Perm("game:banner:view"), handle(m.bannerI18nKeyOptions))`,
		`q := db.WithContext(ctx).Table("lg_language").Select(`,
		`database.CastText(db, "code")`,
		`required AS "required"`,
		`q = q.Where("enabled = 1")`,
		`Order("sort, code")`,
		`return fieldError(field, "请选择有效的语言")`,
		`return fieldError("i18n", k.Label+"的多语言必须填写")`,
		"INSERT INTO lg_banner_i18n (banner_id, lang, title, image_url) VALUES (?, ?, ?, ?)",
	} {
		assert.Contains(t, src, want)
	}
	assert.NotContains(t, src, `checkDict(ctx, m.dict, ""`)
	assert.Contains(t, files["banner.ts"], `export const listBannerI18nKeys = () =>`)
	form := files["BannerFormDialog.vue"]
	assert.Contains(t, form, "keys = (await listBannerI18nKeys()) ?? [];")
	assert.NotContains(t, form, "getDictOptions")
	assert.Contains(t, form, "fieldErrors.i18n")
	assert.Contains(t, strings.Join(res.Notes, "\n"), "表 lg_language")

	for name, mutate := range map[string]func(*ChildSpec){
		"both sources": func(c *ChildSpec) { c.KeyDict = "lang" },
		"no source":    func(c *ChildSpec) { c.KeyTable = "" },
		"bad column":   func(c *ChildSpec) { c.KeyValueColumn = "Code;" },
		"statement":    func(c *ChildSpec) { c.KeyFilter = "1 = 1; DROP TABLE x" },
		"placeholder":  func(c *ChildSpec) { c.KeyFilter = "enabled = ?" },
		"comment":      func(c *ChildSpec) { c.KeyFilter = "enabled = 1 -- x" },
	} {
		c := bannerChild()
		mutate(&c)
		tb := bannerTable()
		tb.Children[0].Spec = c
		_, err := Generate(tb, Spec{Module: "game", Resource: "banner", Name: "公告横幅", Children: []ChildSpec{c}})
		assert.Error(t, err, name)
	}
}
