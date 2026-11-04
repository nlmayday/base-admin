//go:build integration

package ops

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/httpx"
)

func TestManualRunOutcomes(t *testing.T) {
	e := newEnv(t)
	a := e.newInstance("inst-a", nil)

	cases := []struct {
		handler    string
		extra      map[string]any
		status     string
		attempts   int
		errContain string
	}{
		{"test.ok", nil, statusSuccess, 1, ""},
		{"test.fail", nil, statusFailed, 1, "boom"},
		{"test.slow", map[string]any{"timeoutSeconds": 1}, statusTimeout, 1, "执行超时"},
		{"test.panic", nil, statusFailed, 1, "panic: kaput"},
		{"test.flaky", map[string]any{"maxRetries": 3}, statusSuccess, 3, ""},
		{"test.fail", map[string]any{"maxRetries": 2}, statusFailed, 3, "boom"},
	}
	for i, tc := range cases {
		t.Run(tc.handler, func(t *testing.T) {
			job := a.createJob(jobBody("manual-"+string(rune('a'+i)), tc.handler, "@daily", tc.extra))
			started := a.runNow(job.ID)
			assert.Equal(t, statusRunning, started.Status)
			run := a.waitRun(started.RunID)
			assert.Equal(t, tc.status, run.Status)
			assert.Equal(t, tc.attempts, run.Attempt)
			assert.Equal(t, triggerManual, run.Trigger)
			assert.Equal(t, "inst-a", run.Instance)
			require.NotNil(t, run.TriggeredBy)
			assert.Equal(t, e.uid, int64(*run.TriggeredBy))
			require.NotNil(t, run.FinishedAt)
			require.NotNil(t, run.DurationMs)
			assert.Nil(t, run.ScheduledAt)
			if tc.errContain == "" {
				assert.Empty(t, run.Error)
			} else {
				assert.Contains(t, run.Error, tc.errContain)
			}
			after := a.getJob(job.ID)
			assert.Equal(t, tc.status, after.LastStatus)
			assert.NotNil(t, after.LastRunAt)
			// The lock is released right after the run is recorded.
			eventually(t, 2*time.Second, func() bool {
				return e.rdb.Exists(context.Background(), lockKey(int64(job.ID))).Val() == 0
			}, "job lock was not released")
		})
	}
	// A disabled job can still be run manually.
	job := a.createJob(jobBody("disabled-manual", "test.ok", "@daily", map[string]any{"enabled": false}))
	assert.Equal(t, statusSuccess, a.waitRun(a.runNow(job.ID).RunID).Status)
}

func TestManualRunSkippedWhileRunning(t *testing.T) {
	e := newEnv(t)
	a := e.newInstance("inst-a", nil)
	b := e.newInstance("inst-b", nil)
	job := a.createJob(jobBody("blocking", "test.block", "@daily", nil))

	first := a.runNow(job.ID)
	require.Equal(t, statusRunning, first.Status)
	// The lock is shared through Redis, so another instance skips too.
	second := b.runNow(job.ID)
	assert.Equal(t, statusSkipped, second.Status)
	skipped := a.waitRun(second.RunID)
	assert.Equal(t, statusSkipped, skipped.Status)
	assert.Contains(t, skipped.Error, "尚未结束")
	assert.Equal(t, statusRunning, a.getJob(job.ID).LastStatus, "a skip does not hide the running run")

	e.fakes.unblock()
	assert.Equal(t, statusSuccess, a.waitRun(first.RunID).Status)
	third := b.runNow(job.ID)
	assert.Equal(t, statusRunning, third.Status)
	assert.Equal(t, statusSuccess, b.waitRun(third.RunID).Status)
}

func TestManualRunErrors(t *testing.T) {
	e := newEnv(t)
	a := e.newInstance("inst-a", nil)
	assert.Equal(t, http.StatusNotFound, a.do(http.MethodPost, "/jobs/999999/run", nil).Status)

	// A job whose handler was removed from the build cannot run.
	require.NoError(t, e.db.Exec(`INSERT INTO sys_jobs (name, handler_key, cron_expr) VALUES ('gone', 'gone.handler', '@daily')`).Error)
	var id int64
	require.NoError(t, e.db.Raw("SELECT id FROM sys_jobs WHERE name = 'gone'").Scan(&id).Error)
	list := decode[httpx.Page[jobView]](t, a.do(http.MethodGet, "/jobs?keyword=gone", nil))
	require.Len(t, list.List, 1)
	assert.False(t, list.List[0].HandlerExists)
	r := a.do(http.MethodPost, "/jobs/"+list.List[0].ID.String()+"/run", nil)
	assert.Equal(t, http.StatusConflict, r.Status)
	assert.Equal(t, CodeHandlerMissing, r.Code)

	// After shutdown manual runs are refused.
	a.start()
	a.stop()
	job := a.createJob(jobBody("after-stop", "test.ok", "@daily", nil))
	r = a.do(http.MethodPost, "/jobs/"+job.ID.String()+"/run", nil)
	assert.Equal(t, http.StatusServiceUnavailable, r.Status)
	assert.Equal(t, CodeSchedulerStopping, r.Code)
}

