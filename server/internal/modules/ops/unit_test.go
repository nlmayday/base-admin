package ops

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"base-admin/server/internal/core/jobs"
)

func TestParseCronAccepts(t *testing.T) {
	base := time.Date(2026, 1, 5, 10, 7, 30, 0, time.Local) // a Monday
	cases := []struct {
		expr string
		want time.Time
	}{
		{"* * * * *", time.Date(2026, 1, 5, 10, 8, 0, 0, time.Local)},
		{"0 * * * *", time.Date(2026, 1, 5, 11, 0, 0, 0, time.Local)},
		{"0 3 * * *", time.Date(2026, 1, 6, 3, 0, 0, 0, time.Local)},
		{"0 0 * * 1", time.Date(2026, 1, 12, 0, 0, 0, 0, time.Local)},
		{"*/15 9-18 * * 1-5", time.Date(2026, 1, 5, 10, 15, 0, 0, time.Local)},
		{"@daily", time.Date(2026, 1, 6, 0, 0, 0, 0, time.Local)},
		{"@hourly", time.Date(2026, 1, 5, 11, 0, 0, 0, time.Local)},
		{"  @weekly ", time.Date(2026, 1, 11, 0, 0, 0, 0, time.Local)},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			sched, err := parseCron(tc.expr)
			require.NoError(t, err)
			assert.Equal(t, tc.want, sched.Next(base))
		})
	}
}

func TestParseCronRejects(t *testing.T) {
	cases := map[string]string{
		"":                        "不能为空",
		"* * * *":                 "表达式无效",
		"0 0 0 * * *":             "表达式无效", // seconds field is not allowed
		"61 * * * *":              "表达式无效",
		"@every 1500ms":           "整秒",
		"@every soon":             "应为时长",
		"@every 500ms":            "不能小于 1 秒",
		"@yearlyish":              "表达式无效",
		"0 0 30 2 *":              "永远不会触发",
		"CRON_TZ=UTC 0 * * * *":   "不支持指定时区",
		"TZ=Asia/Shanghai @daily": "不支持指定时区",
		strings.Repeat("1", 129):  "不能多于",
	}
	for expr, msg := range cases {
		t.Run(expr, func(t *testing.T) {
			_, err := parseCron(expr)
			require.Error(t, err)
			assert.Contains(t, err.Error(), msg)
		})
	}
}

func TestEveryIsAlignedAcrossInstances(t *testing.T) {
	sched, err := parseCron("@every 10s")
	require.NoError(t, err)
	// Two instances asking at different moments agree on the fire times.
	a := sched.Next(time.Date(2026, 1, 1, 0, 0, 3, 0, time.UTC))
	b := sched.Next(time.Date(2026, 1, 1, 0, 0, 7, 500, time.UTC))
	assert.Equal(t, a, b)
	assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 10, 0, time.UTC), a)
	assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 20, 0, time.UTC), sched.Next(a))
}

func TestNextTimes(t *testing.T) {
	sched, err := parseCron("@every 1h")
	require.NoError(t, err)
	from := time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC)
	got := nextTimes(sched, from, 5)
	require.Len(t, got, 5)
	for i, ts := range got {
		assert.Equal(t, time.Date(2026, 1, 1, i+1, 0, 0, 0, time.UTC), ts)
	}
	assert.Nil(t, nextRun("bogus", from))
	require.NotNil(t, nextRun("@every 1h", from))
}

func TestRetryDelay(t *testing.T) {
	p := retryPolicy{base: time.Second, max: 10 * time.Second}
	cases := []struct {
		failed int
		want   time.Duration
	}{
		{0, 0}, {1, time.Second}, {2, 2 * time.Second}, {3, 4 * time.Second},
		{4, 8 * time.Second}, {5, 10 * time.Second}, {60, 10 * time.Second},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, p.delay(tc.failed), "failed=%d", tc.failed)
	}
	assert.Equal(t, time.Duration(0), retryPolicy{}.delay(3))
	// 3 attempts of 5s plus waits of 1s and 2s.
	assert.Equal(t, 18*time.Second, p.worstCase(5*time.Second, 2))
	assert.Equal(t, 5*time.Second, p.worstCase(5*time.Second, 0))
}

