//go:build integration

package devtools

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/testinfra/dbtest"
)

// agentRepo is a fake repository holding a copy of the real migrations of
// both tracks, so the agent API can number, write and apply a new one.
func agentRepo(t *testing.T) string {
	t.Helper()
	root := fakeRepo(t)
	for _, sub := range []string{"", "app", "mysql", "app/mysql"} {
		dst := filepath.Join(root, "server", "migrations", sub)
		require.NoError(t, os.MkdirAll(dst, 0o750))
		src := filepath.Join("..", "..", "..", "migrations", sub)
		entries, err := os.ReadDir(src)
		require.NoError(t, err)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(src, e.Name())) //nolint:gosec // test fixture
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dst, e.Name()), raw, 0o600))
		}
	}
	return root
}

// agentUser creates a user, optionally holding PermCodegen through a role.
func (h *harness) agentUser(name string, codegen bool) int64 {
	h.t.Helper()
	uid := dbtest.InsertID(h.t, h.db, `INSERT INTO sys_users (username, password_hash) VALUES (?, 'x')`, name)
	if codegen {
		rid := dbtest.InsertID(h.t, h.db, `INSERT INTO sys_roles (code, name) VALUES (?, ?)`, name+"_role", name)
		require.NoError(h.t, h.db.Exec(`INSERT INTO sys_role_permissions (role_id, permission_code) VALUES (?, ?)`, rid, PermCodegen).Error)
		require.NoError(h.t, h.db.Exec(`INSERT INTO sys_user_roles (user_id, role_id) VALUES (?, ?)`, uid, rid).Error)
	}
	return uid
}

// newKey creates a key as uid through the management API.
func (h *harness) newKey(uid int64, body any) createdAgentKey {
	h.t.Helper()
	r := h.doAs(uid, http.MethodPost, "/api/v1/devtools/agent-keys", PermAgentKeys, body)
	require.Equal(h.t, http.StatusOK, r.status, string(r.body))
	var out createdAgentKey
	require.NoError(h.t, json.Unmarshal(r.env.Data, &out))
	return out
}

func (h *harness) doAs(uid int64, method, path, perms string, body any) response {
	h.t.Helper()
	return h.request(method, path, body, func(req *http.Request) {
		req.Header.Set("X-Test-User", strconv.FormatInt(uid, 10))
		req.Header.Set("X-Test-Perms", perms)
	})
}

// agent calls the agent API with key only (no session headers).
func (h *harness) agent(key, method, path string, body any) response {
	h.t.Helper()
	return h.request(method, "/api/v1/devtools/agent"+path, body, func(req *http.Request) {
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
	})
}