func TestShutdownWaitsForInflightRuns(t *testing.T) {
	e := newEnv(t)
	a := e.newInstance("inst-a", nil)
	a.start()
	job := a.createJob(jobBody("inflight", "test.block", "@daily", nil))
	run := a.runNow(job.ID)

	stopped := make(chan struct{})
	go func() {
		a.stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("Run returned while a run was in flight")
	case <-time.After(300 * time.Millisecond):
	}
	e.fakes.unblock()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the run finished")
	}
	assert.Equal(t, statusSuccess, a.waitRun(run.RunID).Status)
}

func TestRunListFilters(t *testing.T) {
	e := newEnv(t)
	a := e.newInstance("inst-a", nil)
	okJob := a.createJob(jobBody("ok-job", "test.ok", "@daily", nil))
	failJob := a.createJob(jobBody("fail-job", "test.fail", "@daily", nil))
	for range 2 {
		a.waitRun(a.runNow(okJob.ID).RunID)
	}
	failed := a.waitRun(a.runNow(failJob.ID).RunID)

	page := func(q url.Values) httpx.Page[runView] {
		r := a.do(http.MethodGet, "/job-runs?"+q.Encode(), nil)
		require.Equal(t, http.StatusOK, r.Status, string(r.Data))
		return decode[httpx.Page[runView]](t, r)
	}
	assert.Equal(t, int64(3), page(url.Values{}).Total)
	assert.Equal(t, int64(2), page(url.Values{"jobId": {okJob.ID.String()}}).Total)
	got := page(url.Values{"status": {statusFailed}})
	require.Equal(t, int64(1), got.Total)
	assert.Equal(t, failed.ID, got.List[0].ID)
	assert.Equal(t, int64(3), page(url.Values{"trigger": {triggerManual}}).Total)
	assert.Equal(t, int64(0), page(url.Values{"trigger": {triggerCron}}).Total)
	now := time.Now()
	assert.Equal(t, int64(3), page(url.Values{"startFrom": {now.Add(-time.Hour).Format(time.RFC3339)},
		"startTo": {now.Add(time.Hour).Format(time.RFC3339)}}).Total)
	assert.Equal(t, int64(0), page(url.Values{"startFrom": {now.Add(time.Hour).Format(time.RFC3339)}}).Total)

	bad := a.do(http.MethodGet, "/job-runs?status=weird&trigger=x&jobId=abc&startFrom=yesterday", nil)
	fields := fieldsOf(t, bad)
	for _, f := range []string{"status", "trigger", "jobId", "startFrom"} {
		assert.Contains(t, fields, f)
	}
	assert.Equal(t, http.StatusNotFound, a.do(http.MethodGet, "/job-runs/999999", nil).Status)

	// Deleting a job keeps its history.
	require.Equal(t, http.StatusOK, a.do(http.MethodDelete, "/jobs/"+failJob.ID.String(), nil).Status)
	kept := a.waitRun(failed.ID)
	assert.Nil(t, kept.JobID)
	assert.Equal(t, "fail-job", kept.JobName)
}

func TestCleanupHandler(t *testing.T) {
	e := newEnv(t)
	a := e.newInstance("inst-a", nil)
	insert := func(name, status string, age time.Duration) {
		require.NoError(t, e.db.Exec(`INSERT INTO sys_job_runs (job_name, handler_key, "trigger", status, started_at, expires_at)
			VALUES (?, 'test.ok', 'cron', ?, ?, ?)`,
			name, status, time.Now().Add(-age), time.Now().Add(time.Hour)).Error)
	}
	insert("old-success", statusSuccess, 40*24*time.Hour)
	insert("old-failed", statusFailed, 31*24*time.Hour)
	insert("old-running", statusRunning, 40*24*time.Hour)
	insert("recent", statusSuccess, 2*24*time.Hour)
	insert("week-old", statusSuccess, 8*24*time.Hour)

	list := decode[httpx.Page[jobView]](t, a.do(http.MethodGet, "/jobs?handlerKey="+CleanupRunsKey, nil))
	require.Len(t, list.List, 1)
	run := a.waitRun(a.runNow(list.List[0].ID).RunID)
	require.Equal(t, statusSuccess, run.Status, run.Error)
	assert.Zero(t, e.countRuns("job_name IN ('old-success', 'old-failed')"))
	assert.Equal(t, int64(3), e.countRuns("job_name IN ('old-running', 'recent', 'week-old')"))

	// Custom retention via params; bad params fail the run.
	job := a.createJob(jobBody("cleanup-7", CleanupRunsKey, "@daily", map[string]any{"params": map[string]any{"days": 7}}))
	require.Equal(t, statusSuccess, a.waitRun(a.runNow(job.ID).RunID).Status)
	assert.Zero(t, e.countRuns("job_name = 'week-old'"))
	assert.Equal(t, int64(1), e.countRuns("job_name = 'recent'"))

	bad := a.createJob(jobBody("cleanup-bad", CleanupRunsKey, "@daily", map[string]any{"params": map[string]any{"days": 0}}))
	failed := a.waitRun(a.runNow(bad.ID).RunID)
	assert.Equal(t, statusFailed, failed.Status)
	assert.Contains(t, failed.Error, "days")
}
