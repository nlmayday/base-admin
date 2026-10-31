package file

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/platform/config"
)

type nopGuard struct{}

func (nopGuard) Require(access.Level) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }

func TestModulePassesStartupValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	reg := module.NewRegistry()
	m := New(Deps{Config: config.FileConfig{MaxUploadBytes: 10 << 20}})
	m.Routes(module.NewRouter(engine.Group("/api/v1"), Name, nopGuard{}, reg))

	report := module.Validate([]module.Module{m}, reg, engine.Routes(), "/api/v1")
	require.NoError(t, report.Err())
	// The only warning is the startup listing of the one public route; in
	// particular every declared permission is used by a route.
	assert.Equal(t, []string{"public route: GET /api/v1/files/signed/:token"}, report.Warnings)

	upload, ok := reg.Find("POST", "/api/v1/files")
	require.True(t, ok)
	assert.Equal(t, int64(10<<20+multipartOverhead), upload.MaxBodyBytes)
	for _, r := range reg.Routes() {
		assert.NotEmpty(t, r.Doc.Summary, "%s %s needs a Doc summary", r.Method, r.Path)
		if r.Path == "/api/v1/files/signed/:token" {
			assert.Equal(t, access.KindPublic, r.Access.Kind())
		} else {
			assert.NotEqual(t, access.KindPublic, r.Access.Kind(), r.Path)
		}
	}
}
