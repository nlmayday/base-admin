//go:build integration

package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/modules/auth"
	"base-admin/server/internal/platform/httpx"
)

func TestLoginLogs(t *testing.T) {
	h := newHarness(t)
	frank := h.createUser("frank", "")
	w := h.mod.Writer()
	ctx := context.Background()
	old := time.Now().Add(-72 * time.Hour)
	w.RecordLogin(ctx, auth.LoginEvent{Kind: auth.EventLogin, Username: "frank", UserID: frank, Success: true,
		IP: "192.168.1.10", UserAgent: "Mozilla/5.0", RequestID: "req_a"})
	w.RecordLogin(ctx, auth.LoginEvent{Kind: auth.EventLogin, Username: "frank", Success: false,
		Reason: auth.ReasonBadCredentials, IP: "192.168.1.11", RequestID: "req_b"})
	w.RecordLogin(ctx, auth.LoginEvent{Kind: auth.EventLogin, Username: "=ghost", Success: false,
		Reason: auth.ReasonRateLimited, IP: "10.0.0.9", At: old})
	w.RecordLogin(ctx, auth.LoginEvent{Kind: auth.EventLogout, Username: "frank", UserID: frank, Success: true,
		IP: "192.168.1.10"})
	h.waitCount(tableLoginLogs, "TRUE", 4)

	var page httpx.Page[loginLogView]
	h.decode(h.call(http.MethodGet, "/api/v1/audit/login-logs", "", frank, PermLoginLogView), &page)
	require.EqualValues(t, 4, page.Total)
	first := page.List[len(page.List)-1]
	assert.Equal(t, "=ghost", first.Username, "oldest last")
	assert.Nil(t, first.UserID)
	assert.Equal(t, auth.ReasonRateLimited, first.Reason)

	cases := []struct {
		query string
		want  int
	}{
		{"username=FRA", 3},
		{"success=false", 2},
		{"success=true&kind=login", 1},
		{"kind=logout", 1},
		{"ip=192.168.1", 3},
		{"ip=10.0", 1},
		{"ip=" + url.QueryEscape("_"), 0},
		{"startTime=" + url.QueryEscape(time.Now().Add(-time.Hour).Format(time.RFC3339)), 3},
		{"endTime=" + url.QueryEscape(time.Now().Add(-48*time.Hour).Format(time.RFC3339)), 1},
	}
	for _, tc := range cases {
		var p httpx.Page[loginLogView]
		h.decode(h.call(http.MethodGet, "/api/v1/audit/login-logs?"+tc.query, "", frank, PermLoginLogView), &p)
		assert.EqualValues(t, tc.want, p.Total, tc.query)
	}
	assert.Equal(t, http.StatusUnprocessableEntity,
		h.call(http.MethodGet, "/api/v1/audit/login-logs?success=maybe", "", frank, PermLoginLogView).Status)
	assert.Equal(t, http.StatusBadRequest,
		h.call(http.MethodGet, "/api/v1/audit/login-logs?kind=other", "", frank, PermLoginLogView).Status)

	r := h.call(http.MethodGet, "/api/v1/audit/login-logs/export?success=false", "", frank, PermLoginLogExport)
	require.Equal(t, http.StatusOK, r.Status, r.Body)
	lines := strings.Split(strings.TrimSpace(strings.TrimPrefix(r.Body, "\xef\xbb\xbf")), "\n")
	require.Len(t, lines, 3, r.Body)
	assert.Contains(t, r.Body, "用户名或密码错误")
	assert.Contains(t, r.Body, "请求过于频繁")
	assert.Contains(t, r.Body, "'=ghost")
	h.waitCount(tableOperationLogs, "summary = '导出登录日志'", 1)
}

