//go:build integration

package file

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/datascope"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/middleware"
	"base-admin/server/internal/platform/storage"
	"base-admin/server/internal/platform/testinfra/dbtest"
	"base-admin/server/internal/platform/testinfra/migrated"
)

const (
	defaultBodyLimit = 2 << 20
	testCeiling      = 4 << 20
	testSigningKey   = "test-signing-key-0123456789abcdef"
)

var allPerms = []string{permFileDelete, permFileUpdate, permFileUpload, permFileView, permStorageView}

// testClock is a settable clock shared by the module and the signer.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// testGuard authenticates every non-public request as the harness user
// with the harness's current permission set.
type testGuard struct{ h *harness }

func (g testGuard) Require(level access.Level) gin.HandlerFunc {
	return func(c *gin.Context) {
		if level.Kind() == access.KindPublic {
			c.Next()
			return
		}
		perms, anonymous := g.h.currentPerms()
		if anonymous {
			httpx.Fail(c, httpx.NewError(http.StatusUnauthorized, 20001, "未登录"))
			return
		}
		snap := &actor.Snapshot{UserID: g.h.userID, Status: actor.StatusEnabled, Permissions: perms,
			DataScope: datascope.Resolved{All: true}}
		if level.Kind() == access.KindPermission && !level.Allows(snap.HasPermission) {
			httpx.Fail(c, httpx.NewError(http.StatusForbidden, 20002, "无权限"))
			return
		}
		c.Request = c.Request.WithContext(actor.With(c.Request.Context(), &actor.Actor{Snapshot: snap}))
		c.Next()
	}
}

type harness struct {
	t      *testing.T
	db     *gorm.DB
	dir    string
	store  storage.Storage
	clock  *testClock
	mod    *Module
	engine *gin.Engine
	userID int64

	mu        sync.Mutex
	perms     []string
	anonymous bool
}

type harnessOpts struct {
	policy  UploadPolicy
	storage func(storage.Storage) storage.Storage
}

func newHarness(t *testing.T, opts ...harnessOpts) *harness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, httpx.RegisterJSONFieldNames())
	ctx := context.Background()
	dbCfg := migrated.Database(t)
	sqlDB, err := database.OpenSQL(ctx, dbCfg, "file-test")
	require.NoError(t, err)
	gdb, err := database.NewGorm(sqlDB, dbCfg, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	dir := t.TempDir()
	local, err := storage.NewLocal(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = local.Close() })

	h := &harness{t: t, db: gdb, dir: dir, store: local, clock: &testClock{now: time.Now()}, perms: slices.Clone(allPerms)}
	var o harnessOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.storage != nil {
		h.store = o.storage(local)
	}
	h.userID = dbtest.InsertID(t, gdb, `INSERT INTO sys_users (username, password_hash, nickname) VALUES ('uploader', 'x', '上传者')`)

	cfg := config.FileConfig{Driver: config.FileDriverLocal, LocalDir: dir, SignedURLTTL: 10 * time.Minute,
		MaxUploadBytes: testCeiling, SigningKey: testSigningKey}
	h.mod = New(Deps{DB: gdb, Storage: h.store, Policy: o.policy, Config: cfg, Logger: zap.NewNop(), Now: h.clock.Now})
	h.engine = h.buildEngine()
	return h
}

func (h *harness) buildEngine() *gin.Engine {
	engine := gin.New()
	engine.ContextWithFallback = true
	reg := module.NewRegistry()
	engine.Use(middleware.SecurityHeaders(), middleware.BodyLimitFunc(func(c *gin.Context) int64 {
		if r, ok := reg.Find(c.Request.Method, c.FullPath()); ok && r.MaxBodyBytes > 0 {
			return r.MaxBodyBytes
		}
		return defaultBodyLimit
	}))
	engine.NoRoute(httpx.NoRoute)
	h.mod.Routes(module.NewRouter(engine.Group("/api/v1"), Name, testGuard{h: h}, reg))
	return engine
}

func (h *harness) currentPerms() ([]string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.perms), h.anonymous
}

func (h *harness) setPerms(perms ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.perms = slices.Sorted(slices.Values(perms))
	h.anonymous = false
}

func (h *harness) setAnonymous() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.anonymous = true
}

type response struct {
	Status int
	Code   int
	Data   json.RawMessage
	Header http.Header
	Body   []byte
}

func (h *harness) serve(req *http.Request) response {
	rec := httptest.NewRecorder()
	h.engine.ServeHTTP(rec, req)
	resp := response{Status: rec.Code, Header: rec.Header(), Body: rec.Body.Bytes()}
	var env struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		resp.Code, resp.Data = env.Code, env.Data
	}
	return resp
}

func (h *harness) do(method, path string, body any) response {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(h.t, err)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	return h.serve(req)
}

// uploadReq builds a multipart upload; fields are written before the file.
func uploadReq(t *testing.T, name string, content []byte, fields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		require.NoError(t, w.WriteField(k, v))
	}
	if name != "" {
		fw, err := w.CreateFormFile("file", name)
		require.NoError(t, err)
		_, err = fw.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func (h *harness) upload(name string, content []byte, fields map[string]string) response {
	h.t.Helper()
	return h.serve(uploadReq(h.t, name, content, fields))
}

func (h *harness) mustUpload(name string, content []byte, fields map[string]string) fileView {
	h.t.Helper()
	resp := h.upload(name, content, fields)
	require.Equal(h.t, http.StatusOK, resp.Status, "upload %s: %s", name, resp.Body)
	var v fileView
	require.NoError(h.t, json.Unmarshal(resp.Data, &v))
	return v
}

func (h *harness) mustCategory(name string) categoryView {
	h.t.Helper()
	resp := h.do(http.MethodPost, "/api/v1/files/categories", gin.H{"name": name, "sort": 1})
	require.Equal(h.t, http.StatusOK, resp.Status, string(resp.Body))
	var v categoryView
	require.NoError(h.t, json.Unmarshal(resp.Data, &v))
	return v
}

// storedFiles lists every regular file under the storage directory.
func (h *harness) storedFiles() []string {
	h.t.Helper()
	var out []string
	require.NoError(h.t, filepath.WalkDir(h.dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			rel, _ := filepath.Rel(h.dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return err
	}))
	return out
}

func (h *harness) liveRows() int64 {
	var n int64
	require.NoError(h.t, h.db.Raw("SELECT count(*) FROM sys_files WHERE deleted_at IS NULL").Scan(&n).Error)
	return n
}

func decode[T any](t *testing.T, r response) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(r.Data, &v), string(r.Body))
	return v
}

// failingStorage fails Put after consuming the reader, or Delete.
type failingStorage struct {
	storage.Storage
	failPut    bool
	failDelete bool
}

func (f *failingStorage) Put(ctx context.Context, key string, r io.Reader, size int64, ct string) error {
	if f.failPut {
		_, _ = io.Copy(io.Discard, r)
		return io.ErrClosedPipe
	}
	return f.Storage.Put(ctx, key, r, size, ct)
}

func (f *failingStorage) Delete(ctx context.Context, key string) error {
	if f.failDelete {
		return io.ErrClosedPipe
	}
	return f.Storage.Delete(ctx, key)
}
