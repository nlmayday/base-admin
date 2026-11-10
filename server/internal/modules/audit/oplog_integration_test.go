//go:build integration

package audit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/core/module"
	"base-admin/server/internal/platform/httpx"
)

func TestModuleDefinitionsValid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := New(Deps{})
	engine := gin.New()
	reg := module.NewRegistry()
	m.Routes(module.NewRouter(engine.Group(apiPrefix), Name, testGuard{}, reg))
	rep := module.Validate([]module.Module{m}, reg, engine.Routes(), apiPrefix)
	require.NoError(t, rep.Err())
	assert.Empty(t, rep.Warnings, "every permission is used by a route")
	for _, r := range reg.Routes() {
		assert.NotEmpty(t, r.Doc.Summary, r.Path)
		assert.Equal(t, strings.HasSuffix(r.Path, "/export"), r.Doc.Sensitive, r.Path)
	}
	require.Len(t, m.Jobs(), 1)
	assert.Equal(t, "audit.cleanup", m.Jobs()[0].Key)
}

func TestOperationLogRecording(t *testing.T) {
	h := newHarness(t)
	alice := h.createUser("alice", "爱丽丝")

	r := h.call(http.MethodPost, "/api/v1/probe/items", `{"name":"pen","password":"hunter2","nested":{"token":"t"}}`, alice)
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	assert.JSONEq(t, `{"name":"pen","password":"hunter2","nested":{"token":"t"}}`, string(r.Data), "handler got the full body")

	h.call(http.MethodGet, "/api/v1/probe/items", "", alice)                            // plain read: not recorded
	h.call(http.MethodGet, "/api/v1/probe/export", "", alice)                           // sensitive read
	h.call(http.MethodPost, "/api/v1/auth/login", `{"username":"a","password":"b"}`, 0) // covered by login log
	h.call(http.MethodPost, "/api/v1/probe/items", `{"name":"anon"}`, 0)                // 401: not stored
	h.call(http.MethodDelete, "/api/v1/probe/items/5", "", alice)                       // 403
	h.call(http.MethodPost, "/api/v1/probe/items", `{"password":"x"`, alice)            // malformed JSON
	h.waitCount(tableOperationLogs, "TRUE", 4)
	assert.Zero(t, h.count(tableOperationLogs, "route LIKE '%/auth/login'"), "login is not in the operation log")
	assert.Zero(t, h.count(tableOperationLogs, "method = 'GET' AND route = '/api/v1/probe/items'"))

	var list httpx.Page[opLogView]
	h.decode(h.call(http.MethodGet, "/api/v1/audit/operation-logs?pageSize=100", "", alice, allViewPerms...), &list)
	require.EqualValues(t, 4, list.Total)

	created := findOp(t, list.List, func(v opLogView) bool { return v.Success && v.Method == http.MethodPost })
	assert.Equal(t, "新增条目", created.Summary)
	assert.Equal(t, "probe", created.Module)
	assert.Equal(t, "/api/v1/probe/items", created.Route)
	require.NotNil(t, created.UserID)
	assert.EqualValues(t, alice, *created.UserID)
	assert.Equal(t, "alice", created.Username)
	assert.Equal(t, "爱丽丝", created.Nickname)
	assert.Equal(t, "audit-test/1.0", created.UserAgent)
	assert.NotEmpty(t, created.RequestID)
	assert.Equal(t, http.StatusOK, created.Status)

	var detail opLogDetail
	h.decode(h.call(http.MethodGet, "/api/v1/audit/operation-logs/"+created.ID.String(), "", alice, PermOperationLogView), &detail)
	assert.JSONEq(t, `{"name":"pen","password":"***","nested":{"token":"***"}}`, detail.RequestBody)
	assert.NotContains(t, detail.RequestBody, "hunter2")

	export := findOp(t, list.List, func(v opLogView) bool { return v.Method == http.MethodGet })
	assert.Equal(t, "导出条目", export.Summary)

	for _, v := range list.List {
		assert.NotNil(t, v.UserID, "unauthenticated requests are not stored")
	}

	denied := findOp(t, list.List, func(v opLogView) bool { return v.Status == http.StatusForbidden })
	assert.Equal(t, "删除条目", denied.Summary)
	assert.EqualValues(t, alice, *denied.UserID, "403 attempts keep the user")
	assert.Equal(t, "/api/v1/probe/items/:id", denied.Route)
	assert.Equal(t, "/api/v1/probe/items/5", denied.Path)

	bad := findOp(t, list.List, func(v opLogView) bool { return v.Status == http.StatusBadRequest })
	assert.Equal(t, httpx.CodeBadRequest, bad.Code)
	h.decode(h.call(http.MethodGet, "/api/v1/audit/operation-logs/"+bad.ID.String(), "", alice, PermOperationLogView), &detail)
	assert.Equal(t, bodyInvalid, detail.RequestBody, "unparseable bodies are never stored raw")
}