func TestStats(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("gina", "")
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	h.insertLogin("login", "a", true, today.Add(time.Second))
	h.insertLogin("login", "a", true, today.Add(2*time.Second))
	h.insertLogin("login", "a", false, today.Add(3*time.Second))
	h.insertLogin("logout", "a", true, today.Add(4*time.Second))
	h.insertLogin("login", "a", false, today.AddDate(0, 0, -2).Add(time.Hour))
	h.insertLogin("login", "a", true, today.AddDate(0, 0, -10))
	h.insertOp(&uid, "iam", "POST", "/x", "x", true, today.Add(time.Second))
	h.insertOp(&uid, "iam", "POST", "/x", "x", false, today.Add(time.Second))
	h.insertOp(&uid, "iam", "POST", "/x", "x", true, today.AddDate(0, 0, -6))
	h.insertOp(&uid, "iam", "POST", "/x", "x", true, today.AddDate(0, 0, -7))

	var s statsView
	h.decode(h.call(http.MethodGet, "/api/v1/audit/stats", "", uid, PermLoginLogView), &s)
	assert.Equal(t, dayStats{LoginSuccess: 2, LoginFailure: 1, Operations: 2, OperationFailures: 1}, s.Today)
	require.Len(t, s.Trend, 7)
	assert.Equal(t, today.Format(time.DateOnly), s.Trend[6].Date)
	assert.Equal(t, today.AddDate(0, 0, -6).Format(time.DateOnly), s.Trend[0].Date)
	assert.EqualValues(t, 1, s.Trend[4].LoginFailure, "two days ago")
	assert.EqualValues(t, 1, s.Trend[0].Operations, "six days ago is the first bucket")
	var total int64
	for _, p := range s.Trend {
		total += p.LoginSuccess + p.LoginFailure
	}
	assert.EqualValues(t, 4, total, "logouts and rows older than 7 days are excluded")
}

func TestCleanupJob(t *testing.T) {
	h := newHarness(t)
	now := time.Now()
	for range 3 {
		h.insertLogin("login", "old", true, now.AddDate(0, 0, -200))
		h.insertOp(nil, "iam", "POST", "/old", "old", true, now.AddDate(0, 0, -200))
	}
	h.insertLogin("login", "mid", true, now.AddDate(0, 0, -40))
	h.insertOp(nil, "iam", "POST", "/mid", "mid", true, now.AddDate(0, 0, -40))
	h.insertLogin("login", "new", true, now.Add(-time.Hour))
	h.insertOp(nil, "iam", "POST", "/new", "new", true, now.Add(-time.Hour))

	job := h.mod.Jobs()[0]
	require.NoError(t, job.Run(context.Background(), nil), "default 180 days")
	assert.EqualValues(t, 2, h.count(tableLoginLogs, "TRUE"))
	assert.EqualValues(t, 2, h.count(tableOperationLogs, "TRUE"))

	require.NoError(t, job.Run(context.Background(), json.RawMessage(`{"days":30}`)))
	assert.EqualValues(t, 1, h.count(tableLoginLogs, "username = 'new'"))
	assert.EqualValues(t, 1, h.count(tableLoginLogs, "TRUE"))
	assert.EqualValues(t, 1, h.count(tableOperationLogs, "TRUE"))

	assert.Error(t, job.Run(context.Background(), json.RawMessage(`{"days":0}`)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Error(t, job.Run(ctx, nil), "honours cancellation")
}

func TestWriterFlushesToDatabaseOnShutdown(t *testing.T) {
	db := openDB(t)
	mod := New(Deps{DB: db, Writer: WriterConfig{FlushInterval: time.Hour, BatchSize: 10000}})
	stop := runWriter(mod.Writer())
	for range 25 {
		mod.Writer().RecordLogin(context.Background(), auth.LoginEvent{Kind: auth.EventLogin, Username: "z", Success: true})
	}
	var n int64
	require.NoError(t, db.Raw("SELECT count(*) FROM sys_login_logs").Scan(&n).Error)
	assert.Zero(t, n, "nothing is written before a flush")
	stop()
	require.NoError(t, db.Raw("SELECT count(*) FROM sys_login_logs").Scan(&n).Error)
	assert.EqualValues(t, 25, n, "shutdown flushes the queue")
	assert.Zero(t, mod.Writer().Dropped())
}
