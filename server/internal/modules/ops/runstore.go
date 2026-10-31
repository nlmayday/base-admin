package ops

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// dbTimeout bounds the scheduler's own database and Redis calls.
const dbTimeout = 10 * time.Second

// jobRow is one sys_jobs row as the scheduler and API read it.
type jobRow struct {
	ID             int64
	Name           string
	HandlerKey     string
	CronExpr       string
	Params         string
	Enabled        bool
	TimeoutSeconds int
	MaxRetries     int
	Remark         string
	Version        int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastRunAt      *time.Time
	LastStatus     string
}

const jobColumns = `id, name, handler_key, cron_expr, params::text AS params, enabled, timeout_seconds,
	max_retries, remark, version, created_at, updated_at, last_run_at, last_status`

func (s *scheduler) loadJob(ctx context.Context, id int64) (jobRow, bool, error) {
	var rows []jobRow
	if err := s.db.WithContext(ctx).Raw("SELECT "+jobColumns+" FROM sys_jobs WHERE id = ?", id).Scan(&rows).Error; err != nil {
		return jobRow{}, false, fmt.Errorf("load job: %w", err)
	}
	if len(rows) == 0 {
		return jobRow{}, false, nil
	}
	return rows[0], true, nil
}

// runRecord describes a run row to insert.
type runRecord struct {
	job         jobRow
	trigger     string
	status      string
	scheduledAt *time.Time
	triggeredBy *int64
	instance    string
	errText     string
	expiresAt   time.Time
}

// insertRunning records a started run and marks the job as running in one
// statement, so readers never see one without the other.
func (s *scheduler) insertRunning(ctx context.Context, r runRecord) (int64, error) {
	var id int64
	err := s.db.WithContext(ctx).Raw(`WITH run AS (
		INSERT INTO sys_job_runs (job_id, job_name, handler_key, trigger, status, attempt, scheduled_at, instance,
			triggered_by, expires_at)
		VALUES (?, ?, ?, ?, 'running', 1, ?, ?, ?, ?) RETURNING id, job_id, started_at
	), job AS (
		UPDATE sys_jobs SET last_run_at = (SELECT started_at FROM run), last_status = 'running'
		WHERE id = (SELECT job_id FROM run)
	)
	SELECT id FROM run`,
		r.job.ID, r.job.Name, r.job.HandlerKey, r.trigger, r.scheduledAt, r.instance, r.triggeredBy, r.expiresAt).Scan(&id).Error
	if err != nil {
		return 0, fmt.Errorf("insert job run: %w", err)
	}
	return id, nil
}

// insertFinishedRun records a run that never executed (skipped, or its
// handler is missing). The job's last status is left alone.
func (s *scheduler) insertFinishedRun(ctx context.Context, r runRecord) (int64, error) {
	var id int64
	err := s.db.WithContext(ctx).Raw(`INSERT INTO sys_job_runs
		(job_id, job_name, handler_key, trigger, status, attempt, scheduled_at, instance, triggered_by,
		 finished_at, duration_ms, error, expires_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, now(), 0, ?, now()) RETURNING id`,
		r.job.ID, r.job.Name, r.job.HandlerKey, r.trigger, r.status, r.scheduledAt, r.instance, r.triggeredBy, r.errText).Scan(&id).Error
	if err != nil {
		return 0, fmt.Errorf("insert job run: %w", err)
	}
	return id, nil
}

func (s *scheduler) markAttempt(runID int64, attempt int) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	if err := s.db.WithContext(ctx).Exec("UPDATE sys_job_runs SET attempt = ? WHERE id = ?", attempt, runID).Error; err != nil {
		s.logger.Warn("update run attempt failed", zap.Int64("run", runID), zap.Error(err))
	}
}

// finishRun stores the outcome of a run and the job's last status in one
// statement. It runs while the job lock is still held.
func (s *scheduler) finishRun(runID, jobID int64, status, errText string, attempts int, took time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	err := s.db.WithContext(ctx).Exec(`WITH run AS (
		UPDATE sys_job_runs SET status = ?, error = ?, attempt = ?, finished_at = now(), duration_ms = ?
		WHERE id = ? RETURNING job_id
	)
	UPDATE sys_jobs SET last_status = ? WHERE id = (SELECT job_id FROM run)`,
		status, errText, attempts, took.Milliseconds(), runID, status).Error
	if err != nil {
		s.logger.Error("record job run result failed", zap.Int64("run", runID), zap.String("status", status), zap.Error(err))
	}
	if status != statusSuccess {
		s.logger.Warn("job run did not succeed", zap.Int64("job", jobID), zap.Int64("run", runID),
			zap.String("status", status), zap.Int("attempts", attempts), zap.String("error", errText))
	}
}

// failExpiredRuns marks runs whose instance died while running as failed.
func (s *scheduler) failExpiredRuns(ctx context.Context) error {
	db := s.db.WithContext(ctx)
	if err := db.Exec(`UPDATE sys_job_runs SET status = 'failed', finished_at = now(),
		error = '执行实例已中断，未上报结果' WHERE status = 'running' AND expires_at < now()`).Error; err != nil {
		return fmt.Errorf("fail expired runs: %w", err)
	}
	return db.Exec(`UPDATE sys_jobs j SET last_status = 'failed' WHERE last_status = 'running'
		AND NOT EXISTS (SELECT 1 FROM sys_job_runs r WHERE r.job_id = j.id AND r.status = 'running')`).Error
}
