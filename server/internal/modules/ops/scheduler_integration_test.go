//go:build integration

package ops

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cronRun struct {
	ScheduledAt time.Time
	Instance    string
	Status      string
	Trigger     string
}

func (e *env) cronRuns(jobID int64) []cronRun {
	e.t.Helper()
	var rows []cronRun
	require.NoError(e.t, e.db.Raw(`SELECT scheduled_at, instance, status, trigger FROM sys_job_runs
		WHERE job_id = ? AND trigger = 'cron' ORDER BY scheduled_at`, jobID).Scan(&rows).Error)
	return rows
}

func (e *env) insertJob(name, handler, expr string) int64 {
	e.t.Helper()
	var id int64
	require.NoError(e.t, e.db.Raw(`INSERT INTO sys_jobs (name, handler_key, cron_expr) VALUES (?, ?, ?) RETURNING id`,
		name, handler, expr).Scan(&id).Error)
	return id
}

// Two instances share PostgreSQL and Redis and fire the same schedule:
// every fire runs exactly once.
func TestSingleExecutionAcrossInstances(t *testing.T) {
	e := newEnv(t)
	id := e.insertJob("every-second", "test.count", "@every 1s")
	a := e.newInstance("inst-a", nil)
	b := e.newInstance("inst-b", nil)
	a.start()
	b.start()

	eventually(t, 10*time.Second, func() bool { return len(e.cronRuns(id)) >= 4 }, "expected at least 4 fires")
	a.stop()
	b.stop()

	runs := e.cronRuns(id)
	seen := map[time.Time]bool{}
	for _, r := range runs {
		assert.False(t, seen[r.ScheduledAt], "fire %s ran twice", r.ScheduledAt)
		seen[r.ScheduledAt] = true
		assert.Equal(t, statusSuccess, r.Status)
		assert.Zero(t, r.ScheduledAt.Nanosecond(), "fires are aligned to whole seconds")
	}
	assert.Equal(t, int64(len(runs)), e.fakes.countCalls.Load(), "handler calls must equal recorded runs")
	assert.Equal(t, int64(len(runs)), e.countRuns("job_id = ?", id), "no skipped or duplicate rows")
	var last string
	require.NoError(t, e.db.Raw("SELECT last_status FROM sys_jobs WHERE id = ?", id).Scan(&last).Error)
	assert.Equal(t, statusSuccess, last)
}

// A change made through one instance reaches a running scheduler on another
// through pub/sub (resync is set to an hour here).
func TestReloadAfterChangeViaPubSub(t *testing.T) {
	e := newEnv(t)
	api := e.newInstance("inst-api", nil)
	worker := e.newInstance("inst-worker", nil)
	worker.start()

	job := api.createJob(jobBody("pubsub", "test.count", "@every 1s", nil))
	id := int64(job.ID)
	eventually(t, 5*time.Second, func() bool { return len(e.cronRuns(id)) >= 1 }, "new job was not scheduled")
	for _, r := range e.cronRuns(id) {
		assert.Equal(t, "inst-worker", r.Instance)
	}

	cur := api.getJob(job.ID)
	r := api.do(http.MethodPut, "/jobs/"+job.ID.String()+"/status", map[string]any{"enabled": false, "version": cur.Version})
	require.Equal(t, http.StatusOK, r.Status)
	time.Sleep(300 * time.Millisecond) // let an in-progress fire finish
	before := len(e.cronRuns(id))
	time.Sleep(2500 * time.Millisecond)
	assert.Equal(t, before, len(e.cronRuns(id)), "disabled job kept firing")

	// Changing the schedule takes effect too.
	cur = api.getJob(job.ID)
	r = api.do(http.MethodPut, "/jobs/"+job.ID.String(), jobBody("pubsub", "test.count", "@every 1s",
		map[string]any{"enabled": true, "version": cur.Version}))
	require.Equal(t, http.StatusOK, r.Status)
	eventually(t, 5*time.Second, func() bool { return len(e.cronRuns(id)) > before }, "re-enabled job did not fire")
}

