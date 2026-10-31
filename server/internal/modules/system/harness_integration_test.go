//go:build integration

package system

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/datascope"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/testinfra"
	"base-admin/server/internal/platform/testinfra/migrated"
)

type harness struct {
	t      *testing.T
	db     *gorm.DB
	rdb    *redis.Client
	mod    *Module
	engine *gin.Engine
	userID int64
	// perms is the permission set of the simulated caller; anonymous makes
	// every non-public route answer 401.
	perms     []string
	anonymous bool
}

// testGuard enforces access levels with a synthetic actor, standing in for
// the auth module's guard.
type testGuard struct{ h *harness }

func (g testGuard) Require(level access.Level) gin.HandlerFunc {
	if level.Kind() == access.KindPublic {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		if g.h.anonymous {
			httpx.Fail(c, httpx.NewError(http.StatusUnauthorized, 20001, "未登录"))
			return
		}
		perms := slices.Clone(g.h.perms)
		slices.Sort(perms)
		snap := &actor.Snapshot{UserID: g.h.userID, Status: actor.StatusEnabled, Permissions: perms,
			DataScope: datascope.Resolved{All: true}}
		c.Request = c.Request.WithContext(actor.With(c.Request.Context(), &actor.Actor{Snapshot: snap}))
		if level.Kind() == access.KindPermission && !level.Allows(snap.HasPermission) {
			httpx.Fail(c, httpx.NewError(http.StatusForbidden, 20002, "无权限"))
			return
		}
		c.Next()
	}
}

func allCodes() []string {
	codes := make([]string, len(permissionDefs))
	for i, d := range permissionDefs {
		codes[i] = d.Code
	}
	return codes
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, httpx.RegisterJSONFieldNames())
	ctx := context.Background()
	dbCfg := migrated.Database(t)
	sqlDB, err := database.OpenSQL(ctx, dbCfg, "system-test")
	require.NoError(t, err)
	gdb, err := database.NewGorm(sqlDB, dbCfg, zap.NewNop())
	require.NoError(t, err)
	redisCfg := testinfra.RedisDB(t)
	rdb := redis.NewClient(&redis.Options{Addr: redisCfg.Addr, DB: redisCfg.DB})
	t.Cleanup(func() { _ = rdb.Close(); _ = sqlDB.Close() })

	h := &harness{t: t, db: gdb, rdb: rdb, perms: allCodes()}
	require.NoError(t, gdb.Raw(`INSERT INTO sys_users (username, password_hash, nickname)
		VALUES ('sysadmin', 'x', '配置管理员') RETURNING id`).Scan(&h.userID).Error)
	h.mod = New(Deps{DB: gdb, Redis: rdb, Logger: zap.NewNop()})
	require.NoError(t, module.Sync(ctx, gdb, []module.Module{h.mod}))

	h.engine = gin.New()
	h.engine.NoRoute(httpx.NoRoute)
	h.mod.Routes(module.NewRouter(h.engine.Group("/api/v1"), Name, testGuard{h}, module.NewRegistry()))
	return h
}

type response struct {
	Status int
	Code   int
	Data   json.RawMessage
}

func (h *harness) do(method, path string, body any) response {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(h.t, err)
		reader = strings.NewReader(string(raw))
	}
	req := httptest.NewRequestWithContext(context.Background(), method, "/api/v1"+path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.engine.ServeHTTP(rec, req)
	var env struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	require.NoError(h.t, json.Unmarshal(rec.Body.Bytes(), &env), rec.Body.String())
	return response{Status: rec.Code, Code: env.Code, Data: env.Data}
}

// ok asserts a 200 response and decodes its data into out (if not nil).
func (h *harness) ok(resp response, out any) {
	h.t.Helper()
	require.Equal(h.t, http.StatusOK, resp.Status, "code %d data %s", resp.Code, resp.Data)
	if out != nil {
		require.NoError(h.t, json.Unmarshal(resp.Data, out))
	}
}

// fail asserts an error response with the given HTTP status and code.
func (h *harness) fail(resp response, status, code int) {
	h.t.Helper()
	require.Equal(h.t, status, resp.Status, "data %s", resp.Data)
	require.Equal(h.t, code, resp.Code, "data %s", resp.Data)
}

// fields returns the field names of a 422 response.
func (h *harness) fields(resp response) []string {
	h.t.Helper()
	h.fail(resp, http.StatusUnprocessableEntity, httpx.CodeValidation)
	var d httpx.ValidationDetails
	require.NoError(h.t, json.Unmarshal(resp.Data, &d))
	out := make([]string, len(d.Fields))
	for i, f := range d.Fields {
		out[i] = f.Field
	}
	return out
}

func (h *harness) count(query string, args ...any) int64 {
	h.t.Helper()
	var n int64
	require.NoError(h.t, h.db.Raw(query, args...).Scan(&n).Error)
	return n
}

func (h *harness) cached(code string) bool {
	h.t.Helper()
	n, err := h.rdb.Exists(context.Background(), dictCacheKey(code)).Result()
	require.NoError(h.t, err)
	return n == 1
}
