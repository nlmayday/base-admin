//go:build integration

package ops

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/datascope"
	"base-admin/server/internal/core/jobs"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
	"base-admin/server/internal/platform/testinfra"
	"base-admin/server/internal/platform/testinfra/migrated"
)

const (
	redisDBOps = 7
	permHeader = "X-Test-Perms"
)

// testGuard authenticates every request as uid with the permissions listed
// in the X-Test-Perms header ("none" for no permission; absent means every
// ops permission) and enforces access levels like the real guard.
type testGuard struct{ uid int64 }

func (g testGuard) Require(level access.Level) gin.HandlerFunc {
	return func(c *gin.Context) {
		if level.Kind() == access.KindPublic {
			c.Next()
			return
		}
		perms := allPerms()
		if h, ok := c.Request.Header[permHeader]; ok {
			perms = nil
			if h[0] != "none" {
				perms = strings.Split(h[0], ",")
			}
		}
		slices.Sort(perms)
		snap := &actor.Snapshot{UserID: g.uid, Status: actor.StatusEnabled, Permissions: perms,
			DataScope: datascope.Resolved{All: true}}
		c.Request = c.Request.WithContext(actor.With(c.Request.Context(), &actor.Actor{Snapshot: snap}))
		if level.Kind() == access.KindPermission && !level.Allows(snap.HasPermission) {
			httpx.Fail(c, httpx.NewError(http.StatusForbidden, 20002, "无权限"))
			return
		}
		c.Next()
	}
}

func allPerms() []string {
	out := make([]string, len(permissionDefs))
	for i, d := range permissionDefs {
		out[i] = d.Code
	}
	return out
}

// fakes are test handlers with observable behaviour.
type fakes struct {
	okCalls    atomic.Int64
	flakyCalls atomic.Int64
	countCalls atomic.Int64
	block      chan struct{}
	blockOnce  sync.Once
}

func newFakes() *fakes { return &fakes{block: make(chan struct{})} }

func (f *fakes) unblock() { f.blockOnce.Do(func() { close(f.block) }) }

func (f *fakes) handlers() []jobs.Handler {
	h := func(key string, run func(context.Context, json.RawMessage) error) jobs.Handler {
		return jobs.Handler{Key: key, Name: key, Description: "test " + key, DefaultTimeout: 5 * time.Second, Run: run}
	}
	return []jobs.Handler{
		h("test.ok", func(context.Context, json.RawMessage) error { f.okCalls.Add(1); return nil }),
		h("test.fail", func(context.Context, json.RawMessage) error { return errorString("boom") }),
		h("test.slow", func(ctx context.Context, _ json.RawMessage) error { <-ctx.Done(); return ctx.Err() }),
		h("test.panic", func(context.Context, json.RawMessage) error { panic("kaput") }),
		// test.flaky fails twice, then succeeds.
		h("test.flaky", func(context.Context, json.RawMessage) error {
			if f.flakyCalls.Add(1) < 3 {
				return errorString("not yet")
			}
			return nil
		}),
		h("test.count", func(context.Context, json.RawMessage) error { f.countCalls.Add(1); return nil }),
		h("test.block", func(ctx context.Context, _ json.RawMessage) error {
			select {
			case <-f.block:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}),
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }

type env struct {
	t     *testing.T
	db    *gorm.DB
	rdb   *redis.Client
	deps  Deps
	fakes *fakes
	uid   int64
}

func fastTuning() Tuning {
	return Tuning{ResyncInterval: time.Hour, RetryBaseDelay: 50 * time.Millisecond,
		RetryMaxDelay: 200 * time.Millisecond, LockMargin: 2 * time.Second, MonitorCacheTTL: time.Second,
		MinEvery: time.Second}
}

func newEnv(t *testing.T) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, httpx.RegisterJSONFieldNames())
	ctx := context.Background()
	dbCfg := migrated.Database(t)
	sqlDB, err := database.OpenSQL(ctx, dbCfg, "ops-test")
	require.NoError(t, err)
	gdb, err := database.NewGorm(sqlDB, dbCfg, zap.NewNop())
	require.NoError(t, err)
	redisCfg := testinfra.RedisDB(t, redisDBOps)
	rdb := redis.NewClient(&redis.Options{Addr: redisCfg.Addr, DB: redisCfg.DB})
	t.Cleanup(func() { _ = rdb.Close(); _ = sqlDB.Close() })

	var uid int64
	require.NoError(t, gdb.Raw(`INSERT INTO sys_users (username, password_hash) VALUES ('ops-tester', 'x') RETURNING id`).Scan(&uid).Error)
	f := newFakes()
	t.Cleanup(f.unblock)
	e := &env{t: t, db: gdb, rdb: rdb, fakes: f, uid: uid}
	e.deps = Deps{DB: gdb, SQLDB: sqlDB, Redis: rdb, Handlers: f.handlers(), Logger: zap.NewNop(),
		InstanceID: "inst-a", StartedAt: time.Now().Add(-time.Minute), Version: "test", Tuning: fastTuning()}
	return e
}

