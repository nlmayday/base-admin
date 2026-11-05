package ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// reloadChannel carries "jobs changed" notifications between instances.
const reloadChannel = "ops:jobs:changed"

// idleWait is how long the loop sleeps when no job is scheduled.
const idleWait = time.Minute

type entry struct {
	expr  string
	sched cron.Schedule
	next  time.Time
}

// scheduler fires enabled jobs. robfig/cron v3 parses the expressions; the
// dispatch loop is ours so that every fire knows its exact scheduled time,
// which is the cross-instance deduplication key. entries and fingerprint are
// only touched by the loop goroutine.
type scheduler struct {
	db       *gorm.DB
	rdb      *redis.Client
	handlers handlerSet
	logger   *zap.Logger
	instance string
	tuning   Tuning
	retry    retryPolicy

	entries     map[int64]*entry
	fingerprint string
	wake        chan struct{}
	// ready is closed once the loop has subscribed and loaded the jobs.
	ready     chan struct{}
	readyOnce sync.Once

	// mu guards stopping and the WaitGroup Add/Wait ordering.
	mu       sync.Mutex
	stopping bool
	stopCh   chan struct{}
	inflight sync.WaitGroup
	// runCtx is the parent of every handler attempt; shutdown cancels it
	// once ShutdownGrace has passed.
	runCtx     context.Context
	cancelRuns context.CancelFunc
}

func newScheduler(db *gorm.DB, rdb *redis.Client, hs handlerSet, logger *zap.Logger, instance string, t Tuning) *scheduler {
	runCtx, cancelRuns := context.WithCancel(context.Background())
	return &scheduler{
		runCtx: runCtx, cancelRuns: cancelRuns,
		db: db, rdb: rdb, handlers: hs, logger: logger, instance: instance, tuning: t,
		retry:   retryPolicy{base: t.RetryBaseDelay, max: t.RetryMaxDelay},
		entries: map[int64]*entry{},
		wake:    make(chan struct{}, 1),
		ready:   make(chan struct{}),
		stopCh:  make(chan struct{}),
	}
}

// track reserves a slot for an asynchronous run; ok is false once the
// scheduler is stopping. done must be called when the run ends.
func (s *scheduler) track() (done func(), ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping {
		return nil, false
	}
	s.inflight.Add(1)
	return s.inflight.Done, true
}

// shutdown stops new fires and retries, waits up to ShutdownGrace for
// running jobs, then cancels them. A cancelled attempt returns at once (a
// handler ignoring ctx is abandoned), so shutdown is bounded.
func (s *scheduler) shutdown() {
	s.mu.Lock()
	if !s.stopping {
		s.stopping = true
		close(s.stopCh)
	}
	s.mu.Unlock()
	done := make(chan struct{})
	go func() {
		s.inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(s.tuning.ShutdownGrace):
		s.logger.Warn("jobs still running at shutdown; cancelling them", zap.Duration("grace", s.tuning.ShutdownGrace))
		s.cancelRuns()
		<-done
	}
	s.cancelRuns()
}

// requestReload asks the loop to reload the job list soon.
func (s *scheduler) requestReload() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// notifyChanged tells every instance, including this one, to reload.
func (s *scheduler) notifyChanged(ctx context.Context) {
	s.requestReload()
	if err := s.rdb.Publish(ctx, reloadChannel, s.instance).Err(); err != nil {
		s.logger.Warn("publish job change failed; periodic resync will converge", zap.Error(err))
	}
}

func (s *scheduler) run(ctx context.Context) {
	defer s.shutdown()
	sub := s.rdb.Subscribe(ctx, reloadChannel)
	defer func() { _ = sub.Close() }()
	// Wait for the subscription to be confirmed so no change published
	// after startup is missed; a Redis outage is covered by resync.
	if _, err := sub.Receive(ctx); err != nil && ctx.Err() == nil {
		s.logger.Warn("subscribe to job changes failed; relying on periodic resync", zap.Error(err))
	}
	go func() {
		for range sub.Channel() {
			s.requestReload()
		}
	}()
	if err := s.reload(ctx); err != nil && ctx.Err() == nil {
		s.logger.Warn("initial job load failed", zap.Error(err))
	}
	s.readyOnce.Do(func() { close(s.ready) })
	resync := time.NewTicker(s.tuning.ResyncInterval)
	defer resync.Stop()
	for {
		timer := time.NewTimer(s.untilNext(time.Now()))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.fireDue(time.Now())
		case <-s.wake:
			timer.Stop()
			if err := s.reload(ctx); err != nil && ctx.Err() == nil {
				s.logger.Warn("job reload failed", zap.Error(err))
			}
		case <-resync.C:
			timer.Stop()
			s.resync(ctx)
		}
	}
}

