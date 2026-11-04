//go:build integration

package system

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/testinfra/dbtest"
)

func (h *harness) changes(id string) []configChangeView {
	h.t.Helper()
	var page httpx.Page[configChangeView]
	h.ok(h.do(http.MethodGet, "/configs/"+id+"/changes", nil), &page)
	require.Equal(h.t, int64(len(page.List)), page.Total)
	return page.List
}

func TestConfigCRUDAndHistory(t *testing.T) {
	h := newHarness(t)
	var created configView
	h.ok(h.do(http.MethodPost, "/configs", gin.H{"key": "order.auto_close_minutes", "value": " 30 ",
		"valueType": "number", "groupName": "订单", "name": "自动关单"}), &created)
	assert.Equal(t, "30", created.Value, "numbers are trimmed")
	assert.True(t, created.Editable)
	assert.False(t, created.Builtin)
	id := created.ID.String()

	hist := h.changes(id)
	require.Len(t, hist, 1)
	assert.Nil(t, hist[0].OldValue, "creation has no old value")
	assert.Equal(t, "30", hist[0].NewValue)
	require.NotNil(t, hist[0].ChangedBy)
	assert.Equal(t, h.userID, int64(*hist[0].ChangedBy))
	assert.Equal(t, "配置管理员", hist[0].ChangedByName)

	var updated configView
	h.ok(h.do(http.MethodPut, "/configs/"+id, gin.H{"key": "ignored", "value": "45", "valueType": "number",
		"groupName": "订单", "name": "自动关单时间", "version": 0}), &updated)
	assert.Equal(t, "order.auto_close_minutes", updated.Key, "key is immutable")
	assert.Equal(t, "45", updated.Value)
	assert.Equal(t, 1, updated.Version)
	hist = h.changes(id)
	require.Len(t, hist, 2)
	require.NotNil(t, hist[0].OldValue)
	assert.Equal(t, "30", *hist[0].OldValue, "newest first")
	assert.Equal(t, "45", hist[0].NewValue)

	// Metadata-only change: no history row.
	h.ok(h.do(http.MethodPut, "/configs/"+id, gin.H{"value": "45", "valueType": "number", "groupName": "订单",
		"name": "关单", "description": "分钟", "version": 1}), &updated)
	assert.Len(t, h.changes(id), 2)

	h.fail(h.do(http.MethodPut, "/configs/"+id, gin.H{"value": "1", "valueType": "number", "name": "x", "version": 0}),
		http.StatusConflict, httpx.CodeConflict)
	assert.Len(t, h.changes(id), 2, "a rejected write leaves no history")
	assert.Equal(t, []string{"value"}, h.fields(h.do(http.MethodPut, "/configs/"+id,
		gin.H{"value": "abc", "valueType": "number", "name": "x", "version": 2})))
	assert.Equal(t, []string{"version"}, h.fields(h.do(http.MethodPut, "/configs/"+id, gin.H{"value": "1", "name": "x"})))

	h.ok(h.do(http.MethodDelete, "/configs/"+id, nil), nil)
	h.fail(h.do(http.MethodGet, "/configs/"+id+"/changes", nil), http.StatusNotFound, httpx.CodeNotFound)
	assert.Zero(t, h.count("SELECT count(*) FROM sys_config_changes WHERE config_id = ?", created.ID))
}

