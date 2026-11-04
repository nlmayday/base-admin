//go:build integration

package audit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/datascope"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/requestid"
	"base-admin/server/internal/platform/testinfra/dbtest"
	"base-admin/server/internal/platform/testinfra/migrated"
)

const (
	apiPrefix = "/api/v1"
	// Test guard headers: the caller's user id and comma-separated codes.
	hdrUser  = "X-Test-User"
	hdrPerms = "X-Test-Perms"
)

var allViewPerms = []string{PermLoginLogView, PermOperationLogView, PermLoginLogExport, PermOperationLogExport}

// testGuard authenticates from headers and enforces access levels like the
// real guard: 401 without a user, 403 without a matching permission.
type testGuard struct{}

func (testGuard) Require(level access.Level) gin.HandlerFunc {
	if level.Kind() == access.KindPublic {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		uid, err := strconv.ParseInt(c.GetHeader(hdrUser), 10, 64)
		if err != nil || uid <= 0 {
			httpx.Fail(c, httpx.NewError(http.StatusUnauthorized, 20001, "未登录或会话已过期"))
			return
		}
		var perms []string
		if raw := c.GetHeader(hdrPerms); raw != "" {
			perms = strings.Split(raw, ",")
		}
		slices.Sort(perms)
		snap := &actor.Snapshot{UserID: uid, Status: actor.StatusEnabled, Permissions: perms,
			DataScope: datascope.Resolved{All: true}}
		c.Request = c.Request.WithContext(actor.With(c.Request.Context(), &actor.Actor{Snapshot: snap}))
		if level.Kind() == access.KindPermission && !level.Allows(snap.HasPermission) {
			httpx.Fail(c, httpx.NewError(http.StatusForbidden, 20002, "无权限"))
			return
		}
		c.Next()
	}
}

type harness struct {
	t      *testing.T
	db     *gorm.DB
	mod    *Module
	engine *gin.Engine
	stop   func()
}

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	cfg := migrated.Database(t)
	sqlDB, err := database.OpenSQL(context.Background(), cfg, "audit-test")
	require.NoError(t, err)
	gdb, err := database.NewGorm(sqlDB, cfg, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return gdb
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := openDB(t)
	mod := New(Deps{DB: db, Writer: WriterConfig{FlushInterval: 10 * time.Millisecond}})
	h := &harness{t: t, db: db, mod: mod, stop: runWriter(mod.Writer())}
	t.Cleanup(h.stop)
	h.engine = h.buildEngine()
	return h
}

// buildEngine mounts the audit module, a probe module and a fake login
// route behind the audit middleware, like app wiring does.
func (h *harness) buildEngine() *gin.Engine {
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestid.With(c.Request.Context(), requestid.New()))
		c.Next()
	})
	reg := module.NewRegistry()
	api := engine.Group(apiPrefix, h.mod.Middleware(reg))
	h.mod.Routes(module.NewRouter(api, Name, testGuard{}, reg))

	ok := func(c *gin.Context) { httpx.OK(c, nil) }
	auth := module.NewRouter(api, "auth", testGuard{}, reg)
	auth.POST("/auth/login", access.Public(), ok).Doc(module.Doc{Summary: "登录"})
	auth.POST("/auth/logout", access.Authenticated(), ok).Doc(module.Doc{Summary: "退出登录"})

	probe := module.NewRouter(api, "probe", testGuard{}, reg)
	probe.POST("/probe/items", access.Authenticated(), func(c *gin.Context) {
		var body map[string]any
		if err := httpx.BindJSON(c, &body); err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, body)
	}).Doc(module.Doc{Summary: "新增条目"})
	probe.DELETE("/probe/items/:id", access.Perm("probe:item:delete"), ok).Doc(module.Doc{Summary: "删除条目"})
	probe.GET("/probe/items", access.Authenticated(), ok).Doc(module.Doc{Summary: "条目列表"})
	probe.GET("/probe/export", access.Authenticated(), ok).Doc(module.Doc{Summary: "导出条目", Sensitive: true})
	probe.PUT("/probe/players/:id/ban", access.Authenticated(), func(c *gin.Context) {
		SetTarget(c, "player:"+c.Param("id"))
		if c.Query("fail") != "" {
			httpx.Fail(c, httpx.Conflict("玩家已封禁"))
			return
		}
		httpx.OK(c, nil)
	}).Doc(module.Doc{Summary: "封禁玩家"})
	probe.POST("/probe/players/ban", access.Authenticated(), func(c *gin.Context) {
		SetTarget(c, "player:1", " ", "player:2")
		httpx.OK(c, nil)
	}).Doc(module.Doc{Summary: "批量封禁"})
	return engine
}

type apiResponse struct {
	Status int
	Code   int
	Data   json.RawMessage
	Body   string
	Header http.Header
}

// call sends a request as uid (0 = anonymous) holding perms.
func (h *harness) call(method, path string, body string, uid int64, perms ...string) apiResponse {
	h.t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "audit-test/1.0")
	if uid > 0 {
		req.Header.Set(hdrUser, strconv.FormatInt(uid, 10))
	}
	if len(perms) > 0 {
		req.Header.Set(hdrPerms, strings.Join(perms, ","))
	}
	rec := httptest.NewRecorder()
	h.engine.ServeHTTP(rec, req)
	out := apiResponse{Status: rec.Code, Body: rec.Body.String(), Header: rec.Header()}
	if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		var env struct {
			Code int             `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		require.NoError(h.t, json.Unmarshal(rec.Body.Bytes(), &env))
		out.Code, out.Data = env.Code, env.Data
	}
	return out
}

func (h *harness) decode(r apiResponse, dst any) {
	h.t.Helper()
	require.Equal(h.t, http.StatusOK, r.Status, r.Body)
	require.NoError(h.t, json.Unmarshal(r.Data, dst))
}

func (h *harness) createUser(username, nickname string) int64 {
	h.t.Helper()
	return dbtest.InsertID(h.t, h.db, `INSERT INTO sys_users (username, password_hash, nickname) VALUES (?, 'x', ?)`,
		username, nickname)
}

func (h *harness) count(table, cond string, args ...any) int64 {
	h.t.Helper()
	var n int64
	require.NoError(h.t, h.db.Raw("SELECT count(*) FROM "+table+" WHERE "+cond, args...).Scan(&n).Error)
	return n
}

// waitCount waits for the asynchronous writer to persist n matching rows.
func (h *harness) waitCount(table, cond string, n int64, args ...any) {
	h.t.Helper()
	require.Eventually(h.t, func() bool { return h.count(table, cond, args...) == n },
		3*time.Second, 10*time.Millisecond, "waiting for %d rows in %s where %s", n, table, cond)
}

func (h *harness) insertOp(uid *int64, module, method, path, summary string, success bool, at time.Time) {
	h.t.Helper()
	require.NoError(h.t, h.db.Exec(`INSERT INTO sys_operation_logs
		(user_id, module, method, route, path, summary, status, code, success, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 200, 0, ?, 5, ?)`, uid, module, method, path, path, summary, success, at).Error)
}

func (h *harness) insertLogin(kind, username string, success bool, at time.Time) {
	h.t.Helper()
	require.NoError(h.t, h.db.Exec(`INSERT INTO sys_login_logs (kind, username, success, reason, ip, created_at)
		VALUES (?, ?, ?, '', '10.0.0.1', ?)`, kind, username, success, at).Error)
}