func noopRun(context.Context, json.RawMessage) error { return nil }

func TestHandlerSetValidation(t *testing.T) {
	good := jobs.Handler{Key: "audit.cleanup", Name: "清理", Run: noopRun}
	own := jobs.Handler{Key: "ops.cleanup-runs", Name: "清理执行记录", Run: noopRun}

	set, err := newHandlerSet([]jobs.Handler{own}, []jobs.Handler{good, own})
	require.NoError(t, err, "re-passing the module's own handler is allowed")
	assert.Len(t, set.list(), 2)
	_, ok := set.get("audit.cleanup")
	assert.True(t, ok)
	_, ok = set.get("nope.x")
	assert.False(t, ok)

	bad := []jobs.Handler{
		{Key: "nodot", Name: "x", Run: noopRun},
		{Key: "Upper.case", Name: "x", Run: noopRun},
		{Key: "a.b", Name: "", Run: noopRun},
		{Key: "a.c", Name: "x"},
		{Key: "a.d", Name: "x", Run: noopRun, DefaultTimeout: 48 * time.Hour},
		{Key: "a." + strings.Repeat("x", 64), Name: "x", Run: noopRun},
	}
	for _, h := range bad {
		_, err := newHandlerSet(nil, []jobs.Handler{h})
		assert.Error(t, err, h.Key)
	}
	_, err = newHandlerSet(nil, []jobs.Handler{good, good})
	assert.ErrorContains(t, err, "registered twice")
}

func TestNewPanicsOnInvalidHandlers(t *testing.T) {
	assert.Panics(t, func() { New(Deps{Handlers: []jobs.Handler{{Key: "bad"}}}) })
	m := New(Deps{Handlers: []jobs.Handler{{Key: "x.y", Name: "y", Run: noopRun}}})
	assert.Len(t, m.Jobs(), 1)
	assert.Len(t, m.handlers.list(), 2)
}

func TestEffectiveTimeout(t *testing.T) {
	h := jobs.Handler{DefaultTimeout: time.Minute}
	assert.Equal(t, 30*time.Second, effectiveTimeout(30, h))
	assert.Equal(t, time.Minute, effectiveTimeout(0, h))
	assert.Equal(t, defaultJobTimeout, effectiveTimeout(0, jobs.Handler{}))
}

func TestAttemptOnceOutcomes(t *testing.T) {
	s := newScheduler(nil, nil, handlerSet{}, zap.NewNop(), "test", Tuning{}.withDefaults())
	var handlers sync.WaitGroup
	run := func(fn func(context.Context, json.RawMessage) error, timeout time.Duration) (string, string) {
		return s.attemptOnce(1, jobs.Handler{Key: "t.t", Run: fn}, nil, timeout, &handlers)
	}
	status, msg := run(noopRun, time.Second)
	assert.Equal(t, statusSuccess, status)
	assert.Empty(t, msg)

	status, msg = run(func(context.Context, json.RawMessage) error { return errors.New("boom") }, time.Second)
	assert.Equal(t, statusFailed, status)
	assert.Equal(t, "boom", msg)

	status, msg = run(func(context.Context, json.RawMessage) error { panic("kaput") }, time.Second)
	assert.Equal(t, statusFailed, status)
	assert.Equal(t, "panic: kaput", msg)

	// A handler honouring ctx and one ignoring it both time out.
	status, _ = run(func(ctx context.Context, _ json.RawMessage) error { <-ctx.Done(); return ctx.Err() }, 50*time.Millisecond)
	assert.Equal(t, statusTimeout, status)
	release := make(chan struct{})
	defer close(release)
	start := time.Now()
	status, msg = run(func(context.Context, json.RawMessage) error { <-release; return nil }, 50*time.Millisecond)
	assert.Equal(t, statusTimeout, status)
	assert.Contains(t, msg, "执行超时")
	assert.Less(t, time.Since(start), time.Second)

	// The abandoned handler is still tracked, so the job lock is kept until
	// it really returns.
	waited := make(chan struct{})
	go func() { handlers.Wait(); close(waited) }()
	select {
	case <-waited:
		t.Fatal("the abandoned handler must still be tracked")
	case <-time.After(50 * time.Millisecond):
	}
	release <- struct{}{}
	<-waited
}

