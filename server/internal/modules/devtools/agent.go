package devtools

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/devtools/codegen"
	"base-admin/server/internal/devtools/openapi"
	"base-admin/server/internal/modules/auth"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/dbmigrate"
	"base-admin/server/internal/platform/httpx"
)

// The AI agent API: the code generator's endpoints plus table creation,
// for AI coding agents. Requests carry `Authorization: Bearer <key>` (no
// session cookie, so the CSRF check does not apply) and act as the key's
// creator, who must still be enabled and allowed to use the generator. Like
// the generator it only works in the development environment.

// agentPath is the route prefix of the agent API under the API prefix.
const agentPath = "/devtools/agent"

//go:embed agent_guide.md
var agentGuide string

var (
	errAgentKeyMissing = httpx.NewError(http.StatusUnauthorized, auth.CodeUnauthenticated,
		"缺少 AI 接口密钥：请求头加 Authorization: Bearer <密钥>（在「开发工具 → AI 接口」创建）")
	errAgentKeyInvalid = httpx.NewError(http.StatusUnauthorized, auth.CodeUnauthenticated, "AI 接口密钥无效、已过期或已撤销")
	errAgentUser       = httpx.NewError(http.StatusForbidden, auth.CodeForbidden,
		"密钥所属用户已停用，或没有「使用代码生成」权限")
)

// keyTouchInterval limits how often last_used_at is written.
const keyTouchInterval = time.Minute

// agentAuth authenticates a request by its API key and runs it as the
// key's creator.
func (m *Module) agentAuth(c *gin.Context) {
	if err := m.requireDev(); err != nil {
		httpx.Fail(c, err)
		return
	}
	token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	token = strings.TrimSpace(token)
	if !ok || !strings.HasPrefix(token, agentKeyPrefix) {
		httpx.Fail(c, errAgentKeyMissing)
		return
	}
	ctx := c.Request.Context()
	var keys []struct {
		ID         int64
		UserID     int64
		LastUsedAt *time.Time
	}
	err := m.deps.DB.WithContext(ctx).Raw(`SELECT id, user_id, last_used_at FROM sys_agent_keys
		WHERE key_hash = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())`,
		hashAgentKey(token)).Scan(&keys).Error
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if len(keys) == 0 {
		httpx.Fail(c, errAgentKeyInvalid)
		return
	}
	key := keys[0]
	snap, err := auth.Build(ctx, m.deps.DB, key.UserID)
	switch {
	case errors.Is(err, auth.ErrUserNotFound):
		httpx.Fail(c, errAgentKeyInvalid)
		return
	case err != nil:
		httpx.Fail(c, err)
		return
	case snap.Status != actor.StatusEnabled || !snap.HasPermission(PermCodegen):
		httpx.Fail(c, errAgentUser)
		return
	}
	if key.LastUsedAt == nil || time.Since(*key.LastUsedAt) > keyTouchInterval {
		if err := m.deps.DB.WithContext(ctx).Exec(`UPDATE sys_agent_keys SET last_used_at = now() WHERE id = ?`, key.ID).Error; err != nil {
			m.deps.Logger.Warn("agent key touch failed", zap.Error(err))
		}
	}
	a := &actor.Actor{Snapshot: snap, IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}
	c.Request = c.Request.WithContext(actor.With(ctx, a))
	c.Next()
}

// agentGuideView serves the usage guide for agents as Markdown.
func (m *Module) agentGuideView(c *gin.Context) {
	base := m.deps.APIPrefix + agentPath
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.String(http.StatusOK, strings.ReplaceAll(agentGuide, "{{BASE}}", base))
}

type createTableRequest struct {
	codegen.TableSpec
	// DryRun returns the migration without writing or applying it.
	DryRun bool `json:"dryRun,omitempty" doc:"只返回迁移 SQL，不写文件也不执行"`
}

