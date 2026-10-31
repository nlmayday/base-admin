package module

import (
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/platform/httpx"
)

// Router wraps a Gin route group. Every method takes an access level, so a
// route cannot be registered without declaring one; the level is recorded
// in the registry and enforced by the Guard before the handlers run.
type Router struct {
	group  *gin.RouterGroup
	module string
	guard  Guard
	reg    *Registry
}

// NewRouter returns a router for one module.
func NewRouter(group *gin.RouterGroup, module string, guard Guard, reg *Registry) *Router {
	return &Router{group: group, module: module, guard: guard, reg: reg}
}

// Group returns a router for a sub-path.
func (r *Router) Group(relativePath string) *Router {
	return &Router{group: r.group.Group(relativePath), module: r.module, guard: r.guard, reg: r.reg}
}

// GET registers a GET route.
func (r *Router) GET(p string, level access.Level, h ...gin.HandlerFunc) *RouteBuilder {
	return r.handle(http.MethodGet, p, level, h)
}

// POST registers a POST route.
func (r *Router) POST(p string, level access.Level, h ...gin.HandlerFunc) *RouteBuilder {
	return r.handle(http.MethodPost, p, level, h)
}

// PUT registers a PUT route.
func (r *Router) PUT(p string, level access.Level, h ...gin.HandlerFunc) *RouteBuilder {
	return r.handle(http.MethodPut, p, level, h)
}

// PATCH registers a PATCH route.
func (r *Router) PATCH(p string, level access.Level, h ...gin.HandlerFunc) *RouteBuilder {
	return r.handle(http.MethodPatch, p, level, h)
}

// DELETE registers a DELETE route.
func (r *Router) DELETE(p string, level access.Level, h ...gin.HandlerFunc) *RouteBuilder {
	return r.handle(http.MethodDelete, p, level, h)
}

func (r *Router) handle(method, relativePath string, level access.Level, h []gin.HandlerFunc) *RouteBuilder {
	i := r.reg.add(Route{
		Method: method,
		Path:   joinPaths(r.group.BasePath(), relativePath),
		Module: r.module,
		Access: level,
	})
	handlers := make([]gin.HandlerFunc, 0, len(h)+2)
	handlers = append(handlers, r.guard.Require(level), r.overrides(i))
	handlers = append(handlers, h...)
	r.group.Handle(method, relativePath, handlers...)
	return &RouteBuilder{reg: r.reg, index: i}
}

// overrides applies the route's MaxBody and Timeout. It runs after the
// guard, so an unauthenticated client never gets the larger body limit or
// the longer connection deadlines (the global middleware leaves such routes'
// bodies unlimited until here). Public routes cannot declare MaxBody, and
// their handlers extend the deadlines themselves once the request is
// authorized in their own way (e.g. a signed link).
func (r *Router) overrides(i int) gin.HandlerFunc {
	return func(c *gin.Context) {
		rt := r.reg.at(i)
		if rt.MaxBodyBytes > 0 {
			if c.Request.ContentLength > rt.MaxBodyBytes {
				httpx.Fail(c, httpx.PayloadTooLarge())
				return
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, rt.MaxBodyBytes)
		}
		if rt.Timeout > 0 && rt.Access.Kind() != access.KindPublic {
			httpx.ExtendDeadlines(c.Writer, rt.Timeout)
		}
		c.Next()
	}
}

// RouteBuilder adds optional metadata to a registered route.
type RouteBuilder struct {
	reg   *Registry
	index int
}

// Doc attaches documentation used by the API docs and operation log.
func (b *RouteBuilder) Doc(d Doc) *RouteBuilder {
	b.reg.update(b.index, func(r *Route) { r.Doc = d })
	return b
}

// MaxBody raises the request body limit of this route, e.g. for uploads.
func (b *RouteBuilder) MaxBody(n int64) *RouteBuilder {
	b.reg.update(b.index, func(r *Route) { r.MaxBodyBytes = n })
	return b
}

// Timeout raises the request timeout of this route, e.g. for uploads and
// downloads of large files.
func (b *RouteBuilder) Timeout(d time.Duration) *RouteBuilder {
	b.reg.update(b.index, func(r *Route) { r.Timeout = d })
	return b
}

// joinPaths mirrors how Gin computes a route's full path.
func joinPaths(base, relative string) string {
	if relative == "" {
		return base
	}
	joined := path.Join(base, relative)
	if strings.HasSuffix(relative, "/") && !strings.HasSuffix(joined, "/") {
		return joined + "/"
	}
	return joined
}
