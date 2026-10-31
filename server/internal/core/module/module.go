// Package module is the compile-time module registry (design doc section 4):
// the Module interface, an access-enforcing router, startup validation and
// the synchronization of permissions, menus and seed data.
package module

import (
	"context"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/perm"
)

// Module is a unit of business functionality. The core never depends on a
// concrete module; modules are listed explicitly in app wiring.
type Module interface {
	// Name is the module identifier and the permission code prefix.
	Name() string
	Permissions() []perm.Def
	Menus() []menu.Def
	Routes(r *Router)
	// Seed writes idempotent initial data inside the startup transaction.
	Seed(ctx context.Context, tx *gorm.DB) error
}

// Guard turns an access level into middleware that enforces it. The auth
// module provides the real implementation.
type Guard interface {
	Require(level access.Level) gin.HandlerFunc
}

// Route is one registered endpoint.
type Route struct {
	Method string
	Path   string
	Module string
	Access access.Level
	Doc    Doc
	// MaxBodyBytes overrides the global request body limit when > 0
	// (e.g. file uploads).
	MaxBodyBytes int64
	// Timeout overrides the global request timeout when > 0, for long
	// transfers such as uploads and downloads.
	Timeout time.Duration
}

// Doc describes a route for the API documentation and the operation log.
// Query, Body and Response are zero values of the Go types used, e.g.
// userFilter{}, createUserRequest{}, httpx.Page[userView]{}; they are only
// inspected by reflection.
type Doc struct {
	// Summary is a short operation name such as "新增用户"; the operation
	// log shows it as the action.
	Summary     string
	Description string
	Query       any
	Body        any
	Response    any
	// Sensitive marks reads that must be audited, such as exports.
	Sensitive bool
}

// Registry records every route registered through a Router. Routes are
// added during startup and only read afterwards.
type Registry struct {
	mu     sync.RWMutex
	routes []Route
	index  map[string]int
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{index: map[string]int{}} }

// Routes returns a copy of the registered routes.
func (r *Registry) Routes() []Route {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Route(nil), r.routes...)
}

// Find returns the route registered for method and full path (the Gin
// route template, e.g. "/api/v1/users/:id").
func (r *Registry) Find(method, fullPath string) (Route, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	i, ok := r.index[method+" "+fullPath]
	if !ok {
		return Route{}, false
	}
	return r.routes[i], true
}

func (r *Registry) at(i int) Route {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.routes[i]
}

func (r *Registry) add(route Route) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = append(r.routes, route)
	i := len(r.routes) - 1
	r.index[route.Method+" "+route.Path] = i
	return i
}

func (r *Registry) update(i int, fn func(*Route)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(&r.routes[i])
}