type createTableView struct {
	// Migration is the file stem in server/migrations/app.
	Migration string   `json:"migration"`
	Up        string   `json:"up"`
	Down      string   `json:"down"`
	Applied   bool     `json:"applied" doc:"已写入迁移文件并在开发库执行"`
	Notes     []string `json:"notes"`
}

// createTable turns a table description into a migration following
// docs/table-guide.md, writes it to the application track
// (server/migrations/app) and applies it. The development database must be
// at the last application migration on disk, so only the
// new migration runs. Repeating a request that already succeeded (a retry
// after a lost response) returns the same result.
func (m *Module) createTable(c *gin.Context) (any, error) {
	if err := m.requireDev(); err != nil {
		return nil, err
	}
	var req createTableRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if problems := req.Problems(); len(problems) > 0 {
		return nil, problemsError(problems)
	}
	root, err := m.root()
	if err != nil {
		return nil, httpx.NewError(http.StatusConflict, CodeCodegenNoRepo, "找不到代码仓库，无法写入迁移文件")
	}
	dir := filepath.Join(root, filepath.FromSlash(appMigrationsDir))
	up, down := req.MigrationSQL()
	ctx := c.Request.Context()

	m.migrateMu.Lock()
	defer m.migrateMu.Unlock()
	if view, ok, err := m.sameTable(ctx, dir, req.Name, up, down); err != nil || ok {
		return view, err
	}
	if problems, err := m.tableProblems(ctx, req.TableSpec); err != nil || len(problems) > 0 {
		if err != nil {
			return nil, err
		}
		return nil, problemsError(problems)
	}
	if err := m.trySQL(ctx, up); err != nil {
		return nil, err
	}
	last, err := lastMigration(dir)
	if err != nil {
		return nil, err
	}
	view := createTableView{Migration: codegen.MigrationName(last, req.Name), Up: up, Down: down, Notes: []string{}}
	if req.DryRun {
		view.Notes = append(view.Notes, "dryRun：未写入文件、未执行")
		return view, nil
	}
	if err := m.requireMigrated(ctx, last); err != nil {
		return nil, err
	}
	if err := writeMigration(dir, view.Migration, up, down); err != nil {
		return nil, err
	}
	if err := m.applyMigrations(ctx, dir, last+1); err != nil {
		return nil, httpx.NewError(http.StatusInternalServerError, httpx.CodeInternal,
			"迁移文件已写入但执行失败，请人工检查 "+appMigrationsDir+"/"+view.Migration+"："+err.Error()).WithCause(err)
	}
	view.Applied = true
	view.Notes = append(view.Notes,
		"已写入 "+appMigrationsDir+"/"+view.Migration+".up.sql / .down.sql 并在开发库执行",
		"下一步：POST "+m.deps.APIPrefix+agentPath+"/generate 生成模块")
	return view, nil
}

// sameTable recognizes a repeated request: the table exists and was
// created by an applied migration with exactly this SQL.
func (m *Module) sameTable(ctx context.Context, dir, table, up, down string) (createTableView, bool, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "[0-9]*_create_"+table+".up.sql"))
	if err != nil || len(matches) != 1 {
		return createTableView{}, false, err
	}
	stem := strings.TrimSuffix(filepath.Base(matches[0]), ".up.sql")
	n, _ := codegen.ParseMigrationNumber(stem)
	gotUp, errUp := os.ReadFile(matches[0])                               //nolint:gosec // G304: migration in the repository
	gotDown, errDown := os.ReadFile(filepath.Join(dir, stem+".down.sql")) //nolint:gosec // G304: migration in the repository
	if errUp != nil || errDown != nil || string(gotUp) != up || string(gotDown) != down {
		return createTableView{}, false, nil
	}
	version, dirty, err := m.schemaVersion(ctx)
	if err != nil || dirty || version < n {
		return createTableView{}, false, err
	}
	if _, err := codegen.Introspect(ctx, m.deps.DB, table); err != nil {
		if errors.Is(err, codegen.ErrTableNotFound) {
			err = nil
		}
		return createTableView{}, false, err
	}
	return createTableView{Migration: stem, Up: up, Down: down, Applied: true, Notes: []string{
		"该表已按相同定义创建过（重复请求），未做改动",
		"下一步：POST " + m.deps.APIPrefix + agentPath + "/generate 生成模块",
	}}, true, nil
}

