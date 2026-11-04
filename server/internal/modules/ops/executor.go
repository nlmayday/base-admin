package ops

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"base-admin/server/internal/core/jobs"
)

// Run statuses and triggers (sys_job_runs).
const (
	statusRunning = "running"
	statusSuccess = "success"
	statusFailed  = "failed"
	statusTimeout = "timeout"
	statusSkipped = "skipped"

	triggerCron   = "cron"
	triggerManual = "manual"
)

// fireKeyTTL keeps a fire's dedupe key long enough to cover clock skew
// between instances.
const fireKeyTTL = 10 * time.Minute

// maxErrorLength bounds the stored error text.
const maxErrorLength = 4000

func lockKey(jobID int64) string { return "ops:job:lock:" + strconv.FormatInt(jobID, 10) }

func fireKey(jobID int64, fire time.Time) string {
	return "ops:job:fire:" + strconv.FormatInt(jobID, 10) + ":" + strconv.FormatInt(fire.Unix(), 10)
}

// releaseScript deletes the lock only if this run still owns it, so a run
// that outlived its TTL cannot delete a lock taken by a later run.
var releaseScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0`)

// renewScript extends the lock only if this run still owns it.
var renewScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('PEXPIRE', KEYS[1], ARGV[2])
end
return 0`)

func newToken(instance string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return instance + ":" + hex.EncodeToString(b)
}

// fireCron handles one cron fire. The per-fire key makes sure only one
// instance handles it; the per-job lock then skips the fire when the
// previous run is still going.
func (s *scheduler) fireCron(jobID int64, fire time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	claimed, err := s.rdb.SetNX(ctx, fireKey(jobID, fire), s.instance, fireKeyTTL).Result()
	if err != nil {
		// Without Redis the fire cannot be deduplicated; skipping it is
		// safer than running it on every instance.
		s.logger.Warn("claim job fire failed; fire skipped", zap.Int64("job", jobID), zap.Error(err))
		return
	}
	if !claimed {
		return
	}
	job, found, err := s.loadJob(ctx, jobID)
	if err != nil {
		s.logger.Warn("load job for fire failed", zap.Int64("job", jobID), zap.Error(err))
		return
	}
	if !found || !job.Enabled {
		s.requestReload()
		return
	}
	h, ok := s.handlers.get(job.HandlerKey)
	if !ok {
		s.recordUnrunnable(ctx, job, fire)
		return
	}
	run, err := s.begin(ctx, job, h, triggerCron, &fire, nil)
	if err != nil {
		s.logger.Warn("start job run failed", zap.Int64("job", jobID), zap.Error(err))
		return
	}
	if run.exec != nil {
		cancel()
		run.exec()
	}
}

// startedRun is a run row that was created; exec is nil when the run was
// skipped because the job's lock is held.
type startedRun struct {
	id     int64
	status string
	exec   func()
}

// begin takes the job lock and records the run. The caller runs exec,
// synchronously or in a goroutine.
func (s *scheduler) begin(ctx context.Context, job jobRow, h jobs.Handler, trigger string, fire *time.Time, by *int64) (startedRun, error) {
	timeout := effectiveTimeout(job.TimeoutSeconds, h)
	// The run record expires after the worst case, so a run left behind by a
	// crashed instance is eventually marked failed. The lock itself is short
	// and renewed while the fire runs.
	ttl := s.retry.worstCase(timeout, job.MaxRetries) + s.tuning.LockMargin
	token := newToken(s.instance)
	acquired, err := s.rdb.SetNX(ctx, lockKey(job.ID), token, s.tuning.LockTTL).Result()
	if err != nil {
		return startedRun{}, fmt.Errorf("acquire job lock: %w", err)
	}
	rec := runRecord{job: job, trigger: trigger, scheduledAt: fire, triggeredBy: by, instance: s.instance}
	if !acquired {
		rec.status, rec.errText = statusSkipped, "上一次执行尚未结束，本次跳过"
		id, err := s.insertFinishedRun(ctx, rec)
		return startedRun{id: id, status: statusSkipped}, err
	}
	rec.status, rec.expiresAt = statusRunning, time.Now().Add(ttl)
	id, err := s.insertRunning(ctx, rec)
	if err != nil {
		s.release(job.ID, token)
		return startedRun{}, err
	}
	exec := func() { s.execute(job, h, id, token, timeout) }
	return startedRun{id: id, status: statusRunning, exec: exec}, nil
}

// recordUnrunnable records a fire of a job whose handler is no longer
// registered in this build.
func (s *scheduler) recordUnrunnable(ctx context.Context, job jobRow, fire time.Time) {
	rec := runRecord{job: job, trigger: triggerCron, scheduledAt: &fire, instance: s.instance,
		status: statusFailed, errText: "任务处理器未注册：" + job.HandlerKey}
	if _, err := s.insertFinishedRun(ctx, rec); err != nil {
		s.logger.Warn("record unrunnable job failed", zap.Int64("job", job.ID), zap.Error(err))
		return
	}
	if err := s.db.WithContext(ctx).Exec(`UPDATE sys_jobs SET last_run_at = CURRENT_TIMESTAMP(6), last_status = 'failed' WHERE id = ?`,
		job.ID).Error; err != nil {
		s.logger.Warn("update job last status failed", zap.Int64("job", job.ID), zap.Error(err))
	}
}