func TestShutdownCancelsRunningJobsAfterGrace(t *testing.T) {
	s := newScheduler(nil, nil, handlerSet{}, zap.NewNop(), "test",
		Tuning{ShutdownGrace: 50 * time.Millisecond}.withDefaults())
	done, ok := s.track()
	require.True(t, ok)
	result := make(chan string, 1)
	go func() {
		defer done()
		var handlers sync.WaitGroup
		status, msg := s.attemptOnce(1, jobs.Handler{Key: "t.t", Run: func(ctx context.Context, _ json.RawMessage) error {
			<-ctx.Done()
			return ctx.Err()
		}}, nil, time.Hour, &handlers)
		result <- status + "|" + msg
	}()

	start := time.Now()
	s.shutdown()
	assert.Less(t, time.Since(start), 2*time.Second, "shutdown is bounded by the grace period")
	assert.Equal(t, statusFailed+"|服务停止，执行被中断", <-result)
	_, ok = s.track()
	assert.False(t, ok, "no new runs after shutdown")
}

func TestCleanupParams(t *testing.T) {
	cases := []struct {
		raw     string
		days    int
		wantErr bool
	}{
		{"", 30, false}, {"null", 30, false}, {"{}", 30, false}, {`{"days":7}`, 7, false},
		{`{"days":0}`, 0, true}, {`{"days":99999}`, 0, true}, {`{"days":"x"}`, 0, true}, {`[1]`, 0, true},
	}
	for _, tc := range cases {
		days, err := parseCleanupParams(json.RawMessage(tc.raw))
		if tc.wantErr {
			assert.Error(t, err, tc.raw)
			continue
		}
		require.NoError(t, err, tc.raw)
		assert.Equal(t, tc.days, days, tc.raw)
	}
}

func TestNormalizeParams(t *testing.T) {
	p, msg := normalizeParams(nil)
	assert.Equal(t, "{}", p)
	assert.Empty(t, msg)
	p, msg = normalizeParams(json.RawMessage(` {"a":1} `))
	assert.Equal(t, `{"a":1}`, p)
	assert.Empty(t, msg)
	_, msg = normalizeParams(json.RawMessage(`[1,2]`))
	assert.NotEmpty(t, msg)
	_, msg = normalizeParams(json.RawMessage(`"x"`))
	assert.NotEmpty(t, msg)
	_, msg = normalizeParams(json.RawMessage(`{"a":"` + strings.Repeat("x", maxParamsBytes) + `"}`))
	assert.Contains(t, msg, "8 KB")
}

func TestParseRedisInfo(t *testing.T) {
	kv := parseRedisInfo("# Server\r\nredis_version:7.2.4\r\nuptime_in_seconds:42\r\n\r\n# Clients\r\nconnected_clients:3\r\n")
	assert.Equal(t, "7.2.4", kv["redis_version"])
	assert.Equal(t, "42", kv["uptime_in_seconds"])
	assert.Equal(t, "3", kv["connected_clients"])
	assert.Len(t, kv, 3)
}

func TestTruncateKeepsUTF8(t *testing.T) {
	assert.Equal(t, "short", truncate("short"))
	long := strings.Repeat("错", maxErrorLength)
	got := truncate(long)
	assert.True(t, strings.HasSuffix(got, "…"))
	assert.LessOrEqual(t, len(got), maxErrorLength+len("…"))
	assert.True(t, strings.HasPrefix(long, strings.TrimSuffix(got, "…")))
}

func TestModuleDefinitions(t *testing.T) {
	m := New(Deps{})
	assert.Equal(t, "ops", m.Name())
	assert.Len(t, m.Permissions(), 7)
	menus := m.Menus()
	require.Len(t, menus, 4)
	assert.Equal(t, 95, menus[0].Sort)
	assert.Equal(t, "ops/monitor/index", menus[3].Component)
	assert.NoError(t, m.Seed(context.Background(), nil))
}
