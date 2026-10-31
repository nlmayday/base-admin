package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/requestid"
)

func init() { gin.SetMode(gin.TestMode) }

func do(engine *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestRequestID(t *testing.T) {
	engine := gin.New()
	engine.Use(RequestID())
	var seen string
	engine.GET("/x", func(c *gin.Context) { seen = requestid.From(c.Request.Context()) })

	t.Run("reuses valid incoming id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set(requestid.Header, "client-123")
		rec := do(engine, req)
		assert.Equal(t, "client-123", seen)
		assert.Equal(t, "client-123", rec.Header().Get(requestid.Header))
	})

	t.Run("replaces invalid incoming id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set(requestid.Header, "bad id with spaces")
		rec := do(engine, req)
		assert.True(t, strings.HasPrefix(seen, "req_"))
		assert.Equal(t, seen, rec.Header().Get(requestid.Header))
	})
}

func newObservedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	return zap.New(core), logs
}

func TestAccessLogLevels(t *testing.T) {
	logger, logs := newObservedLogger()
	engine := gin.New()
	engine.Use(RequestID(), AccessLog(logger))
	engine.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/health/live", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/health/ready", func(c *gin.Context) { httpx.Fail(c, httpx.Unavailable("draining")) })
	engine.GET("/bad", func(c *gin.Context) { httpx.Fail(c, httpx.BadRequest("bad")) })
	engine.GET("/missing", func(c *gin.Context) { httpx.Fail(c, httpx.NotFound("missing")) })
	engine.GET("/limited", func(c *gin.Context) { httpx.Fail(c, httpx.TooManyRequests()) })
	engine.GET("/boom", func(c *gin.Context) { httpx.Fail(c, assert.AnError) })

	cases := map[string]zapcore.Level{
		"/ok":           zapcore.InfoLevel,
		"/health/live":  zapcore.DebugLevel,
		"/health/ready": zapcore.WarnLevel,
		"/bad":          zapcore.InfoLevel,
		"/missing":      zapcore.InfoLevel,
		"/limited":      zapcore.WarnLevel,
		"/boom":         zapcore.ErrorLevel,
	}
	for path, level := range cases {
		do(engine, httptest.NewRequest(http.MethodGet, path+"?token=secret", nil))
		entries := logs.TakeAll()
		require.Len(t, entries, 1, path)
		assert.Equal(t, level, entries[0].Level, path)
		ctx := entries[0].ContextMap()
		assert.Equal(t, path, ctx["path"])
		assert.NotEmpty(t, ctx["request_id"])
		assert.NotContains(t, fmt.Sprint(ctx), "secret", "the query string must not be logged in any field")
	}
}

func TestAccessLogErrorFields(t *testing.T) {
	const userData = "客户「张三」已在李四的私海"
	logger, logs := newObservedLogger()
	engine := gin.New()
	engine.Use(AccessLog(logger))
	engine.GET("/conflict", func(c *gin.Context) {
		httpx.Fail(c, httpx.Conflict(userData).WithCause(errors.New("duplicate key")))
	})
	engine.GET("/boom", func(c *gin.Context) { httpx.Fail(c, assert.AnError) })

	do(engine, httptest.NewRequest(http.MethodGet, "/conflict", nil))
	entry := logs.TakeAll()[0]
	assert.Equal(t, zapcore.InfoLevel, entry.Level)
	ctx := entry.ContextMap()
	assert.NotContains(t, ctx, "errors")
	assert.NotContains(t, fmt.Sprint(ctx), "张三", "a 4xx message may quote user data")
	assert.Equal(t, []any{httpx.CodeConflict}, ctx["error_codes"])

	do(engine, httptest.NewRequest(http.MethodGet, "/boom", nil))
	entry = logs.TakeAll()[0]
	assert.Equal(t, zapcore.ErrorLevel, entry.Level)
	ctx = entry.ContextMap()
	assert.Contains(t, ctx["errors"], assert.AnError.Error())
	assert.Equal(t, []any{httpx.CodeInternal}, ctx["error_codes"])
}

