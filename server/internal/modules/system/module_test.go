package system

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/module"
)

type passGuard struct{}

func (passGuard) Require(access.Level) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }

func TestModuleDeclarationsAreValid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	reg := module.NewRegistry()
	m := New(Deps{})
	m.Routes(module.NewRouter(engine.Group("/api/v1"), m.Name(), passGuard{}, reg))

	report := module.Validate([]module.Module{m}, reg, engine.Routes(), "/api/v1")
	require.NoError(t, report.Err())
	for _, w := range report.Warnings {
		assert.Contains(t, w, "public route", "every permission code must be used by a route")
	}

	for _, r := range reg.Routes() {
		assert.NotEmpty(t, r.Doc.Summary, "%s %s needs a Doc summary", r.Method, r.Path)
	}
	public, ok := reg.Find("GET", "/api/v1/settings/public")
	require.True(t, ok)
	assert.Equal(t, access.KindPublic, public.Access.Kind())
	dicts, ok := reg.Find("GET", "/api/v1/dicts/:code/items")
	require.True(t, ok)
	assert.Equal(t, access.KindAuthenticated, dicts.Access.Kind())
	for _, r := range reg.Routes() {
		if r.Path != public.Path && r.Path != dicts.Path {
			assert.Equal(t, access.KindPermission, r.Access.Kind(), r.Path)
		}
	}
}

func TestNonViewPermissionsRequireView(t *testing.T) {
	m := New(Deps{})
	for _, d := range m.Permissions() {
		req := d.EffectiveRequires()
		if d.Code[len(d.Code)-5:] == ":view" {
			assert.Empty(t, req, d.Code)
			continue
		}
		require.Len(t, req, 1, d.Code)
		assert.Regexp(t, `^system:[a-z]+:view$`, req[0], d.Code)
	}
}

func TestValueCache(t *testing.T) {
	now := time.Unix(1000, 0)
	c := newValueCache(15*time.Second, func() time.Time { return now })
	_, ok := c.get("k")
	assert.False(t, ok)

	c.put("k", "v", c.generation())
	v, ok := c.get("k")
	assert.True(t, ok)
	assert.Equal(t, "v", v)

	now = now.Add(14 * time.Second)
	_, ok = c.get("k")
	assert.True(t, ok, "fresh before ttl")
	now = now.Add(time.Second)
	_, ok = c.get("k")
	assert.False(t, ok, "expired at ttl")

	c.put("k", "v2", c.generation())
	c.clear()
	_, ok = c.get("k")
	assert.False(t, ok, "cleared")

	// A value read before a clear (a local write) is not stored after it.
	gen := c.generation()
	c.clear()
	c.put("k", "stale", gen)
	_, ok = c.get("k")
	assert.False(t, ok, "stale refill dropped")
}