func (h *harness) request(method, path string, body any, mutate func(*http.Request)) response {
	h.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(h.t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	mutate(req)
	rec := httptest.NewRecorder()
	h.engine.ServeHTTP(rec, req)
	r := response{status: rec.Code, header: rec.Header(), body: rec.Body.Bytes()}
	_ = json.Unmarshal(r.body, &r.env)
	return r
}

func TestAgentKeyLifecycle(t *testing.T) {
	h := newHarness(t, config.EnvDev, "")
	uid := h.agentUser("keeper", true)

	bad := h.doAs(uid, http.MethodPost, "/api/v1/devtools/agent-keys", PermAgentKeys, map[string]any{"name": " ", "expiresInDays": 0})
	assert.Equal(t, http.StatusUnprocessableEntity, bad.status, string(bad.body))
	bad = h.doAs(uid, http.MethodPost, "/api/v1/devtools/agent-keys", PermAgentKeys, map[string]any{"name": "x", "expiresInDays": 31})
	assert.Equal(t, http.StatusUnprocessableEntity, bad.status, string(bad.body))

	created := h.newKey(uid, map[string]any{"name": "Claude Code", "expiresInDays": 30})
	assert.True(t, strings.HasPrefix(created.Key, agentKeyPrefix))
	assert.Len(t, created.Key, len(agentKeyPrefix)+43)
	assert.Equal(t, created.Key[:12], created.Item.Prefix)
	assert.Equal(t, "Claude Code", created.Item.Name)
	assert.Equal(t, "keeper", created.Item.UserName)
	require.NotNil(t, created.Item.ExpiresAt)
	assert.WithinDuration(t, time.Now().AddDate(0, 0, 30), *created.Item.ExpiresAt, time.Minute)

	var stored string
	require.NoError(t, h.db.Raw(`SELECT key_hash FROM sys_agent_keys WHERE id = ?`, int64(created.Item.ID)).Scan(&stored).Error)
	assert.Equal(t, hashAgentKey(created.Key), stored, "only the hash is stored")
	assert.NotContains(t, stored, created.Key)

	r := h.doAs(uid, http.MethodGet, "/api/v1/devtools/agent-keys", PermAgentKeys, nil)
	require.Equal(t, http.StatusOK, r.status)
	var list agentKeyList
	require.NoError(t, json.Unmarshal(r.env.Data, &list))
	assert.True(t, list.Enabled)
	assert.Equal(t, "/api/v1/devtools/agent", list.Endpoint)
	require.Len(t, list.Keys, 1)
	assert.NotContains(t, string(r.body), created.Key, "the key is never listed")

	id := strconv.FormatInt(int64(created.Item.ID), 10)
	assert.Equal(t, http.StatusOK, h.doAs(uid, http.MethodDelete, "/api/v1/devtools/agent-keys/"+id, PermAgentKeys, nil).status)
	assert.Equal(t, http.StatusNotFound, h.doAs(uid, http.MethodDelete, "/api/v1/devtools/agent-keys/999999", PermAgentKeys, nil).status)
	assert.Equal(t, http.StatusForbidden, h.doAs(uid, http.MethodGet, "/api/v1/devtools/agent-keys", PermCodegen, nil).status)

	short := h.newKey(uid, map[string]any{"name": "default"})
	require.NotNil(t, short.Item.ExpiresAt, "every key expires")
	assert.WithinDuration(t, time.Now().AddDate(0, 0, defaultKeyDays), *short.Item.ExpiresAt, time.Minute)
}

func TestAgentAuth(t *testing.T) {
	h := newHarness(t, config.EnvDev, "")
	dev := h.agentUser("dev", true)
	plain := h.agentUser("plain", false)
	key := h.newKey(dev, map[string]any{"name": "dev"}).Key
	noPerm := h.newKey(plain, map[string]any{"name": "plain"}).Key

	r := h.agent(key, http.MethodGet, "/tables", nil)
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	var used int
	require.NoError(t, h.db.Raw(`SELECT count(*) FROM sys_agent_keys WHERE last_used_at IS NOT NULL`).Scan(&used).Error)
	assert.Equal(t, 1, used, "last_used_at is recorded")

	guide := h.agent(key, http.MethodGet, "/guide", nil)
	require.Equal(t, http.StatusOK, guide.status)
	assert.Contains(t, guide.header.Get("Content-Type"), "text/markdown")
	assert.Contains(t, string(guide.body), "基础地址：`/api/v1/devtools/agent`")
	assert.NotContains(t, string(guide.body), "{{BASE}}")

	for name, k := range map[string]string{"missing": "", "malformed": "abc", "unknown": agentKeyPrefix + "nope"} {
		r := h.agent(k, http.MethodGet, "/tables", nil)
		assert.Equal(t, http.StatusUnauthorized, r.status, name)
	}
	assert.Equal(t, http.StatusForbidden, h.agent(noPerm, http.MethodGet, "/tables", nil).status, "creator lacks codegen permission")

	require.NoError(t, h.db.Exec(`UPDATE sys_users SET status = 2 WHERE id = ?`, dev).Error)
	assert.Equal(t, http.StatusForbidden, h.agent(key, http.MethodGet, "/tables", nil).status, "disabled creator")
	require.NoError(t, h.db.Exec(`UPDATE sys_users SET status = 1 WHERE id = ?`, dev).Error)

	require.NoError(t, h.db.Exec(`UPDATE sys_agent_keys SET expires_at = ? WHERE key_hash = ?`, time.Now().Add(-time.Minute), hashAgentKey(key)).Error)
	assert.Equal(t, http.StatusUnauthorized, h.agent(key, http.MethodGet, "/tables", nil).status, "expired")
	require.NoError(t, h.db.Exec(`UPDATE sys_agent_keys SET expires_at = NULL, revoked_at = CURRENT_TIMESTAMP(6) WHERE key_hash = ?`, hashAgentKey(key)).Error)
	assert.Equal(t, http.StatusUnauthorized, h.agent(key, http.MethodGet, "/tables", nil).status, "revoked")
}

func TestAgentIsDevOnly(t *testing.T) {
	h := newHarness(t, config.EnvProd, "")
	uid := h.agentUser("prod", true)
	key := h.newKey(uid, map[string]any{"name": "prod"}).Key
	r := h.agent(key, http.MethodPost, "/tables", map[string]any{"name": "biz_x"})
	assert.Equal(t, http.StatusForbidden, r.status)
	assert.Equal(t, CodeCodegenDisabled, r.env.Code)
	// Refused before any key lookup, the guide included.
	for _, k := range []string{key, "", "bak_guess"} {
		r := h.agent(k, http.MethodGet, "/guide", nil)
		assert.Equal(t, http.StatusForbidden, r.status, k)
		assert.Equal(t, CodeCodegenDisabled, r.env.Code, k)
	}
}

func TestAgentCreateTable(t *testing.T) {
	root := agentRepo(t)
	h := newHarness(t, config.EnvDev, root)
	key := h.newKey(h.agentUser("builder", true), map[string]any{"name": "builder"}).Key
	dir := filepath.Join(root, filepath.FromSlash(appMigrationsDir(database.DialectOf(h.db))))
	before, err := os.ReadDir(dir)
	require.NoError(t, err)

	category := map[string]any{
		"name": "biz_categories", "label": "分类", "status": true,
		"columns": []map[string]any{{"name": "name", "type": "string", "label": "名称", "required": true, "unique": true}},
	}
	dry := h.agent(key, http.MethodPost, "/tables", map[string]any{
		"name": category["name"], "label": category["label"], "columns": category["columns"], "dryRun": true,
	})
	require.Equal(t, http.StatusOK, dry.status, string(dry.body))
	var view createTableView
	require.NoError(t, json.Unmarshal(dry.env.Data, &view))
	assert.False(t, view.Applied)
	assert.Contains(t, view.Up, "CREATE TABLE biz_categories")
	after, _ := os.ReadDir(dir)
	assert.Len(t, after, len(before), "dryRun writes nothing")
	assert.False(t, h.tableExists("biz_categories"), "dryRun leaves the database unchanged")

	r := h.agent(key, http.MethodPost, "/tables", category)
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	require.NoError(t, json.Unmarshal(r.env.Data, &view))
	assert.True(t, view.Applied)
	assert.Regexp(t, `^\d{6}_create_biz_categories$`, view.Migration)
	assert.FileExists(t, filepath.Join(dir, view.Migration+".up.sql"))
	assert.FileExists(t, filepath.Join(dir, view.Migration+".down.sql"))
	assert.True(t, h.tableExists("biz_categories"))

	// A retry of the same request (lost response) succeeds without changes;
	// a different definition of an existing table is rejected.
	again := h.agent(key, http.MethodPost, "/tables", category)
	require.Equal(t, http.StatusOK, again.status, string(again.body))
	var repeated createTableView
	require.NoError(t, json.Unmarshal(again.env.Data, &repeated))
	assert.Equal(t, view.Migration, repeated.Migration)
	assert.True(t, repeated.Applied)
	assert.Contains(t, repeated.Notes[0], "重复请求")
	changed := map[string]any{"name": "biz_categories", "label": "分类", "columns": category["columns"]}
	conflict := h.agent(key, http.MethodPost, "/tables", changed)
	assert.Equal(t, http.StatusUnprocessableEntity, conflict.status)
	assert.Contains(t, string(conflict.body), "已存在")

	// A migration on disk that the database has not run blocks table
	// creation instead of being run along with it.
	pending := filepath.Join(dir, "000099_manual.up.sql")
	require.NoError(t, os.WriteFile(pending, []byte("CREATE TABLE biz_manual (id bigint);"), 0o600))
	blocked := h.agent(key, http.MethodPost, "/tables", map[string]any{
		"name": "biz_tags", "label": "标签",
		"columns": []map[string]any{{"name": "name", "type": "string", "label": "名称"}},
	})
	assert.Equal(t, http.StatusConflict, blocked.status, string(blocked.body))
	assert.Equal(t, CodeMigrationState, blocked.env.Code)
	assert.False(t, h.tableExists("biz_manual"))
	assert.False(t, h.tableExists("biz_tags"))
	require.NoError(t, os.Remove(pending))

	products := map[string]any{
		"name": "biz_products", "label": "商品",
		"columns": []map[string]any{
			{"name": "title", "type": "string", "label": "名称", "required": true},
			{"name": "category_id", "type": "ref", "ref": "biz_categories", "label": "分类"},
			{"name": "brand_id", "type": "ref", "ref": "biz_brands", "label": "品牌"},
		},
	}
	bad := h.agent(key, http.MethodPost, "/tables", products)
	assert.Equal(t, http.StatusUnprocessableEntity, bad.status)
	assert.Contains(t, string(bad.body), "columns[2].ref")

	products["columns"] = products["columns"].([]map[string]any)[:2]
	r = h.agent(key, http.MethodPost, "/tables", products)
	require.Equal(t, http.StatusOK, r.status, string(r.body))
	var second createTableView
	require.NoError(t, json.Unmarshal(r.env.Data, &second))
	assert.Greater(t, second.Migration, view.Migration, "numbered after the previous one")

	fields := h.agent(key, http.MethodGet, "/fields?table=biz_products", nil)
	require.Equal(t, http.StatusOK, fields.status, string(fields.body))
	assert.Contains(t, string(fields.body), `"relationTable":"biz_categories"`)

	invalid := h.agent(key, http.MethodPost, "/tables", map[string]any{"name": "sys_x", "label": "x", "columns": []any{}})
	assert.Equal(t, http.StatusUnprocessableEntity, invalid.status)
	assert.Zero(t, h.countTables(`tmp\_try\_%`), "the scratch tables of the MySQL check are dropped")
}

func (h *harness) tableExists(name string) bool {
	h.t.Helper()
	return h.countTables(name) == 1
}

// countTables counts the tables of the current schema whose name is LIKE
// pattern.
func (h *harness) countTables(pattern string) int {
	h.t.Helper()
	schema := "current_schema()"
	if database.IsMySQL(h.db) {
		schema = "DATABASE()"
	}
	var n int
	require.NoError(h.t, h.db.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema = `+schema+` AND table_name LIKE ?`, pattern).Scan(&n).Error)
	return n
}
