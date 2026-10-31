//go:build integration

package system

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"base-admin/server/internal/platform/httpx"
)

func (h *harness) createType(code, name string) dictTypeView {
	h.t.Helper()
	var v dictTypeView
	h.ok(h.do(http.MethodPost, "/dict-types", gin.H{"code": code, "name": name}), &v)
	return v
}

func (h *harness) createItem(typeID, label, value string, sort int, extra gin.H) dictItemView {
	h.t.Helper()
	body := gin.H{"label": label, "value": value, "sort": sort}
	for k, v := range extra {
		body[k] = v
	}
	var v dictItemView
	h.ok(h.do(http.MethodPost, "/dict-types/"+typeID+"/items", body), &v)
	return v
}

func TestDictTypeCRUD(t *testing.T) {
	h := newHarness(t)
	created := h.createType("order_channel", "订单渠道")
	assert.Equal(t, "order_channel", created.Code)
	assert.Equal(t, int16(1), created.Status)
	assert.Equal(t, 0, created.Version)

	assert.Equal(t, []string{"code"}, h.fields(h.do(http.MethodPost, "/dict-types", gin.H{"code": "order_channel", "name": "x"})))
	assert.Equal(t, []string{"code"}, h.fields(h.do(http.MethodPost, "/dict-types", gin.H{"code": "Bad-Code", "name": "x"})))
	assert.Equal(t, []string{"name"}, h.fields(h.do(http.MethodPost, "/dict-types", gin.H{"code": "ok_code", "name": "  "})))
	h.createType("gender", "性别")

	var page httpx.Page[dictTypeView]
	h.ok(h.do(http.MethodGet, "/dict-types?keyword=渠道", nil), &page)
	require.Equal(t, int64(1), page.Total)
	assert.Equal(t, created.ID, page.List[0].ID)
	h.ok(h.do(http.MethodGet, "/dict-types?status=2", nil), &page)
	assert.Equal(t, int64(0), page.Total)
	h.fields(h.do(http.MethodGet, "/dict-types?status=9", nil))

	id := created.ID.String()
	var updated dictTypeView
	h.ok(h.do(http.MethodPut, "/dict-types/"+id, gin.H{"code": "ignored", "name": "等级", "status": 2, "version": 0}), &updated)
	assert.Equal(t, "order_channel", updated.Code, "code is immutable")
	assert.Equal(t, "等级", updated.Name)
	assert.Equal(t, int16(2), updated.Status)
	assert.Equal(t, 1, updated.Version)

	h.fail(h.do(http.MethodPut, "/dict-types/"+id, gin.H{"name": "x", "version": 0}), http.StatusConflict, httpx.CodeConflict)
	assert.Equal(t, []string{"version"}, h.fields(h.do(http.MethodPut, "/dict-types/"+id, gin.H{"name": "x"})))
	h.fail(h.do(http.MethodPut, "/dict-types/999999", gin.H{"name": "x", "version": 0}), http.StatusNotFound, httpx.CodeNotFound)

	h.createItem(id, "VIP", "vip", 1, nil)
	h.ok(h.do(http.MethodDelete, "/dict-types/"+id, nil), nil)
	assert.Zero(t, h.count("SELECT count(*) FROM sys_dict_items WHERE type_id = ?", created.ID), "items cascade")
	h.fail(h.do(http.MethodDelete, "/dict-types/"+id, nil), http.StatusNotFound, httpx.CodeNotFound)
	h.fail(h.do(http.MethodGet, "/dict-types/"+id+"/items", nil), http.StatusNotFound, httpx.CodeNotFound)
}