// schemaVersion reads the application track's migration state.
func (m *Module) schemaVersion(ctx context.Context) (version uint, dirty bool, err error) {
	sqlDB, err := m.deps.DB.DB()
	if err != nil {
		return 0, false, err
	}
	v, dirty, err := dbmigrate.AppTrack.State(ctx, sqlDB)
	return uint(v), dirty, err
}

// requireMigrated refuses to add a migration while the database is not at
// the last one on disk: applying the new one would also run the others.
func (m *Module) requireMigrated(ctx context.Context, last uint) error {
	version, dirty, err := m.schemaVersion(ctx)
	if err != nil {
		return err
	}
	if dirty || version != last {
		state := fmt.Sprint(version)
		if dirty {
			state += "（dirty，上次迁移执行失败）"
		}
		return httpx.NewError(http.StatusConflict, CodeMigrationState, fmt.Sprintf(
			"开发库的业务迁移版本是 %s，而 %s 最后一个迁移是 %06d：请先处理（执行 make migrate-up，或删除未执行的迁移文件）后再建表",
			state, appMigrationsDir, last))
	}
	return nil
}

// tableProblems checks the spec against the database: the table must be
// new and referenced tables must have a bigint id.
func (m *Module) tableProblems(ctx context.Context, spec codegen.TableSpec) ([]codegen.Problem, error) {
	var out []codegen.Problem
	if _, err := codegen.Introspect(ctx, m.deps.DB, spec.Name); err == nil {
		out = append(out, codegen.Problem{Field: "name", Message: "表 " + spec.Name + " 已存在"})
	} else if !errors.Is(err, codegen.ErrTableNotFound) {
		return nil, err
	}
	for i, col := range spec.Columns {
		if col.Type != codegen.ColRef {
			continue
		}
		ref, err := codegen.Introspect(ctx, m.deps.DB, col.Ref)
		if errors.Is(err, codegen.ErrTableNotFound) {
			out = append(out, codegen.Problem{Field: fmt.Sprintf("columns[%d].ref", i), Message: "被引用的表 " + col.Ref + " 不存在"})
			continue
		}
		if err != nil {
			return nil, err
		}
		if id, ok := ref.Column("id"); !ok || !id.Primary || id.UDT != "int8" {
			out = append(out, codegen.Problem{Field: fmt.Sprintf("columns[%d].ref", i), Message: "被引用的表 " + col.Ref + " 没有 bigint id 主键"})
		}
	}
	return out, nil
}

// appMigrationsDir holds the application track, relative to the repository.
const appMigrationsDir = "server/migrations/app"

// errRollback ends trySQL's transaction.
var errRollback = errors.New("rollback")

// trySQL runs the migration in a transaction that is rolled back, so a
// failure never leaves a half-applied (dirty) migration behind.
func (m *Module) trySQL(ctx context.Context, up string) error {
	err := m.deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(up).Error; err != nil {
			return err
		}
		return errRollback
	})
	if errors.Is(err, errRollback) {
		return nil
	}
	return httpx.NewError(http.StatusUnprocessableEntity, httpx.CodeValidation, "建表 SQL 执行失败："+err.Error()).WithCause(err)
}

func lastMigration(dir string) (uint, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var last uint
	for _, e := range entries {
		if n, ok := codegen.ParseMigrationNumber(e.Name()); ok && strings.HasSuffix(e.Name(), ".up.sql") {
			last = max(last, n)
		}
	}
	return last, nil
}

