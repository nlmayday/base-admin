//go:build integration

package system

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/core/export"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/platform/httpx"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func (h *harness) settings() settingsView {
	h.t.Helper()
	var v settingsView
	h.ok(h.do(http.MethodGet, "/settings", nil), &v)
	return v
}

func settingsBody(v settingsView) gin.H {
	return gin.H{"siteName": v.SiteName, "logoUrl": v.LogoURL, "uploadAllowedExtensions": v.UploadAllowedExtensions,
		"uploadMaxSizeMB": v.UploadMaxSizeMB, "auditRetentionDays": v.AuditRetentionDays,
		"exportMaxRows": v.ExportMaxRows, "pageSize": v.PageSize, "version": v.Version}
}

func TestSettingsDefaultsAndUpdate(t *testing.T) {
	h := newHarness(t)
	cur := h.settings()
	assert.Equal(t, "Base Admin", cur.SiteName)
	assert.Equal(t, "", cur.LogoURL)
	assert.Equal(t, strings.Split(defaultExtensionCSV, ","), cur.UploadAllowedExtensions)
	assert.Equal(t, 20, cur.UploadMaxSizeMB)
	assert.Equal(t, 180, cur.AuditRetentionDays)
	assert.Equal(t, 0, cur.Version)

	assert.Equal(t, 100000, cur.ExportMaxRows)
	assert.Equal(t, 20, cur.PageSize)

	body := settingsBody(cur)
	body["siteName"] = "  运营后台 "
	body["uploadAllowedExtensions"] = []string{"PNG", ".jpg", "png"}
	body["uploadMaxSizeMB"] = 50
	var next settingsView
	h.ok(h.do(http.MethodPut, "/settings", body), &next)
	assert.Equal(t, "运营后台", next.SiteName)
	assert.Equal(t, []string{"png", "jpg"}, next.UploadAllowedExtensions)
	assert.Equal(t, 50, next.UploadMaxSizeMB)
	assert.Equal(t, 3, next.Version, "three rows changed")

	// Exactly one history row per changed key.
	var changed []string
	require.NoError(t, h.db.Raw(`SELECT "key" FROM sys_config_changes ORDER BY "key"`).Scan(&changed).Error)
	assert.Equal(t, []string{KeySiteName, KeyUploadAllowedExtensions, KeyUploadMaxSizeMB}, changed)
	var old string
	require.NoError(t, h.db.Raw(`SELECT old_value FROM sys_config_changes WHERE "key" = ?`, KeySiteName).Scan(&old).Error)
	assert.Equal(t, "Base Admin", old)
	assert.Equal(t, int64(3), h.count("SELECT count(*) FROM sys_config_changes WHERE changed_by = ?", h.userID))

	// Stale version.
	h.fail(h.do(http.MethodPut, "/settings", body), http.StatusConflict, httpx.CodeConflict)
	// Unchanged submit: no new history, same version.
	h.ok(h.do(http.MethodPut, "/settings", settingsBody(next)), &next)
	assert.Equal(t, 3, next.Version)
	assert.Equal(t, int64(3), h.count("SELECT count(*) FROM sys_config_changes"))

	// Export limit and page size.
	body = settingsBody(next)
	body["exportMaxRows"], body["pageSize"] = 5000, 50
	h.ok(h.do(http.MethodPut, "/settings", body), &next)
	assert.Equal(t, 5000, next.ExportMaxRows)
	assert.Equal(t, 50, next.PageSize)
	assert.Equal(t, 5000, export.MaxRows(context.Background(), h.db))

	// Go API sees local writes immediately.
	name, err := h.mod.GetString(context.Background(), KeySiteName)
	require.NoError(t, err)
	assert.Equal(t, "运营后台", name)
}

func TestSettingsValidation(t *testing.T) {
	h := newHarness(t)
	cur := h.settings()
	cases := []struct {
		field string
		value any
	}{
		{"siteName", ""},
		{"logoUrl", "javascript:alert(1)"},
		{"logoUrl", "//evil.example/x.png"},
		{"uploadAllowedExtensions", []string{"png", "html"}},
		{"uploadAllowedExtensions", []string{"svg"}},
		{"uploadAllowedExtensions", []string{"JS"}},
		{"uploadAllowedExtensions", []string{"exe"}},
		{"uploadAllowedExtensions", []string{"tar.gz"}},
		{"uploadAllowedExtensions", []string{}},
		{"uploadMaxSizeMB", 0},
		{"uploadMaxSizeMB", 1025},
		{"auditRetentionDays", 0},
		{"exportMaxRows", 999},
		{"exportMaxRows", 1000001},
		{"pageSize", 30},
	}
	for _, tc := range cases {
		body := settingsBody(cur)
		body[tc.field] = tc.value
		assert.Equal(t, []string{tc.field}, h.fields(h.do(http.MethodPut, "/settings", body)), "%s=%v", tc.field, tc.value)
	}
	body := settingsBody(cur)
	delete(body, "version")
	assert.Equal(t, []string{"version"}, h.fields(h.do(http.MethodPut, "/settings", body)))
	assert.Zero(t, h.count("SELECT count(*) FROM sys_config_changes"))

	body = settingsBody(cur)
	body["logoUrl"] = "/static/logo.png"
	h.ok(h.do(http.MethodPut, "/settings", body), nil)
	body = settingsBody(h.settings())
	body["logoUrl"] = "https://cdn.example.com/logo.png"
	h.ok(h.do(http.MethodPut, "/settings", body), nil)
}