func findOp(t *testing.T, list []opLogView, match func(opLogView) bool) opLogView {
	t.Helper()
	for _, v := range list {
		if match(v) {
			return v
		}
	}
	require.FailNow(t, "no matching operation log")
	return opLogView{}
}

func TestOperationLogFilters(t *testing.T) {
	h := newHarness(t)
	bob := h.createUser("bob", "鲍勃")
	carol := h.createUser("carol", "卡罗尔")
	now := time.Now()
	h.insertOp(&bob, "iam", "POST", "/api/v1/users", "新增用户", true, now.Add(-time.Hour))
	h.insertOp(&bob, "iam", "DELETE", "/api/v1/users/3", "删除用户", false, now.Add(-2*time.Hour))
	h.insertOp(&carol, "system", "PUT", "/api/v1/configs/1", "编辑参数", true, now.Add(-48*time.Hour))
	h.insertOp(nil, "auth", "POST", "/api/v1/auth/logout", "退出登录", false, now.Add(-time.Minute))

	cases := []struct {
		query string
		want  int
	}{
		{"", 4},
		{"user=bo", 2},
		{"user=" + url.QueryEscape("卡罗"), 1},
		{"module=iam", 2},
		{"method=DELETE", 1},
		{"success=false", 2},
		{"success=true&module=iam", 1},
		{"path=configs", 1},
		{"path=" + url.QueryEscape("用户"), 2},
		{"path=" + url.QueryEscape("%"), 0},
		{"startTime=" + url.QueryEscape(now.Add(-3*time.Hour).Format(time.RFC3339)), 3},
		{"endTime=" + url.QueryEscape(now.Add(-24*time.Hour).Format(time.RFC3339)), 1},
	}
	for _, tc := range cases {
		var page httpx.Page[opLogView]
		h.decode(h.call(http.MethodGet, "/api/v1/audit/operation-logs?"+tc.query, "", bob, PermOperationLogView), &page)
		assert.EqualValues(t, tc.want, page.Total, tc.query)
	}

	var page httpx.Page[opLogView]
	h.decode(h.call(http.MethodGet, "/api/v1/audit/operation-logs?page=1&pageSize=2", "", bob, PermOperationLogView), &page)
	require.Len(t, page.List, 2)
	assert.Equal(t, "退出登录", page.List[0].Summary, "newest first")

	bad := h.call(http.MethodGet, "/api/v1/audit/operation-logs?startTime=yesterday", "", bob, PermOperationLogView)
	assert.Equal(t, http.StatusUnprocessableEntity, bad.Status)
	inverted := "startTime=" + url.QueryEscape(now.Format(time.RFC3339)) + "&endTime=" + url.QueryEscape(now.Add(-time.Hour).Format(time.RFC3339))
	assert.Equal(t, http.StatusUnprocessableEntity, h.call(http.MethodGet, "/api/v1/audit/operation-logs?"+inverted, "", bob, PermOperationLogView).Status)
	assert.Equal(t, http.StatusBadRequest, h.call(http.MethodGet, "/api/v1/audit/operation-logs?method=TRACE", "", bob, PermOperationLogView).Status)
	assert.Equal(t, http.StatusNotFound, h.call(http.MethodGet, "/api/v1/audit/operation-logs/999999", "", bob, PermOperationLogView).Status)
	assert.Equal(t, http.StatusNotFound, h.call(http.MethodGet, "/api/v1/audit/operation-logs/abc", "", bob, PermOperationLogView).Status)
}

func TestOperationLogPermissions(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("dave", "")
	cases := []struct {
		path  string
		perms []string
		want  int
	}{
		{"/api/v1/audit/operation-logs", nil, http.StatusForbidden},
		{"/api/v1/audit/operation-logs", []string{PermLoginLogView}, http.StatusForbidden},
		{"/api/v1/audit/operation-logs", []string{PermOperationLogView}, http.StatusOK},
		{"/api/v1/audit/operation-logs/export", []string{PermOperationLogView}, http.StatusForbidden},
		{"/api/v1/audit/operation-logs/export", []string{PermOperationLogExport}, http.StatusOK},
		{"/api/v1/audit/login-logs", []string{PermOperationLogView}, http.StatusForbidden},
		{"/api/v1/audit/login-logs/export", []string{PermLoginLogView}, http.StatusForbidden},
		{"/api/v1/audit/stats", nil, http.StatusForbidden},
		{"/api/v1/audit/stats", []string{PermLoginLogView}, http.StatusOK},
		{"/api/v1/audit/stats", []string{PermOperationLogView}, http.StatusOK},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, h.call(http.MethodGet, tc.path, "", uid, tc.perms...).Status, "%s %v", tc.path, tc.perms)
	}
	assert.Equal(t, http.StatusUnauthorized, h.call(http.MethodGet, "/api/v1/audit/operation-logs", "", 0).Status)
}