func TestDictItemCRUD(t *testing.T) {
	h := newHarness(t)
	typ := h.createType("order_status", "订单状态").ID.String()
	paid := h.createItem(typ, "已支付", "paid", 2, gin.H{"tagType": "success", "remark": "r"})
	h.createItem(typ, "待支付", "pending", 1, gin.H{"tagType": "warning"})
	assert.Equal(t, "success", paid.TagType)
	assert.Equal(t, typ, paid.TypeID.String())

	assert.Equal(t, []string{"value"}, h.fields(h.do(http.MethodPost, "/dict-types/"+typ+"/items", gin.H{"label": "x", "value": "paid"})))
	assert.Equal(t, []string{"tagType"}, h.fields(h.do(http.MethodPost, "/dict-types/"+typ+"/items", gin.H{"label": "x", "value": "y", "tagType": "pink"})))
	assert.ElementsMatch(t, []string{"label", "value"}, h.fields(h.do(http.MethodPost, "/dict-types/"+typ+"/items", gin.H{})))
	h.fail(h.do(http.MethodPost, "/dict-types/999999/items", gin.H{"label": "x", "value": "y"}), http.StatusNotFound, httpx.CodeNotFound)

	other := h.createType("other_dict", "其他").ID.String()
	h.createItem(other, "已支付", "paid", 0, nil) // same value in another type is fine

	var items []dictItemView
	h.ok(h.do(http.MethodGet, "/dict-types/"+typ+"/items", nil), &items)
	require.Len(t, items, 2)
	assert.Equal(t, []string{"pending", "paid"}, []string{items[0].Value, items[1].Value}, "ordered by sort")

	id := paid.ID.String()
	h.fail(h.do(http.MethodPut, "/dict-items/"+id, gin.H{"label": "x", "value": "pending", "version": 0}),
		http.StatusUnprocessableEntity, httpx.CodeValidation)
	var updated dictItemView
	h.ok(h.do(http.MethodPut, "/dict-items/"+id, gin.H{"label": "已付款", "value": "paid", "sort": 3, "status": 2, "version": 0}), &updated)
	assert.Equal(t, "已付款", updated.Label)
	assert.Equal(t, "", updated.TagType, "omitted tag type resets to plain")
	assert.Equal(t, 1, updated.Version)
	h.fail(h.do(http.MethodPut, "/dict-items/"+id, gin.H{"label": "x", "value": "paid", "version": 0}), http.StatusConflict, httpx.CodeConflict)

	h.ok(h.do(http.MethodGet, "/dict-types/"+typ+"/items?status=2", nil), &items)
	require.Len(t, items, 1)
	h.ok(h.do(http.MethodGet, "/dict-types/"+typ+"/items?keyword=待", nil), &items)
	require.Len(t, items, 1)
	assert.Equal(t, "pending", items[0].Value)

	h.ok(h.do(http.MethodDelete, "/dict-items/"+id, nil), nil)
	h.fail(h.do(http.MethodDelete, "/dict-items/"+id, nil), http.StatusNotFound, httpx.CodeNotFound)
}

func TestDictOptionsAndCacheInvalidation(t *testing.T) {
	h := newHarness(t)
	typ := h.createType("level", "等级")
	tid := typ.ID.String()
	gold := h.createItem(tid, "黄金", "gold", 2, gin.H{"tagType": "warning"})
	h.createItem(tid, "白银", "silver", 1, nil)
	h.createItem(tid, "停用", "off", 0, gin.H{"status": 2})

	h.perms = nil // any authenticated user may read options
	var opts []DictOption
	h.ok(h.do(http.MethodGet, "/dicts/level/items", nil), &opts)
	assert.Equal(t, []DictOption{{Label: "白银", Value: "silver"}, {Label: "黄金", Value: "gold", TagType: "warning"}}, opts)
	assert.True(t, h.cached("level"))

	// Served from the cache: a direct database change is not visible.
	require.NoError(t, h.db.Exec("UPDATE sys_dict_items SET label = 'X' WHERE value = 'silver'").Error)
	h.ok(h.do(http.MethodGet, "/dicts/level/items", nil), &opts)
	assert.Equal(t, "白银", opts[0].Label)

	h.perms = allCodes()
	// Item update invalidates.
	h.ok(h.do(http.MethodPut, "/dict-items/"+gold.ID.String(), gin.H{"label": "金", "value": "gold", "sort": 2, "status": 2, "version": 0}), nil)
	assert.False(t, h.cached("level"))
	h.ok(h.do(http.MethodGet, "/dicts/level/items", nil), &opts)
	assert.Equal(t, []DictOption{{Label: "X", Value: "silver"}}, opts)

	// Item create and delete invalidate.
	added := h.createItem(tid, "钻石", "diamond", 9, nil)
	assert.False(t, h.cached("level"))
	h.ok(h.do(http.MethodGet, "/dicts/level/items", nil), &opts)
	assert.Len(t, opts, 2)
	h.ok(h.do(http.MethodDelete, "/dict-items/"+added.ID.String(), nil), nil)
	assert.False(t, h.cached("level"))

	// Disabling the type hides every item.
	h.ok(h.do(http.MethodGet, "/dicts/level/items", nil), &opts)
	h.ok(h.do(http.MethodPut, "/dict-types/"+tid, gin.H{"name": "等级", "status": 2, "version": 0}), nil)
	assert.False(t, h.cached("level"))
	h.ok(h.do(http.MethodGet, "/dicts/level/items", nil), &opts)
	assert.Empty(t, opts)
	assert.NotNil(t, opts, "empty list, not null")

	// Deleting the type invalidates and the code becomes unknown.
	h.ok(h.do(http.MethodDelete, "/dict-types/"+tid, nil), nil)
	assert.False(t, h.cached("level"))
	h.fail(h.do(http.MethodGet, "/dicts/level/items", nil), http.StatusNotFound, httpx.CodeNotFound)
	h.fail(h.do(http.MethodGet, "/dicts/NOT-valid/items", nil), http.StatusNotFound, httpx.CodeNotFound)
	assert.False(t, h.cached("level"), "unknown codes are not cached")

	h.anonymous = true
	h.fail(h.do(http.MethodGet, "/dicts/level/items", nil), http.StatusUnauthorized, 20001)
}

