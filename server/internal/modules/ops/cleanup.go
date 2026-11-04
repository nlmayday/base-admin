package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"base-admin/server/internal/core/jobs"
)

// CleanupRunsKey is the built-in handler deleting old run records.
const CleanupRunsKey = "ops.cleanup-runs"

const (
	defaultRetentionDays = 30
	maxRetentionDays     = 3650
	cleanupBatchSize     = 5000
)

type cleanupParams struct {
	Days *int `json:"days"`
}

// parseCleanupParams reads {"days": N}; empty params mean the default.
func parseCleanupParams(raw json.RawMessage) (int, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return defaultRetentionDays, nil
	}
	var p cleanupParams
	if err := json.Unmarshal(trimmed, &p); err != nil {
		return 0, fmt.Errorf("参数格式错误，应为 {\"days\": 天数}: %w", err)
	}
	if p.Days == nil {
		return defaultRetentionDays, nil
	}
	if *p.Days < 1 || *p.Days > maxRetentionDays {
		return 0, fmt.Errorf("days 必须在 1 到 %d 之间", maxRetentionDays)
	}
	return *p.Days, nil
}

func (m *Module) cleanupHandler() jobs.Handler {
	return jobs.Handler{
		Key:            CleanupRunsKey,
		Name:           "清理执行记录",
		Description:    `删除 N 天前已结束的任务执行记录。参数：{"days": 30}，默认 30 天`,
		DefaultTimeout: 10 * time.Minute,
		Run:            m.cleanupRuns,
	}
}

// cleanupRuns deletes finished runs older than the retention in batches so
// one run never holds a long transaction.
func (m *Module) cleanupRuns(ctx context.Context, params json.RawMessage) error {
	days, err := parseCleanupParams(params)
	if err != nil {
		return err
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	for {
		// The derived table lets MySQL limit a subquery of the table it
		// deletes from; PostgreSQL accepts the same statement.
		res := m.db.WithContext(ctx).Exec(`DELETE FROM sys_job_runs WHERE id IN (
			SELECT id FROM (SELECT id FROM sys_job_runs WHERE status <> 'running' AND started_at < ?
			ORDER BY id LIMIT ?) AS batch)`, cutoff, cleanupBatchSize)
		if res.Error != nil {
			return fmt.Errorf("delete old job runs: %w", res.Error)
		}
		if res.RowsAffected < cleanupBatchSize {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}
