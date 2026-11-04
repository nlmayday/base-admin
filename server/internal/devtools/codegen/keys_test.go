package codegen

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyModes(t *testing.T) {
	gen := func(tb Table, spec Spec) map[string]string {
		t.Helper()
		res, err := Generate(tb, spec)
		require.NoError(t, err)
		out := map[string]string{}
		for _, f := range res.Files {
			out[f.Path[strings.LastIndex(f.Path, "/")+1:]] = f.Content
		}
		out["notes"] = strings.Join(res.Notes, "\n")
		return out
	}

	player := gen(playerTable(), Spec{Module: "game", Resource: "player", Name: "玩家", NewModule: true})
	assert.Contains(t, player["player.go"], "key := m.NextID()")
	assert.Contains(t, player["player.go"], "INSERT INTO lg_player (id, nickname, level, price_currency, user_id)")
	assert.NotContains(t, player["player.go"], "database.InsertID")
	assert.Contains(t, player["module.go"], "NewID func() int64")
	// NOT NULL with a default: may be omitted, so the length rule needs omitempty.
	assert.Contains(t, player["player.go"], "PriceCurrency *string `json:\"priceCurrency\" binding:\"omitempty,max=3\"")
	assert.Contains(t, player["player.go"], "COALESCE(?, 'USD')")
	assert.Contains(t, player["notes"], "由程序生成")
	assert.Contains(t, player["notes"], "表缺少标准字段", "missing standard columns only add a note")

	cfg := gen(configTable(), Spec{Module: "game", Resource: "game-config", Name: "游戏配置"})
	src := cfg["game_config.go"]
	assert.Contains(t, src, "func parseGameConfigKey(c *gin.Context) (string, error)")
	assert.Contains(t, src, `Where("t.cfg_key = ?", key)`)
	assert.Contains(t, src, `Where("cfg_key = ? AND version = ?", key, *req.Version)`)
	assert.Contains(t, src, "`json:\"cfgKey\" binding:\"max=64\" doc:\"配置键\"`", "not required on update")
	assert.Contains(t, src, `return nil, fieldError("cfgKey", "不能为空")`)
	assert.Contains(t, src, `return fieldError("cfgKey", "配置键已存在")`)
	assert.NotContains(t, src, `"cfg_key": req.CfgKey`, "the key is not updated")
	assert.Contains(t, cfg["game-config.ts"], "export const gameConfigKey = (row: GameConfig): string => String(row.cfgKey);")
	assert.Contains(t, cfg["GameConfigFormDialog.vue"], `:disabled="isEdit"`)
	assert.Contains(t, cfg["index.vue"], `row-key="cfgKey"`)

	level := gen(levelTable(), Spec{Module: "game", Resource: "level", Name: "等级"})
	assert.Contains(t, level["level.go"], "strconv.ParseInt(c.Param(\"id\"), 10, 32)")
	assert.Contains(t, level["level.go"], "key := req.Level")

	ro := gen(orderLogTable(), Spec{Module: "ledger", Resource: "order-log", Name: "订单流水", NewModule: true, ReadOnly: true})
	src = ro["order_log.go"]
	for _, absent := range []string{"g.POST(", "g.PUT(", "g.DELETE(", "orderLogRequest", ":create", ":update", ":delete"} {
		assert.NotContains(t, src, absent)
	}
	assert.Contains(t, src, `g.GET("/export"`)
	assert.Contains(t, src, `:export", Name: "导出订单流水"`)
	_, hasForm := ro["OrderLogFormDialog.vue"]
	assert.False(t, hasForm, "no form")
	assert.NotContains(t, ro["index.vue"], "新增订单流水")
	assert.Contains(t, ro["index.vue"], "导出 CSV")
	assert.NotContains(t, ro["order-log.ts"], "createOrderLog")
	assert.NotContains(t, ro["order-log.ts"], "LookupOption", "the relation is not searched: no options route")
	vip := gen(vipLevelTable(), Spec{Module: "game", Resource: "vip-level", Name: "VIP 等级"})
	assert.Contains(t, vip["vip-level.ts"], "export type VipLevelFilter = Record<never, never>;")
	assert.Contains(t, ro["notes"], "只读模块")
	assert.NotContains(t, src, `map[int]string{1: "启用", 2: "停用"}`, "a status with comment options keeps its own meaning")
	assert.Contains(t, src, "var orderLogStatusLabels = map[string]string{")
	assert.Contains(t, src, `"2": "已支付"`)
	assert.Contains(t, src, "orderLogLabel(orderLogStatusLabels, optionKey(r.Status))")
	assert.Contains(t, ro["index.vue"], `staticDict([{ value: "1", label: "待支付" }, { value: "2", label: "已支付" }])`)

	// Requested modes are checked against the column.
	_, err := Generate(customerTable(), Spec{Module: "crm", Resource: "customer", Name: "客户", KeyMode: KeyGenerated})
	assert.ErrorContains(t, err, "由数据库自增")
	_, err = Generate(configTable(), Spec{Module: "game", Resource: "cfg", Name: "配置", KeyMode: KeyGenerated})
	assert.ErrorContains(t, err, "generated 只用于 bigint")
	res, err := Generate(playerTable(), Spec{Module: "game", Resource: "player", Name: "玩家", KeyMode: KeyInput})
	require.NoError(t, err, "a bigint key may be typed in")
	assert.Contains(t, fileByPath(t, res, "/player.go").Content, "key := int64(req.ID)")
}
