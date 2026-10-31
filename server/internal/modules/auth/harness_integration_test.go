//go:build integration

package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/password"
	"base-admin/server/internal/platform/testinfra"
	"base-admin/server/internal/platform/testinfra/migrated"
)

const (
	origin   = "http://admin.test"
	testPass = "correct-horse-1"
)

var fastHash = password.Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

type harness struct {
	t      *testing.T
	db     *gorm.DB
	rdb    *redis.Client
	core   *Core
	server *httptest.Server
	cfg    config.AuthConfig
	clock  *fakeClock
}

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func testAuthConfig() config.AuthConfig {
	return config.AuthConfig{
		CookieName: "sid", AllowedOrigins: []string{origin},
		IdleTimeout: time.Hour, AbsoluteTimeout: 2 * time.Hour, TouchInterval: time.Minute,
		SnapshotTTL: time.Minute, ReconcileInterval: time.Second, LockTimeout: 2 * time.Second,
		LoginWindow: time.Minute, LoginMaxPerIP: 1000, LoginMaxPerAccount: 5,
	}
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	dbCfg := migrated.Database(t)
	sqlDB, err := database.OpenSQL(ctx, dbCfg, "auth-test")
	require.NoError(t, err)
	gdb, err := database.NewGorm(sqlDB, dbCfg, zap.NewNop())
	require.NoError(t, err)
	redisCfg := testinfra.RedisDB(t)
	rdb := redis.NewClient(&redis.Options{Addr: redisCfg.Addr, DB: redisCfg.DB})
	t.Cleanup(func() { _ = rdb.Close(); _ = sqlDB.Close() })

	clock := &fakeClock{now: time.Now()}
	cfg := testAuthConfig()
	core, err := NewCore(Options{DB: gdb, Redis: rdb, Config: cfg, APIPrefix: "/api/v1",
		Logger: zap.NewNop(), Password: fastHash, Now: clock.Now})
	require.NoError(t, err)

	// Session keys survive revocation so tests prove that the generation
	// check, not key deletion, ends sessions.
	core.Revoker.hooks.skipCleanup = true
	h := &harness{t: t, db: gdb, rdb: rdb, core: core, cfg: cfg, clock: clock}
	h.restart()
	return h
}

// restart serves a fresh engine built from the current core. Cookies are
// not port-specific, so clients keep their session.
func (h *harness) restart() {
	srv := httptest.NewServer(h.engine())
	h.t.Cleanup(srv.Close)
	h.server = srv
}

// engine mounts the auth module plus a few probe routes.
func (h *harness) engine() *gin.Engine {
	engine := gin.New()
	reg := module.NewRegistry()
	api := engine.Group("/api/v1", h.core.Guard.CSRF())
	me := func(ctx context.Context) (any, error) {
		return gin.H{"userId": actor.From(ctx).UserID()}, nil
	}
	NewModule(h.core, me).Routes(module.NewRouter(api, "auth", h.core.Guard, reg))
	r := module.NewRouter(api, "probe", h.core.Guard, reg)
	ok := func(c *gin.Context) { httpx.OK(c, gin.H{"userId": actor.From(c.Request.Context()).UserID()}) }
	r.GET("/ping", access.Authenticated(), ok)
	r.POST("/ping", access.Authenticated(), ok)
	r.GET("/me", access.Authenticated(), ok)
	r.GET("/secret", access.Perm("probe:secret:view"), ok)
	return engine
}

// createUser inserts an enabled user with testPass and returns its id.
func (h *harness) createUser(username string) int64 {
	h.t.Helper()
	hash, err := password.Hash(testPass, fastHash)
	require.NoError(h.t, err)
	var id int64
	require.NoError(h.t, h.db.Raw(`INSERT INTO sys_users (username, password_hash) VALUES (?, ?) RETURNING id`,
		username, hash).Scan(&id).Error)
	return id
}

func (h *harness) pgGen(uid int64) int64 {
	var gen int64
	require.NoError(h.t, h.db.Raw("SELECT session_gen FROM sys_users WHERE id = ?", uid).Scan(&gen).Error)
	return gen
}

func (h *harness) mirror(uid int64) int64 {
	gen, found, err := h.core.Sessions.GenMirror(context.Background(), uid)
	require.NoError(h.t, err)
	require.True(h.t, found)
	return gen
}

func (h *harness) status(uid int64) int16 {
	var s int16
	require.NoError(h.t, h.db.Raw("SELECT status FROM sys_users WHERE id = ?", uid).Scan(&s).Error)
	return s
}

// disable runs the real disable protocol (design doc 6.5.3).
func (h *harness) disable(uid int64) error {
	_, err := h.core.Revoker.Revoke(context.Background(), uid, RevokeSpec{
		Apply: func(tx *gorm.DB, _ int64) error {
			return tx.Exec("UPDATE sys_users SET status = 2 WHERE id = ?", uid).Error
		},
	})
	return err
}

// client is a browser-like client with a cookie jar that sends Origin and
// the double-submit header.
type client struct {
	h    *harness
	http *http.Client
}

func (h *harness) newClient() *client {
	jar, err := cookiejar.New(nil)
	require.NoError(h.t, err)
	return &client{h: h, http: &http.Client{Jar: jar}}
}

type response struct {
	Status int
	Code   int
	Data   json.RawMessage
	Header http.Header
}

func (c *client) do(method, path string, body any, mutate ...func(*http.Request)) response {
	c.h.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(c.h.t, err)
		reader = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(context.Background(), method, c.h.server.URL+path, reader)
	require.NoError(c.h.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	if tok := c.cookie(CSRFCookie); tok != "" {
		req.Header.Set(CSRFHeader, tok)
	}
	for _, m := range mutate {
		m(req)
	}
	resp, err := c.http.Do(req)
	require.NoError(c.h.t, err)
	defer func() { _ = resp.Body.Close() }()
	var env struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	raw, err := io.ReadAll(resp.Body)
	require.NoError(c.h.t, err)
	_ = json.Unmarshal(raw, &env)
	return response{Status: resp.StatusCode, Code: env.Code, Data: env.Data, Header: resp.Header}
}

func (c *client) cookie(name string) string {
	u, _ := url.Parse(c.h.server.URL)
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == name {
			return ck.Value
		}
	}
	return ""
}

func (c *client) login(username string) response {
	return c.do(http.MethodPost, "/api/v1/auth/login", gin.H{"username": username, "password": testPass})
}

func (c *client) mustLogin(username string) {
	c.h.t.Helper()
	resp := c.login(username)
	require.Equal(c.h.t, http.StatusOK, resp.Status, "login %s: code %d", username, resp.Code)
}

func (c *client) ping() int { return c.do(http.MethodGet, "/api/v1/ping", nil).Status }

// session returns the server-side session of the client's cookie.
func (c *client) session() Session {
	c.h.t.Helper()
	sess, ok, err := c.h.core.Sessions.Get(context.Background(), c.cookie("sid"))
	require.NoError(c.h.t, err)
	require.True(c.h.t, ok)
	return sess
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}