func TestConfigValidationAndUniqueness(t *testing.T) {
	h := newHarness(t)
	post := func(body gin.H) response { return h.do(http.MethodPost, "/configs", body) }
	assert.Equal(t, []string{"key"}, h.fields(post(gin.H{"key": "Bad Key", "value": "x", "name": "n"})))
	assert.Equal(t, []string{"valueType"}, h.fields(post(gin.H{"key": "a.b", "value": "x", "valueType": "date", "name": "n"})))
	assert.Equal(t, []string{"value"}, h.fields(post(gin.H{"key": "a.b", "value": "yes", "valueType": "boolean", "name": "n"})))
	assert.Equal(t, []string{"value"}, h.fields(post(gin.H{"key": "a.b", "value": "{bad", "valueType": "json", "name": "n"})))
	assert.Equal(t, []string{"name"}, h.fields(post(gin.H{"key": "a.b", "value": "x", "name": ""})))

	var v configView
	h.ok(post(gin.H{"key": "feature.flags", "value": `{"beta": true}`, "valueType": "json", "name": "开关"}), &v)
	assert.Equal(t, "default", v.GroupName)
	h.ok(post(gin.H{"key": "feature.enabled", "value": "true", "valueType": "boolean", "name": "启用", "groupName": "功能"}), nil)
	h.ok(post(gin.H{"key": "feature.title", "value": "hi", "name": "标题", "groupName": "功能"}), nil)
	assert.Equal(t, []string{"key"}, h.fields(post(gin.H{"key": "feature.flags", "value": "x", "name": "n"})))

	var page httpx.Page[configView]
	h.ok(h.do(http.MethodGet, "/configs?group=功能", nil), &page)
	assert.Equal(t, int64(2), page.Total)
	h.ok(h.do(http.MethodGet, "/configs?keyword=site.", nil), &page)
	assert.Zero(t, page.Total, "built-in settings are hidden by default")
	h.ok(h.do(http.MethodGet, "/configs?keyword=site.&includeBuiltin=true", nil), &page)
	assert.Equal(t, int64(2), page.Total, "site.name and site.logo_url")
	for _, c := range page.List {
		assert.True(t, c.Builtin)
	}
	h.ok(h.do(http.MethodGet, "/configs", nil), &page)
	for _, c := range page.List {
		assert.False(t, c.Builtin, "%s is built-in", c.Key)
	}
	h.ok(h.do(http.MethodGet, "/configs?keyword=%25", nil), &page)
	assert.Equal(t, int64(0), page.Total, "LIKE wildcards are escaped")

	var groups []string
	h.ok(h.do(http.MethodGet, "/configs/groups", nil), &groups)
	assert.Equal(t, []string{"default", "功能"}, groups, "groups of built-in settings are hidden too")
	h.ok(h.do(http.MethodGet, "/configs/groups?includeBuiltin=true", nil), &groups)
	assert.Equal(t, []string{"default", "上传设置", "功能", "导出设置", "日志设置", "界面设置", "站点设置"}, groups)
}

func TestProtectedConfigs(t *testing.T) {
	h := newHarness(t)
	var builtinID, readOnlyID int64
	require.NoError(t, h.db.Raw(`SELECT id FROM sys_configs WHERE "key" = ?`, KeySiteName).Scan(&builtinID).Error)
	readOnlyID = dbtest.InsertID(t, h.db, `INSERT INTO sys_configs ("key", value, name, editable) VALUES ('ro.value', 'v', '只读', false)`)

	for _, id := range []int64{builtinID, readOnlyID} {
		path := "/configs/" + itoa(id)
		h.fail(h.do(http.MethodPut, path, gin.H{"value": "hacked", "name": "x", "version": 0}), http.StatusConflict, CodeProtected)
		h.fail(h.do(http.MethodDelete, path, nil), http.StatusConflict, CodeProtected)
	}
	var value string
	require.NoError(t, h.db.Raw("SELECT value FROM sys_configs WHERE id = ?", builtinID).Scan(&value).Error)
	assert.Equal(t, "Base Admin", value)
	assert.Equal(t, int64(2), h.count("SELECT count(*) FROM sys_configs WHERE id IN (?, ?)", builtinID, readOnlyID))

	// The upload policy cannot be widened through the generic config API.
	var extID int64
	require.NoError(t, h.db.Raw(`SELECT id FROM sys_configs WHERE "key" = ?`, KeyUploadAllowedExtensions).Scan(&extID).Error)
	h.fail(h.do(http.MethodPut, "/configs/"+itoa(extID), gin.H{"value": "html", "name": "x", "version": 0}),
		http.StatusConflict, CodeProtected)
}

func TestConfigPermissions(t *testing.T) {
	h := newHarness(t)
	h.perms = []string{permConfigView}
	h.ok(h.do(http.MethodGet, "/configs", nil), nil)
	h.fail(h.do(http.MethodPost, "/configs", gin.H{"key": "a.b", "value": "x", "name": "n"}), http.StatusForbidden, 20002)
	h.perms = []string{permDictView}
	h.fail(h.do(http.MethodGet, "/configs", nil), http.StatusForbidden, 20002)
	h.fail(h.do(http.MethodGet, "/configs/1/changes", nil), http.StatusForbidden, 20002)
}