func TestPublicSettingsExposeOnlySiteInfo(t *testing.T) {
	h := newHarness(t)
	body := settingsBody(h.settings())
	body["logoUrl"] = "/logo.png"
	h.ok(h.do(http.MethodPut, "/settings", body), nil)

	h.anonymous = true
	resp := h.do(http.MethodGet, "/settings/public", nil)
	h.ok(resp, nil)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(resp.Data, &raw))
	assert.Len(t, raw, 3)
	assert.JSONEq(t, `{"siteName":"Base Admin","logoUrl":"/logo.png","pageSize":20}`, string(resp.Data))

	h.fail(h.do(http.MethodGet, "/settings", nil), http.StatusUnauthorized, 20001)
	h.fail(h.do(http.MethodPut, "/settings", body), http.StatusUnauthorized, 20001)

	// A hand-edited unsafe logo is never served publicly.
	require.NoError(t, h.db.Exec(`UPDATE sys_configs SET value = 'javascript:x' WHERE "key" = ?`, KeySiteLogoURL).Error)
	h.mod.values.clear()
	h.ok(h.do(http.MethodGet, "/settings/public", nil), &raw)
	assert.JSONEq(t, `""`, string(raw["logoUrl"]))
}

func TestSettingsPermissions(t *testing.T) {
	h := newHarness(t)
	body := settingsBody(h.settings())
	h.perms = []string{permSettingsView}
	h.ok(h.do(http.MethodGet, "/settings", nil), nil)
	h.fail(h.do(http.MethodPut, "/settings", body), http.StatusForbidden, 20002)
	h.perms = []string{permConfigView}
	h.fail(h.do(http.MethodGet, "/settings", nil), http.StatusForbidden, 20002)
	h.ok(h.do(http.MethodGet, "/settings/public", nil), nil)
}

func TestGoAPIAndCache(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	exts, maxBytes, err := h.mod.UploadPolicy(ctx)
	require.NoError(t, err)
	assert.Equal(t, strings.Split(defaultExtensionCSV, ","), exts)
	assert.Equal(t, int64(20<<20), maxBytes)

	days, err := h.mod.GetInt(ctx, KeyAuditRetentionDays)
	require.NoError(t, err)
	assert.Equal(t, 180, days)
	_, err = h.mod.GetString(ctx, "no.such_key")
	assert.ErrorIs(t, err, ErrConfigNotFound)
	_, err = h.mod.GetInt(ctx, KeySiteName)
	assert.Error(t, err, "not an integer")

	// Cached: a direct database write is not seen until the cache clears.
	require.NoError(t, h.db.Exec(`UPDATE sys_configs SET value = 'png, HTML,svg,.PDF' WHERE "key" = ?`, KeyUploadAllowedExtensions).Error)
	exts, _, err = h.mod.UploadPolicy(ctx)
	require.NoError(t, err)
	assert.Len(t, exts, len(strings.Split(defaultExtensionCSV, ",")))

	// A write through the API clears the local cache; dangerous stored
	// extensions are filtered even if someone edited the row by hand.
	var created configView
	h.ok(h.do(http.MethodPost, "/configs", gin.H{"key": "x.y", "value": "1", "name": "n"}), &created)
	exts, _, err = h.mod.UploadPolicy(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"png", "pdf"}, exts)

	body := settingsBody(h.settings())
	body["uploadMaxSizeMB"] = 7
	h.ok(h.do(http.MethodPut, "/settings", body), nil)
	_, maxBytes, err = h.mod.UploadPolicy(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(7<<20), maxBytes)
}

func TestSeedIsIdempotentAndKeepsValues(t *testing.T) {
	h := newHarness(t)
	body := settingsBody(h.settings())
	body["siteName"] = "自定义名称"
	h.ok(h.do(http.MethodPut, "/settings", body), nil)
	require.NoError(t, h.db.Exec(`UPDATE sys_configs SET name = 'renamed', builtin = false WHERE "key" = ?`, KeySiteName).Error)

	ctx := context.Background()
	for range 2 {
		require.NoError(t, module.Sync(ctx, h.db, []module.Module{h.mod}))
	}
	assert.Equal(t, int64(len(builtinSettings)), h.count("SELECT count(*) FROM sys_configs"))
	var row configRow
	require.NoError(t, h.db.Raw(`SELECT * FROM sys_configs WHERE "key" = ?`, KeySiteName).Scan(&row).Error)
	assert.Equal(t, "自定义名称", row.Value, "admin value kept")
	assert.Equal(t, "站点名称", row.Name, "metadata restored")
	assert.True(t, row.Builtin)
	assert.Equal(t, int64(len(menuDefs)), h.count("SELECT count(*) FROM sys_menus WHERE module = ?", Name))
	assert.Equal(t, int64(len(permissionDefs)), h.count("SELECT count(*) FROM sys_permissions WHERE module = ?", Name))
}
