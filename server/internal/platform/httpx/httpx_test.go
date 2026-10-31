package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/requestid"
)

func init() { gin.SetMode(gin.TestMode) }

type envelope struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data"`
	RequestID string          `json:"requestId"`
}

func serve(t *testing.T, method, body string, h gin.HandlerFunc) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	engine := gin.New()
	engine.Handle(method, "/x", func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestid.With(c.Request.Context(), "req_test"))
		h(c)
	})
	engine.NoRoute(NoRoute)
	req := httptest.NewRequest(method, "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	var env envelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	return rec, env
}

func TestOK(t *testing.T) {
	rec, env := serve(t, http.MethodGet, "", func(c *gin.Context) { OK(c, gin.H{"id": "1001"}) })
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, CodeOK, env.Code)
	assert.Equal(t, "success", env.Message)
	assert.JSONEq(t, `{"id":"1001"}`, string(env.Data))
	assert.Equal(t, "req_test", env.RequestID)
}

func TestFailWithAPIError(t *testing.T) {
	rec, env := serve(t, http.MethodGet, "", func(c *gin.Context) { Fail(c, Conflict("version conflict")) })
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, CodeConflict, env.Code)
	assert.Equal(t, "version conflict", env.Message)
	assert.Equal(t, "null", string(env.Data))
}

func TestFailWithWrappedAPIError(t *testing.T) {
	err := errors.Join(errors.New("context"), NotFound("user not found"))
	rec, env := serve(t, http.MethodGet, "", func(c *gin.Context) { Fail(c, err) })
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, CodeNotFound, env.Code)
}

func TestFailHidesInternalErrors(t *testing.T) {
	var recorded []error
	secret := errors.New(`pq: relation "sys_users" password=hunter2`)
	rec, env := serve(t, http.MethodGet, "", func(c *gin.Context) {
		Fail(c, secret)
		for _, e := range c.Errors {
			recorded = append(recorded, e.Err)
		}
	})
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, CodeInternal, env.Code)
	assert.Equal(t, "服务器内部错误", env.Message)
	assert.NotContains(t, rec.Body.String(), "hunter2")
	require.Len(t, recorded, 1, "original error must be kept for logging")
	assert.ErrorIs(t, recorded[0], secret)
}

func TestFailWithNilError(t *testing.T) {
	rec, env := serve(t, http.MethodGet, "", func(c *gin.Context) { Fail(c, nil) })
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, CodeInternal, env.Code)
}

func TestNoRoute(t *testing.T) {
	engine := gin.New()
	engine.NoRoute(NoRoute)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), `"code":10003`)
}

func TestConstructorStatuses(t *testing.T) {
	cases := []struct {
		err    *Error
		status int
		code   int
	}{
		{BadRequest("x"), http.StatusBadRequest, CodeBadRequest},
		{Validation(nil), http.StatusUnprocessableEntity, CodeValidation},
		{NotFound("x"), http.StatusNotFound, CodeNotFound},
		{Conflict("x"), http.StatusConflict, CodeConflict},
		{TooManyRequests(), http.StatusTooManyRequests, CodeTooManyRequests},
		{Unavailable("x"), http.StatusServiceUnavailable, CodeUnavailable},
		{PayloadTooLarge(), http.StatusRequestEntityTooLarge, CodePayloadTooLarge},
		{Internal(nil), http.StatusInternalServerError, CodeInternal},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.status, tc.err.Status, tc.err.Message)
		assert.Equal(t, tc.code, tc.err.Code, tc.err.Message)
	}
}

func TestErrorCopiesAreIndependent(t *testing.T) {
	base := BadRequest("bad")
	withCause := base.WithCause(errors.New("root"))
	withDetails := base.WithDetails("d")
	assert.Nil(t, base.Unwrap())
	assert.Nil(t, base.Details)
	assert.EqualError(t, withCause, "bad: root")
	assert.Equal(t, "d", withDetails.Details)
}

type createUserRequest struct {
	Username string `json:"username" binding:"required,min=3"`
	Email    string `json:"email" binding:"omitempty,email"`
	Role     string `json:"role" binding:"oneof=admin user"`
	Profile  struct {
		Nickname string `json:"nickname" binding:"max=4"`
	} `json:"profile"`
}

func bindHandler(c *gin.Context) {
	var req createUserRequest
	if err := BindJSON(c, &req); err != nil {
		Fail(c, err)
		return
	}
	OK(c, nil)
}

func TestBindJSONValidation(t *testing.T) {
	require.NoError(t, RegisterJSONFieldNames())
	require.NoError(t, RegisterJSONFieldNames(), "must be idempotent")

	body := `{"username":"ab","email":"nope","role":"root","profile":{"nickname":"toolong"}}`
	rec, env := serve(t, http.MethodPost, body, bindHandler)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Equal(t, CodeValidation, env.Code)

	var details ValidationDetails
	require.NoError(t, json.Unmarshal(env.Data, &details))
	got := map[string]string{}
	for _, f := range details.Fields {
		got[f.Field] = f.Message
	}
	assert.Equal(t, map[string]string{
		"username":         "不能少于 3 个字符",
		"email":            "邮箱格式不正确",
		"role":             "取值必须是以下之一：admin user",
		"profile.nickname": "不能多于 4 个字符",
	}, got)
}

func TestBindJSONMalformed(t *testing.T) {
	rec, env := serve(t, http.MethodPost, `{"username":`, bindHandler)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, CodeBadRequest, env.Code)
}

func TestBindJSONTooLarge(t *testing.T) {
	engine := gin.New()
	engine.POST("/x", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8)
		bindHandler(c)
	})
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"username":"abcdefghijk"}`)))
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestRuleMessageFallbacks(t *testing.T) {
	type req struct {
		Code string `json:"code" binding:"len=2"`
		Tag  string `json:"tag" binding:"required"`
		URL  string `json:"url" binding:"url"`
		Skip string `json:"-"`
		Name string `binding:"required"`
	}
	require.NoError(t, RegisterJSONFieldNames())
	engine := gin.New()
	engine.POST("/x", func(c *gin.Context) {
		var r req
		Fail(c, BindJSON(c, &r))
	})
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"code":"abc","url":"x"}`)))
	body := rec.Body.String()
	assert.Contains(t, body, `"field":"code","message":"长度必须为 2"`)
	assert.Contains(t, body, `"field":"tag","message":"不能为空"`)
	assert.Contains(t, body, `"field":"url","message":"地址格式不正确"`)
	assert.Contains(t, body, `"field":"Name"`)
}

func TestBoundMessagesByKind(t *testing.T) {
	type req struct {
		Name  string   `json:"name" binding:"max=2"`
		Tags  []string `json:"tags" binding:"max=1"`
		Count int      `json:"count" binding:"min=5"`
		Other string   `json:"other" binding:"alpha"`
	}
	require.NoError(t, RegisterJSONFieldNames())
	engine := gin.New()
	engine.POST("/x", func(c *gin.Context) {
		var r req
		Fail(c, BindJSON(c, &r))
	})
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x",
		strings.NewReader(`{"name":"abc","tags":["a","b"],"count":1,"other":"1"}`)))
	body := rec.Body.String()
	assert.Contains(t, body, `"field":"name","message":"不能多于 2 个字符"`)
	assert.Contains(t, body, `"field":"tags","message":"不能多于 1 项"`)
	assert.Contains(t, body, `"field":"count","message":"不能小于 5"`)
	assert.Contains(t, body, `"field":"other","message":"不符合 \"alpha\" 规则"`)
}
