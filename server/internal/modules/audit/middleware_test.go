package audit

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/platform/httpx"
)

func init() { gin.SetMode(gin.TestMode) }

// openGuard authenticates every request as user 42 except when the
// request carries X-Anonymous, which yields 401.
type openGuard struct{}

func (openGuard) Require(access.Level) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("X-Anonymous") != "" {
			httpx.Fail(c, httpx.NewError(http.StatusUnauthorized, 20001, "未登录"))
			return
		}
		a := &actor.Actor{Snapshot: &actor.Snapshot{UserID: 42, Status: 1}}
		c.Request = c.Request.WithContext(actor.With(c.Request.Context(), a))
		c.Next()
	}
}

type unitHarness struct {
	engine *gin.Engine
	sink   *fakeSink
	writer *Writer
	echo   string
}

func newUnitHarness(t *testing.T) *unitHarness {
	t.Helper()
	h := &unitHarness{sink: &fakeSink{}}
	m := &Module{logger: zap.NewNop(), now: time.Now}
	m.writer = newWriter(WriterConfig{FlushInterval: time.Hour}, h.sink, zap.NewNop())
	h.writer = m.writer

	h.engine = gin.New()
	reg := module.NewRegistry()
	api := h.engine.Group("/api/v1", m.Middleware(reg))
	r := module.NewRouter(api, "probe", openGuard{}, reg)
	r.POST("/auth/login", access.Public(), func(c *gin.Context) { httpx.OK(c, nil) }).Doc(module.Doc{Summary: "登录"})
	r.POST("/items", access.Authenticated(), func(c *gin.Context) {
		raw, _ := io.ReadAll(c.Request.Body)
		h.echo = string(raw)
		httpx.OK(c, nil)
	}).Doc(module.Doc{Summary: "新增条目"})
	r.PUT("/items/:id", access.Authenticated(), func(c *gin.Context) {
		httpx.Fail(c, httpx.Conflict("数据已被他人修改，请刷新"))
	}).Doc(module.Doc{Summary: "编辑条目"})
	r.GET("/items", access.Authenticated(), func(c *gin.Context) { httpx.OK(c, nil) }).Doc(module.Doc{Summary: "条目列表"})
	r.POST("/items/boom", access.Authenticated(), func(*gin.Context) { panic("boom") }).Doc(module.Doc{Summary: "爆炸"})
	r.GET("/items/export", access.Authenticated(), func(c *gin.Context) { c.String(http.StatusOK, "a,b") }).
		Doc(module.Doc{Summary: "导出条目", Sensitive: true})
	return h
}

func (h *unitHarness) do(t *testing.T, method, path, body string, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "unit-test")
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.engine.ServeHTTP(rec, req)
	return rec
}

// ops stops the writer and returns everything it recorded.
func (h *unitHarness) ops(t *testing.T) []opRow {
	t.Helper()
	runWriter(h.writer)()
	var out []opRow
	for _, b := range h.sink.ops {
		out = append(out, b...)
	}
	return out
}

func TestMiddlewareRecordsWrites(t *testing.T) {
	h := newUnitHarness(t)
	body := `{"name":"n","password":"p"}`
	rec := h.do(t, http.MethodPost, "/api/v1/items", body)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, body, h.echo, "handler reads the original body")

	ops := h.ops(t)
	require.Len(t, ops, 1)
	op := ops[0]
	assert.Equal(t, "probe", op.Module)
	assert.Equal(t, "新增条目", op.Summary)
	assert.Equal(t, "/api/v1/items", op.Route)
	assert.Equal(t, int64(42), *op.UserID)
	assert.True(t, op.Success)
	assert.Equal(t, 0, op.Code)
	assert.Equal(t, "unit-test", op.UserAgent)
	var stored map[string]string
	require.NoError(t, json.Unmarshal([]byte(op.RequestBody), &stored))
	assert.Equal(t, map[string]string{"name": "n", "password": "***"}, stored)
}

func TestMiddlewareRecordsBusinessErrors(t *testing.T) {
	h := newUnitHarness(t)
	h.do(t, http.MethodPut, "/api/v1/items/9", `{}`)
	ops := h.ops(t)
	require.Len(t, ops, 1)
	assert.Equal(t, "/api/v1/items/:id", ops[0].Route)
	assert.Equal(t, "/api/v1/items/9", ops[0].Path)
	assert.Equal(t, http.StatusConflict, ops[0].Status)
	assert.Equal(t, httpx.CodeConflict, ops[0].Code)
	assert.False(t, ops[0].Success)
	assert.Equal(t, "数据已被他人修改，请刷新", ops[0].ErrorMessage)
}

func TestMiddlewareSkipsUnauthenticated(t *testing.T) {
	h := newUnitHarness(t)
	h.do(t, http.MethodPost, "/api/v1/items", `{}`, "X-Anonymous", "1")
	assert.Empty(t, h.ops(t), "requests rejected before authentication are not stored")
}

func TestMiddlewareRecordsPanics(t *testing.T) {
	h := newUnitHarness(t)
	assert.Panics(t, func() { h.do(t, http.MethodPost, "/api/v1/items/boom", `{}`) })
	ops := h.ops(t)
	require.Len(t, ops, 1)
	assert.Equal(t, http.StatusInternalServerError, ops[0].Status)
	assert.False(t, ops[0].Success)
	assert.Equal(t, "爆炸", ops[0].Summary)
}

func TestMiddlewareSelection(t *testing.T) {
	h := newUnitHarness(t)
	h.do(t, http.MethodGet, "/api/v1/items", "")
	h.do(t, http.MethodPost, "/api/v1/auth/login", `{"username":"a","password":"b"}`)
	h.do(t, http.MethodGet, "/api/v1/items/export", "")
	h.do(t, http.MethodDelete, "/api/v1/unknown", "")
	ops := h.ops(t)
	require.Len(t, ops, 1, "only the sensitive GET is recorded")
	assert.Equal(t, "导出条目", ops[0].Summary)
	assert.Equal(t, http.MethodGet, ops[0].Method)
}

func TestShouldRecord(t *testing.T) {
	sensitive := module.Route{Doc: module.Doc{Sensitive: true}}
	cases := []struct {
		method, path string
		route        module.Route
		found        bool
		want         bool
	}{
		{http.MethodGet, "/api/v1/x", module.Route{}, true, false},
		{http.MethodGet, "/api/v1/x", sensitive, true, true},
		{http.MethodHead, "/api/v1/x", module.Route{}, true, false},
		{http.MethodPost, "/api/v1/auth/login", module.Route{}, true, false},
		{http.MethodPost, "/api/v1/auth/logout", module.Route{}, true, true},
		{http.MethodPut, "/api/v1/x", module.Route{}, false, true},
		{http.MethodDelete, "", module.Route{}, false, false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, shouldRecord(tc.method, tc.path, tc.route, tc.found), tc.method+" "+tc.path)
	}
}