// Without any notification, the periodic resync picks up direct database
// changes and fails runs whose instance died.
func TestPeriodicResync(t *testing.T) {
	e := newEnv(t)
	worker := e.newInstance("inst-worker", func(tu *Tuning) { tu.ResyncInterval = 200 * time.Millisecond })
	worker.start()

	id := e.insertJob("direct", "test.count", "@every 1s")
	eventually(t, 5*time.Second, func() bool { return len(e.cronRuns(id)) >= 1 }, "resync did not pick up the job")

	var orphan int64
	require.NoError(t, e.db.Raw(`INSERT INTO sys_job_runs (job_id, job_name, handler_key, trigger, status, instance, expires_at)
		VALUES (?, 'direct', 'test.count', 'manual', 'running', 'dead', now() - interval '1 second') RETURNING id`, id).Scan(&orphan).Error)
	eventually(t, 5*time.Second, func() bool {
		return e.countRuns("id = ? AND status = 'failed' AND error <> ''", orphan) == 1
	}, "expired run was not failed")

	require.NoError(t, e.db.Exec("DELETE FROM sys_jobs WHERE id = ?", id).Error)
	time.Sleep(500 * time.Millisecond)
	before := e.countRuns("job_name = 'direct' AND trigger = 'cron'")
	time.Sleep(2 * time.Second)
	assert.Equal(t, before, e.countRuns("job_name = 'direct' AND trigger = 'cron'"), "deleted job kept firing")
}

// A cron fire that finds the previous run still going is recorded as skipped.
func TestCronFireSkippedWhileRunning(t *testing.T) {
	e := newEnv(t)
	id := e.insertJob("overlap", "test.block", "@every 1s")
	a := e.newInstance("inst-a", nil)
	a.start()
	eventually(t, 6*time.Second, func() bool {
		return e.countRuns("job_id = ? AND status = 'skipped'", id) >= 1
	}, "overlapping fire was not skipped")
	assert.Equal(t, int64(1), e.countRuns("job_id = ? AND status = 'running'", id))
	e.fakes.unblock()
	a.stop()
	assert.Zero(t, e.countRuns("job_id = ? AND status = 'running'", id))
}

// A fire key held by another instance means this instance does nothing.
func TestFireClaimIsExclusive(t *testing.T) {
	e := newEnv(t)
	id := e.insertJob("claimed", "test.count", "@daily")
	a := e.newInstance("inst-a", nil)
	fire := time.Now().Truncate(time.Second)
	require.NoError(t, e.rdb.Set(context.Background(), fireKey(id, fire), "inst-other", time.Minute).Err())
	a.m.sched.fireCron(id, fire)
	assert.Zero(t, e.countRuns("job_id = ?", id))

	next := fire.Add(time.Second)
	a.m.sched.fireCron(id, next)
	a.m.sched.fireCron(id, next)
	assert.Equal(t, int64(1), e.countRuns("job_id = ?", id))
	assert.Equal(t, int64(1), e.fakes.countCalls.Load())
}

// The job lock is short-lived but renewed while the run lasts, so a crash
// blocks the job only briefly, and it is released when the run ends.
func TestJobLockIsRenewedAndReleased(t *testing.T) {
	e := newEnv(t)
	a := e.newInstance("inst-a", func(t *Tuning) { t.LockTTL = 300 * time.Millisecond })
	a.start()
	job := a.createJob(jobBody("renewed", "test.block", "@daily", nil))
	run := a.runNow(job.ID)
	require.Equal(t, statusRunning, run.Status)

	key := lockKey(int64(job.ID))
	time.Sleep(time.Second) // longer than three TTLs
	ttl, err := e.rdb.PTTL(context.Background(), key).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0), "the lock is still held thanks to renewal")
	assert.LessOrEqual(t, ttl, 300*time.Millisecond)

	e.fakes.unblock()
	assert.Equal(t, statusSuccess, a.waitRun(run.RunID).Status)
	eventually(t, 2*time.Second, func() bool {
		return e.rdb.Exists(context.Background(), key).Val() == 0
	}, "the lock was not released")
}