func TestOperationLogExport(t *testing.T) {
	h := newHarness(t)
	evil := h.createUser("=cmd", "@nick")
	h.insertOp(&evil, "iam", "POST", "/api/v1/users", "=HYPERLINK(\"x\")", true, time.Now())

	r := h.call(http.MethodGet, "/api/v1/audit/operation-logs/export?module=iam", "", evil, PermOperationLogExport)
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	assert.Contains(t, r.Header.Get("Content-Type"), "text/csv")
	assert.Contains(t, r.Header.Get("Content-Disposition"), "operation-logs-")
	body := strings.TrimPrefix(r.Body, "\xef\xbb\xbf")
	lines := strings.Split(strings.TrimSpace(body), "\n")
	require.Len(t, lines, 2, body)
	assert.Contains(t, lines[0], "耗时(ms)")
	assert.Contains(t, lines[1], "'=cmd")
	assert.Contains(t, lines[1], "'@nick")
	assert.Contains(t, lines[1], `"'=HYPERLINK(""x"")"`)
	assert.Contains(t, lines[1], "成功")

	// The export itself is a sensitive read and is audited.
	h.waitCount(tableOperationLogs, "summary = '导出操作日志' AND user_id = ?", 1, evil)
}

func TestOperationLogBodyCaptureLimits(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("erin", "")
	big, err := json.Marshal(map[string]string{"blob": strings.Repeat("x", 5000), "password": "hunter2"})
	require.NoError(t, err)
	r := h.call(http.MethodPost, "/api/v1/probe/items", string(big), uid)
	require.Equal(t, http.StatusOK, r.Status, "large bodies still reach the handler")
	h.waitCount(tableOperationLogs, "TRUE", 1)
	var body string
	require.NoError(t, h.db.Raw("SELECT request_body FROM sys_operation_logs").Scan(&body).Error)
	assert.Equal(t, bodyTooLarge, body)
}

func TestOperationLogModuleNames(t *testing.T) {
	h := newHarness(t)
	alice := h.createUser("alice", "爱丽丝")
	require.NoError(t, h.db.Exec(`INSERT INTO sys_menus (code, type, title, sort, source, module) VALUES
		('sales.root', 'dir', '销售管理', 2, 'module', 'sales'), ('ops.root', 'dir', '运维管理', 1, 'module', 'ops'),
		('auth.security', 'dir', '安全中心', 3, 'module', 'auth'), ('x.root', 'dir', '自建目录', 0, 'custom', '')`).Error)
	require.NoError(t, h.db.Exec(`INSERT INTO sys_menus (code, type, title, sort, source, module, parent_id)
		SELECT 'sales.sub', 'dir', '子目录', 0, 'module', 'sales', id FROM sys_menus WHERE code = 'sales.root'`).Error)

	var names []moduleName
	h.decode(h.call(http.MethodGet, "/api/v1/audit/operation-logs/modules", "", alice, PermOperationLogView), &names)
	assert.Equal(t, []moduleName{{"ops", "运维管理"}, {"sales", "销售管理"}, {"auth", "安全中心"}}, names,
		"top-level module directories only, in menu order")

	r := h.call(http.MethodGet, "/api/v1/audit/operation-logs/modules", "", alice)
	assert.Equal(t, http.StatusForbidden, r.Status)
}