// execute runs every attempt of one fire, then records the outcome and
// releases the lock. Retries stop when the scheduler is shutting down.
func (s *scheduler) execute(job jobRow, h jobs.Handler, runID int64, token string, timeout time.Duration) {
	// The lock is held, and renewed, until every handler goroutine has
	// returned: an attempt abandoned at its timeout may still be running and
	// must not overlap the next fire.
	var handlers sync.WaitGroup
	stopRenew := s.keepLock(job.ID, token)
	defer func() {
		go func() {
			handlers.Wait()
			stopRenew()
			s.release(job.ID, token)
		}()
	}()
	started := time.Now()
	params := json.RawMessage(job.Params)
	var status, errText string
	attempt := 1
	for ; ; attempt++ {
		status, errText = s.attemptOnce(job.ID, h, params, timeout, &handlers)
		if status == statusSuccess || attempt > job.MaxRetries {
			break
		}
		if !s.wait(s.retry.delay(attempt)) {
			errText += "（调度器停止，未继续重试）"
			break
		}
		s.markAttempt(runID, attempt+1)
	}
	s.finishRun(runID, job.ID, status, truncate(errText), attempt, time.Since(started))
}

// wait sleeps d unless the scheduler starts stopping first.
func (s *scheduler) wait(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-s.stopCh:
		return false
	}
}

// keepLock renews the job lock every LockTTL/3 until the returned stop
// function is called. If the lock is lost (e.g. Redis was unreachable for a
// whole TTL) the run continues; another instance may then start the job.
func (s *scheduler) keepLock(jobID int64, token string) (stop func()) {
	quit := make(chan struct{})
	go func() {
		ticker := time.NewTicker(s.tuning.LockTTL / 3)
		defer ticker.Stop()
		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
				owned, err := renewScript.Run(ctx, s.rdb, []string{lockKey(jobID)}, token,
					s.tuning.LockTTL.Milliseconds()).Int()
				cancel()
				if err != nil || owned == 0 {
					s.logger.Warn("job lock renewal failed", zap.Int64("job", jobID), zap.Error(err))
				}
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(quit) }) }
}

func (s *scheduler) release(jobID int64, token string) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	if err := releaseScript.Run(ctx, s.rdb, []string{lockKey(jobID)}, token).Err(); err != nil {
		s.logger.Warn("release job lock failed; it will expire", zap.Int64("job", jobID), zap.Error(err))
	}
}

// panicError is a recovered handler panic.
type panicError struct {
	value any
	stack []byte
}

func (p *panicError) Error() string { return fmt.Sprintf("panic: %v", p.value) }

// attemptOnce runs the handler once under timeout. The handler runs in its
// own goroutine so that a handler ignoring ctx cannot hold the run past its
// timeout, and a panic is recovered into a failed status.
func (s *scheduler) attemptOnce(jobID int64, h jobs.Handler, params json.RawMessage, timeout time.Duration,
	handlers *sync.WaitGroup) (status, errText string) {
	// Derived from runCtx so shutdown can cancel a running handler.
	ctx, cancel := context.WithTimeout(s.runCtx, timeout)
	defer cancel()
	done := make(chan error, 1)
	handlers.Add(1)
	go func() {
		defer handlers.Done()
		defer func() {
			if r := recover(); r != nil {
				done <- &panicError{value: r, stack: debug.Stack()}
			}
		}()
		done <- h.Run(ctx, params)
	}()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		err = ctx.Err()
	}
	if pe, ok := errors.AsType[*panicError](err); ok {
		s.logger.Error("job handler panicked", zap.Int64("job", jobID), zap.String("handler", h.Key),
			zap.Any("panic", pe.value), zap.ByteString("stack", pe.stack))
	}
	if s.runCtx.Err() != nil && err != nil {
		return statusFailed, "服务停止，执行被中断"
	}
	return classify(ctx, err, timeout)
}

func classify(ctx context.Context, err error, timeout time.Duration) (status, errText string) {
	switch {
	case err == nil:
		return statusSuccess, ""
	case isPanic(err):
		return statusFailed, err.Error()
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return statusTimeout, fmt.Sprintf("执行超时（%s）：%v", timeout, err)
	default:
		return statusFailed, err.Error()
	}
}

func isPanic(err error) bool {
	_, ok := errors.AsType[*panicError](err)
	return ok
}

func truncate(s string) string {
	if len(s) <= maxErrorLength {
		return s
	}
	cut := maxErrorLength
	for cut > 0 && !utf8Start(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