// instance is one module mounted on its own HTTP server.
type instance struct {
	env    *env
	m      *Module
	server *httptest.Server
	stop   func()
}

func (e *env) newInstance(id string, tune func(*Tuning)) *instance {
	d := e.deps
	d.InstanceID = id
	if tune != nil {
		tune(&d.Tuning)
	}
	m := New(d)
	engine := gin.New()
	api := engine.Group("/api/v1")
	m.Routes(module.NewRouter(api, m.Name(), testGuard{uid: e.uid}, module.NewRegistry()))
	srv := httptest.NewServer(engine)
	e.t.Cleanup(srv.Close)
	return &instance{env: e, m: m, server: srv, stop: func() {}}
}

// start runs the scheduler until stop (or test end) and waits until it has
// subscribed and loaded the jobs.
func (in *instance) start() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		in.m.Run(ctx)
	}()
	var once sync.Once
	in.stop = func() { once.Do(func() { cancel(); <-done }) }
	in.env.t.Cleanup(in.stop)
	select {
	case <-in.m.sched.ready:
	case <-time.After(5 * time.Second):
		in.env.t.Fatal("scheduler did not become ready")
	}
}

type response struct {
	Status int
	Code   int
	Data   json.RawMessage
}

func (in *instance) do(method, path string, body any, perms ...string) response {
	t := in.env.t
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(context.Background(), method, in.server.URL+"/api/v1"+path, reader)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if len(perms) > 0 {
		req.Header.Set(permHeader, strings.Join(perms, ","))
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	var envl struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envl))
	return response{Status: resp.StatusCode, Code: envl.Code, Data: envl.Data}
}

func decode[T any](t *testing.T, r response) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(r.Data, &v), string(r.Data))
	return v
}

func (in *instance) createJob(body map[string]any) jobView {
	t := in.env.t
	t.Helper()
	r := in.do(http.MethodPost, "/jobs", body)
	require.Equal(t, http.StatusOK, r.Status, string(r.Data))
	return decode[jobView](t, r)
}

func jobBody(name, handler, expr string, extra map[string]any) map[string]any {
	b := map[string]any{"name": name, "handlerKey": handler, "cronExpr": expr}
	for k, v := range extra {
		b[k] = v
	}
	return b
}

func (in *instance) runNow(id ids.ID) runNowView {
	t := in.env.t
	t.Helper()
	r := in.do(http.MethodPost, "/jobs/"+id.String()+"/run", nil)
	require.Equal(t, http.StatusOK, r.Status, string(r.Data))
	return decode[runNowView](t, r)
}

// waitRun polls a run until it is no longer running.
func (in *instance) waitRun(id ids.ID) runView {
	t := in.env.t
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		r := in.do(http.MethodGet, "/job-runs/"+id.String(), nil)
		require.Equal(t, http.StatusOK, r.Status)
		v := decode[runView](t, r)
		if v.Status != statusRunning {
			return v
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run %s did not finish", id)
	return runView{}
}

func (in *instance) getJob(id ids.ID) jobView {
	in.env.t.Helper()
	r := in.do(http.MethodGet, "/jobs/"+id.String(), nil)
	require.Equal(in.env.t, http.StatusOK, r.Status)
	return decode[jobView](in.env.t, r)
}

// eventually polls cond until it holds or the timeout expires.
func eventually(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal(msg)
}

func (e *env) countRuns(where string, args ...any) int64 {
	e.t.Helper()
	var n int64
	require.NoError(e.t, e.db.Raw("SELECT count(*) FROM sys_job_runs WHERE "+where, args...).Scan(&n).Error)
	return n
}