func (s *scheduler) untilNext(now time.Time) time.Duration {
	wait := idleWait
	for _, e := range s.entries {
		if d := e.next.Sub(now); d < wait {
			wait = d
		}
	}
	return max(wait, 0)
}

// fireDue dispatches every entry whose fire time has come. Missed fires
// (e.g. after a long pause) collapse into one.
func (s *scheduler) fireDue(now time.Time) {
	for id, e := range s.entries {
		if e.next.After(now) {
			continue
		}
		fire := e.next
		e.next = e.sched.Next(now)
		if e.next.IsZero() {
			// The expression has no future fire time (robfig returns zero).
			s.logger.Warn("job has no further fire time; unscheduled", zap.Int64("job", id), zap.String("cron", e.expr))
			delete(s.entries, id)
		}
		if now.Sub(fire) > fireKeyTTL {
			// The process was paused for longer than the dedupe window: other
			// instances may have handled this fire and their key expired.
			s.logger.Warn("stale job fire skipped", zap.Int64("job", id), zap.Time("fire", fire))
			continue
		}
		done, ok := s.track()
		if !ok {
			return
		}
		go func() {
			defer done()
			s.fireCron(id, fire)
		}()
	}
}

type scheduledJob struct {
	ID       int64
	CronExpr string
}

// reload replaces the schedule with the enabled jobs in the database. Entries
// whose expression did not change keep their next fire time.
func (s *scheduler) reload(ctx context.Context) error {
	fp, err := s.loadFingerprint(ctx)
	if err != nil {
		return err
	}
	var rows []scheduledJob
	if err := s.db.WithContext(ctx).Raw("SELECT id, cron_expr FROM sys_jobs WHERE enabled").Scan(&rows).Error; err != nil {
		return fmt.Errorf("load jobs: %w", err)
	}
	now := time.Now()
	next := make(map[int64]*entry, len(rows))
	for _, r := range rows {
		if old, ok := s.entries[r.ID]; ok && old.expr == r.CronExpr {
			next[r.ID] = old
			continue
		}
		sched, err := parseCron(r.CronExpr)
		if err != nil {
			s.logger.Warn("job has an invalid cron expression", zap.Int64("job", r.ID), zap.Error(err))
			continue
		}
		nextFire := sched.Next(now)
		if nextFire.IsZero() {
			s.logger.Warn("job has no future fire time", zap.Int64("job", r.ID), zap.String("cron", r.CronExpr))
			continue
		}
		next[r.ID] = &entry{expr: r.CronExpr, sched: sched, next: nextFire}
	}
	s.entries = next
	s.fingerprint = fp
	return nil
}

// loadFingerprint summarizes the scheduling-relevant state: every change
// through the API bumps a job's version, deletes remove ids.
func (s *scheduler) loadFingerprint(ctx context.Context) (string, error) {
	var rows []struct{ ID, Version int64 }
	if err := s.db.WithContext(ctx).Raw(`SELECT id, version FROM sys_jobs ORDER BY id`).Scan(&rows).Error; err != nil {
		return "", fmt.Errorf("load job fingerprint: %w", err)
	}
	if len(rows) == 0 {
		return "", nil
	}
	parts := make([]string, len(rows))
	for i, r := range rows {
		parts[i] = strconv.FormatInt(r.ID, 10) + ":" + strconv.FormatInt(r.Version, 10)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, ",")))
	return hex.EncodeToString(sum[:]), nil
}

// resync reloads when the database changed without a notification reaching
// this instance, and fails runs whose instance died.
func (s *scheduler) resync(ctx context.Context) {
	fp, err := s.loadFingerprint(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("job resync failed", zap.Error(err))
		}
		return
	}
	if fp != s.fingerprint {
		if err := s.reload(ctx); err != nil && ctx.Err() == nil {
			s.logger.Warn("job reload failed", zap.Error(err))
		}
	}
	if err := s.failExpiredRuns(ctx); err != nil && ctx.Err() == nil {
		s.logger.Warn("expire stale runs failed", zap.Error(err))
	}
}