// writeMigration creates the up and down files; on failure it removes
// what it created.
func writeMigration(dir, stem, up, down string) (err error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	var created []string
	defer func() {
		if err != nil {
			for _, p := range created {
				_ = os.Remove(p)
			}
		}
	}()
	for _, f := range []struct{ suffix, content string }{{".up.sql", up}, {".down.sql", down}} {
		p := filepath.Join(dir, stem+f.suffix)
		file, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // G302/G304: source file in the repository
		if err != nil {
			return err
		}
		created = append(created, p)
		_, werr := file.WriteString(f.content)
		if err := errors.Join(werr, file.Close()); err != nil {
			return err
		}
	}
	return nil
}

// applyMigrations runs the migrations on disk with a dedicated connection
// (the runner closes the pool it is given) and checks that the database
// ends at want.
func (m *Module) applyMigrations(ctx context.Context, dir string, want uint) error {
	cfg := m.deps.Database
	cfg.StatementTimeout = 0
	sqlDB, err := database.OpenSQL(ctx, cfg, "agent-migrate")
	if err != nil {
		return err
	}
	track := dbmigrate.AppTrack
	track.Files = os.DirFS(dir)
	runner, err := dbmigrate.New(sqlDB, track)
	if err != nil {
		_ = sqlDB.Close()
		return err
	}
	defer func() { _ = runner.Close() }()
	if err := runner.Up(); err != nil {
		return err
	}
	st, err := runner.Status()
	if err != nil {
		return err
	}
	if st.Dirty || st.Version != want {
		return fmt.Errorf("schema is at version %d (dirty=%t), want %d", st.Version, st.Dirty, want)
	}
	return nil
}

// agentRoutes registers the agent API. The routes are public to the
// session guard and authenticated by agentAuth instead.
func (m *Module) agentRoutes(r *module.Router) {
	const note = "AI 接口：请求头 Authorization: Bearer <密钥>，仅开发环境可用。"
	g := r.Group(agentPath)
	pub := access.Public()
	g.GET("/guide", pub, m.agentAuth, m.agentGuideView).Doc(module.Doc{
		Summary: "AI 接口：使用说明", Description: note + "返回 Markdown。",
		Response: openapi.Raw{ContentType: "text/markdown", Description: "给 AI 的使用说明"},
	})
	g.GET("/tables", pub, m.agentAuth, handle(m.codegenTables)).Doc(module.Doc{
		Summary: "AI 接口：数据表", Description: note, Response: []tableView{},
	})
	g.GET("/fields", pub, m.agentAuth, handle(m.codegenFields)).Doc(module.Doc{
		Summary: "AI 接口：字段", Description: note, Query: fieldsQuery{}, Response: []codegen.FieldInfo{},
	})
	g.GET("/dicts", pub, m.agentAuth, handle(m.codegenDicts)).Doc(module.Doc{
		Summary: "AI 接口：可绑定的字典", Description: note, Response: []dictOption{},
	})
	g.POST("/tables", pub, m.agentAuth, handle(m.createTable)).Doc(module.Doc{
		Summary: "AI 接口：建表", Description: note + "按建表规范生成迁移文件并在开发库执行；dryRun 只返回 SQL。",
		Body: createTableRequest{}, Response: createTableView{},
	})
	g.POST("/preview", pub, m.agentAuth, handle(m.codegenPreview)).Doc(module.Doc{
		Summary: "AI 接口：预览生成结果", Description: note, Body: previewRequest{}, Response: previewView{},
	})
	g.POST("/generate", pub, m.agentAuth, handle(m.agentGenerate)).Doc(module.Doc{
		Summary: "AI 接口：生成模块", Description: note + "写入后端模块、前端页面、菜单与权限，需重启后端生效。",
		Body: generateRequest{}, Response: generateView{},
	})
}

// agentGenerate is codegenGenerate with the step an agent tends to miss.
func (m *Module) agentGenerate(c *gin.Context) (any, error) {
	out, err := m.codegenGenerate(c)
	if view, ok := out.(generateView); ok && err == nil {
		view.Notes = append(slices.Clone(view.Notes), "重启后端后新接口、菜单和权限才会生效（前端开发服务器会自动刷新）")
		return view, nil
	}
	return out, err
}
