package devtools

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/platform/config"
)

func init() { gin.SetMode(gin.TestMode) }

type openGuard struct{}

func (openGuard) Require(access.Level) gin.HandlerFunc { return func(*gin.Context) {} }

func TestModuleDefinitionsValidate(t *testing.T) {
	reg := module.NewRegistry()
	m := New(Deps{Registry: reg, Env: config.EnvDev})
	engine := gin.New()
	m.Routes(module.NewRouter(engine.Group("/api/v1"), m.Name(), openGuard{}, reg))

	rep := module.Validate([]module.Module{m}, reg, engine.Routes(), "/api/v1")
	require.NoError(t, rep.Err())
	assert.Len(t, reg.Routes(), 18)
	for _, r := range reg.Routes() {
		assert.NotEmpty(t, r.Doc.Summary, r.Path)
		// The agent API authenticates by key itself (agentAuth).
		want := access.KindPermission
		if strings.HasPrefix(r.Path, "/api/v1"+agentPath+"/") {
			want = access.KindPublic
		}
		assert.Equal(t, want, r.Access.Kind(), r.Path)
	}
	assert.Equal(t, "devtools.root", m.Menus()[0].Code)
	assert.Equal(t, 96, m.Menus()[0].Sort)
	codes := []string{}
	for _, p := range m.Permissions() {
		codes = append(codes, p.Code)
	}
	assert.Equal(t, []string{PermAPIDoc, PermCodegen, PermAgentKeys}, codes)
}

func TestRequireDev(t *testing.T) {
	assert.NoError(t, New(Deps{Env: config.EnvDev}).requireDev())
	for _, env := range []config.Env{config.EnvTest, config.EnvProd, ""} {
		err := New(Deps{Env: env}).requireDev()
		require.Error(t, err)
		assert.Equal(t, http.StatusForbidden, errCodegenDisabled.Status)
		assert.Equal(t, CodeCodegenDisabled, errCodegenDisabled.Code)
	}
}

func TestCLICommand(t *testing.T) {
	assert.Equal(t, "cd server && go run ./cmd/gen -table biz_customers -module example -resource customer -name '客户'",
		cliCommand(previewRequest{Table: "biz_customers", Module: "example", Resource: "customer", Name: " 客户 "}))
	assert.Contains(t, cliCommand(previewRequest{Table: "t", Module: "m", Resource: "r", Name: "n", ModuleTitle: "示例"}),
		"-module-title '示例'")
}