func TestOperationLogTarget(t *testing.T) {
	h := newHarness(t)
	alice := h.createUser("alice", "爱丽丝")
	require.Equal(t, http.StatusOK, h.call(http.MethodPut, "/api/v1/probe/players/123/ban", "", alice).Status)
	require.Equal(t, http.StatusConflict, h.call(http.MethodPut, "/api/v1/probe/players/123/ban?fail=1", "", alice).Status)
	require.Equal(t, http.StatusOK, h.call(http.MethodPut, "/api/v1/probe/players/1234/ban", "", alice).Status)
	require.Equal(t, http.StatusOK, h.call(http.MethodPost, "/api/v1/probe/players/ban", "{}", alice).Status)
	require.Equal(t, http.StatusOK, h.call(http.MethodPost, "/api/v1/probe/items", `{"name":"pen"}`, alice).Status)
	h.call(http.MethodDelete, "/api/v1/probe/items/77", "", alice, "probe:item:delete")
	h.waitCount(tableOperationLogs, "TRUE", 6)

	var list httpx.Page[opLogView]
	h.decode(h.call(http.MethodGet, "/api/v1/audit/operation-logs?target="+url.QueryEscape("player:123"), "", alice, allViewPerms...), &list)
	require.EqualValues(t, 2, list.Total, "exact match, failed attempts included")
	for _, v := range list.List {
		assert.Equal(t, "player:123", v.Target)
		assert.Equal(t, "封禁玩家", v.Summary)
	}
	h.decode(h.call(http.MethodGet, "/api/v1/audit/operation-logs?pageSize=100", "", alice, allViewPerms...), &list)
	targets := map[string]bool{}
	for _, v := range list.List {
		targets[v.Target] = true
	}
	assert.True(t, targets["player:1,player:2"], "batch targets are joined")
	assert.True(t, targets[""], "requests without a target")
	assert.True(t, targets["item:77"], "a route with :id gets its target from the path")

	export := h.call(http.MethodGet, "/api/v1/audit/operation-logs/export?target="+url.QueryEscape("player:1234"), "", alice, allViewPerms...)
	require.Equal(t, http.StatusOK, export.Status)
	assert.Contains(t, export.Body, "操作对象")
	assert.Contains(t, export.Body, "player:1234")
	assert.NotContains(t, export.Body, "player:123,")
}

// insertMany adds n rows to table in a few statements; values is one row's
// VALUES tuple without parameters.
func (h *harness) insertMany(table, columns, values string, n int) {
	h.t.Helper()
	for n > 0 {
		k := min(n, 1000)
		tuples := strings.TrimSuffix(strings.Repeat(values+",", k), ",")
		require.NoError(h.t, h.db.Exec("INSERT INTO "+table+" ("+columns+") VALUES "+tuples).Error)
		n -= k
	}
}

// Large log tables are counted only up to the cap, and pages past it are
// refused, so the list never runs COUNT(*) over millions of rows.
func TestLogListsCountCap(t *testing.T) {
	h := newHarness(t)
	bob := h.createUser("bob", "鲍勃")
	h.insertMany(tableOperationLogs,
		"user_id, module, method, route, path, summary, status, code, success, duration_ms, created_at",
		fmt.Sprintf("(%d, 'iam', 'GET', '/x', '/x', '查看', 200, 0, TRUE, 5, CURRENT_TIMESTAMP(6))", bob), logCountCap+5)
	h.insertMany(tableLoginLogs, "kind, username, success, reason, ip, created_at",
		"('login', 'bob', TRUE, '', '10.0.0.1', CURRENT_TIMESTAMP(6))", logCountCap+5)

	for _, tc := range []struct{ path, perm string }{
		{"/api/v1/audit/operation-logs", PermOperationLogView},
		{"/api/v1/audit/login-logs", PermLoginLogView},
	} {
		var page httpx.Page[json.RawMessage]
		h.decode(h.call(http.MethodGet, tc.path+"?pageSize=20", "", bob, tc.perm), &page)
		assert.EqualValues(t, logCountCap, page.Total, tc.path)
		assert.True(t, page.TotalCapped, tc.path)
		assert.Len(t, page.List, 20, tc.path)

		last := fmt.Sprintf("%s?page=%d&pageSize=30", tc.path, logCountCap/30+1)
		h.decode(h.call(http.MethodGet, last, "", bob, tc.perm), &page)
		assert.Len(t, page.List, logCountCap%30, "the last page stops at the cap: %s", tc.path)

		beyond := h.call(http.MethodGet, fmt.Sprintf("%s?page=%d&pageSize=20", tc.path, logCountCap/20+1), "", bob, tc.perm)
		assert.Equal(t, http.StatusBadRequest, beyond.Status, tc.path)
		assert.Contains(t, beyond.Body, "只能查看前 1 万条", tc.path)

		var narrow httpx.Page[json.RawMessage]
		h.decode(h.call(http.MethodGet, tc.path+"?pageSize=20&success=false", "", bob, tc.perm), &narrow)
		assert.EqualValues(t, 0, narrow.Total, tc.path)
		assert.False(t, narrow.TotalCapped, "a narrow filter is counted exactly: %s", tc.path)
	}

	var page httpx.Page[opLogView]
	h.decode(h.call(http.MethodGet, "/api/v1/audit/operation-logs?pageSize=5&user=bo", "", bob, PermOperationLogView), &page)
	require.Len(t, page.List, 5)
	assert.Equal(t, "bob", page.List[0].Username, "the user is joined onto the page")
	assert.Equal(t, "鲍勃", page.List[0].Nickname)
}
