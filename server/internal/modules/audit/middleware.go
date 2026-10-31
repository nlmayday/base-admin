package audit

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/requestid"
)

// loginRouteSuffix identifies POST /auth/login, which the login log covers.
const loginRouteSuffix = "/auth/login"

// Middleware records operation logs for the API group. Install it on the
// group before the CSRF check so rejected attempts are recorded too:
//
//	api := engine.Group(APIPrefix, auditModule.Middleware(registry), core.Guard.CSRF())
//
// It records every non-GET request plus GET routes documented as Sensitive,
// except POST /auth/login, made by an authenticated user. The actor is read
// after the handlers ran (the guard sets it). Requests rejected before
// authentication (401, CSRF) are not stored: anyone could flood the table
// with them, and login attempts are already in the login log.
//
// A handler panic is recorded as a 500 before the panic continues to the
// Recover middleware further out.
//
// The business code comes from the error that httpx.Fail attached to the
// Gin context (c.Errors); the response body is never parsed. A request
// without such an error has code 0, and success means status < 400 and
// code 0.
func (m *Module) Middleware(reg *module.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		fullPath := c.FullPath()
		route, found := reg.Find(c.Request.Method, fullPath)
		if !shouldRecord(c.Request.Method, fullPath, route, found) {
			c.Next()
			return
		}
		start := m.now()
		body := captureBody(c.Request)
		defer func() {
			if p := recover(); p != nil {
				m.record(c, route, body, start, http.StatusInternalServerError)
				panic(p)
			}
		}()
		c.Next()
		m.record(c, route, body, start, 0)
	}
}

// record enqueues the operation log of an authenticated request. A non-zero
// status overrides the written one (a panic has not written its 500 yet).
func (m *Module) record(c *gin.Context, route module.Route, body string, start time.Time, status int) {
	a := actor.From(c.Request.Context())
	if a == nil || a.Snapshot == nil {
		return
	}
	row := m.buildOpRow(c, route, body, start)
	if status != 0 {
		row.Status, row.Success = status, false
	}
	m.writer.enqueue(entry{op: row})
}

func shouldRecord(method, fullPath string, route module.Route, found bool) bool {
	if fullPath == "" {
		return false
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return found && route.Doc.Sensitive
	case http.MethodPost:
		return !strings.HasSuffix(fullPath, loginRouteSuffix)
	default:
		return true
	}
}

func (m *Module) buildOpRow(c *gin.Context, route module.Route, body string, start time.Time) *opRow {
	ctx := c.Request.Context()
	var uid *int64
	if a := actor.From(ctx); a != nil && a.Snapshot != nil {
		id := a.UserID()
		uid = &id
	}
	status := c.Writer.Status()
	code, errMsg := 0, ""
	if last := c.Errors.Last(); last != nil {
		apiErr := httpx.AsError(last.Err)
		// Only the client-safe message is stored; causes stay in the log.
		code, errMsg = apiErr.Code, apiErr.Message
	}
	return &opRow{
		UserID:       uid,
		Module:       clean(route.Module, 32),
		Method:       clean(c.Request.Method, 8),
		Route:        clean(c.FullPath(), 255),
		Path:         clean(c.Request.URL.Path, 512),
		Summary:      clean(route.Doc.Summary, 128),
		Status:       status,
		Code:         code,
		Success:      status < http.StatusBadRequest && code == 0,
		DurationMs:   int(m.now().Sub(start).Milliseconds()),
		IP:           clean(c.ClientIP(), 64),
		UserAgent:    clean(c.Request.UserAgent(), 512),
		RequestID:    clean(requestid.From(ctx), 64),
		RequestBody:  clean(body, maxBodyBytes),
		ErrorMessage: clean(errMsg, 512),
		CreatedAt:    start,
	}
}