func TestRecover(t *testing.T) {
	logger, logs := newObservedLogger()
	engine := gin.New()
	engine.Use(RequestID(), Recover(logger))
	engine.GET("/panic", func(*gin.Context) { panic("kaboom") })

	rec := do(engine, httptest.NewRequest(http.MethodGet, "/panic", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), `"code":10000`)
	assert.NotContains(t, rec.Body.String(), "kaboom")
	require.Equal(t, 1, logs.FilterMessage("panic recovered").Len())
}

func TestRecoverRepanicsAbortHandler(t *testing.T) {
	logger, _ := newObservedLogger()
	engine := gin.New()
	engine.Use(Recover(logger))
	engine.GET("/abort", func(*gin.Context) { panic(http.ErrAbortHandler) })
	assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
		do(engine, httptest.NewRequest(http.MethodGet, "/abort", nil))
	})
}

func TestSecurityHeaders(t *testing.T) {
	engine := gin.New()
	engine.Use(SecurityHeaders())
	engine.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	rec := do(engine, httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
	assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}

func TestBodyLimit(t *testing.T) {
	engine := gin.New()
	engine.Use(BodyLimit(8))
	engine.POST("/x", func(c *gin.Context) {
		var body map[string]any
		if err := httpx.BindJSON(c, &body); err != nil {
			httpx.Fail(c, err)
			return
		}
		c.Status(http.StatusOK)
	})

	t.Run("declared length over limit", func(t *testing.T) {
		rec := do(engine, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"a":"0123456789"}`)))
		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	})

	t.Run("chunked body over limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"a":"0123456789"}`))
		req.ContentLength = -1
		rec := do(engine, req)
		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	})

	t.Run("within limit", func(t *testing.T) {
		rec := do(engine, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`)))
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestTimeout(t *testing.T) {
	engine := gin.New()
	engine.Use(Timeout(50 * time.Millisecond))
	var remaining time.Duration
	engine.GET("/x", func(c *gin.Context) {
		deadline, ok := c.Request.Context().Deadline()
		require.True(t, ok)
		remaining = time.Until(deadline)
	})
	do(engine, httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.Greater(t, remaining, time.Duration(0))
	assert.LessOrEqual(t, remaining, 50*time.Millisecond)
}

func TestBodyLimitFuncSkipsDeferredRoutes(t *testing.T) {
	engine := gin.New()
	engine.Use(BodyLimitFunc(func(c *gin.Context) int64 {
		if c.FullPath() == "/upload" {
			return 0
		}
		return 8
	}))
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	engine.POST("/upload", ok)
	engine.POST("/x", ok)
	big := strings.Repeat("a", 64)
	assert.Equal(t, http.StatusOK, do(engine, httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(big))).Code)
	assert.Equal(t, http.StatusRequestEntityTooLarge, do(engine, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(big))).Code)
}

func TestTimeoutFuncOverride(t *testing.T) {
	engine := gin.New()
	engine.Use(TimeoutFunc(50*time.Millisecond, func(c *gin.Context) time.Duration {
		if c.FullPath() == "/long" {
			return time.Minute
		}
		return 0
	}))
	remaining := map[string]time.Duration{}
	record := func(c *gin.Context) {
		deadline, ok := c.Request.Context().Deadline()
		require.True(t, ok)
		remaining[c.FullPath()] = time.Until(deadline)
	}
	engine.GET("/long", record)
	engine.GET("/short", record)
	do(engine, httptest.NewRequest(http.MethodGet, "/long", nil))
	do(engine, httptest.NewRequest(http.MethodGet, "/short", nil))
	assert.Greater(t, remaining["/long"], 50*time.Second)
	assert.LessOrEqual(t, remaining["/short"], 50*time.Millisecond)
}