func TestDictOptionsFallBackWhenRedisFails(t *testing.T) {
	h := newHarness(t)
	h.createItem(h.createType("color", "颜色").ID.String(), "红", "red", 0, gin.H{"tagType": "danger"})
	ctx := context.Background()

	// Corrupt entry: ignored and rewritten.
	require.NoError(t, h.rdb.Set(ctx, dictCacheKey("color"), "not json", 0).Err())
	opts, err := h.mod.DictOptions(ctx, "color")
	require.NoError(t, err)
	assert.Equal(t, []DictOption{{Label: "红", Value: "red", TagType: "danger"}}, opts)

	// Redis unavailable: served from the database.
	broken := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	require.NoError(t, broken.Close())
	m := New(Deps{DB: h.db, Redis: broken, Logger: zap.NewNop()})
	opts, err = m.DictOptions(ctx, "color")
	require.NoError(t, err)
	assert.Len(t, opts, 1)
	m.invalidateDict(ctx, "color") // logs only

	// No Redis configured at all.
	opts, err = New(Deps{DB: h.db}).DictOptions(ctx, "color")
	require.NoError(t, err)
	assert.Len(t, opts, 1)
	_, err = New(Deps{DB: h.db}).DictOptions(ctx, "missing_code")
	assert.ErrorIs(t, err, ErrDictNotFound)
}

func TestDictPermissions(t *testing.T) {
	h := newHarness(t)
	typ := h.createType("perm_test", "权限").ID.String()
	h.perms = []string{permDictView}
	h.ok(h.do(http.MethodGet, "/dict-types", nil), nil)
	h.fail(h.do(http.MethodPost, "/dict-types", gin.H{"code": "x_y", "name": "x"}), http.StatusForbidden, 20002)
	h.fail(h.do(http.MethodPut, "/dict-types/"+typ, gin.H{"name": "x", "version": 0}), http.StatusForbidden, 20002)
	h.fail(h.do(http.MethodDelete, "/dict-types/"+typ, nil), http.StatusForbidden, 20002)
	h.fail(h.do(http.MethodPost, "/dict-types/"+typ+"/items", gin.H{"label": "x", "value": "y"}), http.StatusForbidden, 20002)
	h.perms = nil
	h.fail(h.do(http.MethodGet, "/dict-types", nil), http.StatusForbidden, 20002)
}

// A reader that loaded rows before an invalidation must not put them back
// into the cache afterwards.
func TestDictRefillCannotOverwriteInvalidation(t *testing.T) {
	h := newHarness(t)
	h.createType("race", "竞争")
	ctx := context.Background()

	ver := h.mod.dictVersion(ctx, "race") // the reader captures the version...
	stale := []DictOption{{Label: "旧", Value: "old"}}
	h.mod.invalidateDict(ctx, "race")             // ...a writer commits and invalidates...
	h.mod.writeDictCache(ctx, "race", stale, ver) // ...then the reader tries to fill
	assert.False(t, h.cached("race"), "the stale refill is dropped")

	ok, err := h.mod.HasDictValue(ctx, "no_such_dict", "x")
	require.NoError(t, err, "a missing dictionary is not an error for validation")
	assert.False(t, ok)
}
