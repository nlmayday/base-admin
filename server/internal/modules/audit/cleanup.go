package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
)

const (
	cleanupJobKey     = "audit.cleanup"
	defaultRetainDays = 180
	// minRetainDays is a floor no job parameter or setting can go below, so
	// recent evidence cannot be purged through the job API.
	minRetainDays      = 7
	maxRetainDays      = 36500
	cleanupBatchSize   = 5000
	cleanupParamsField = "days"
)

type cleanupParams struct {
	Days int `json:"days"`
}

// parseCleanupParams reads the job params; fallback is used when they do not
// set days.
func parseCleanupParams(raw json.RawMessage, fallback int) (cleanupParams, error) {
	p := cleanupParams{Days: fallback}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) != 0 && !bytes.Equal(trimmed, []byte("null")) {
		if err := json.Unmarshal(trimmed, &p); err != nil {
			return cleanupParams{}, fmt.Errorf("invalid params: %w", err)
		}
	}
	if p.Days < minRetainDays || p.Days > maxRetainDays {
		return cleanupParams{}, fmt.Errorf("%s must be between %d and %d, got %d", cleanupParamsField, minRetainDays, maxRetainDays, p.Days)
	}
	return p, nil
}

// cleanup deletes login and operation logs older than params.days, in
// batches so no single statement holds locks for long.
func (m *Module) cleanup(ctx context.Context, raw json.RawMessage) error {
	p, err := parseCleanupParams(raw, m.retentionDays(ctx))
	if err != nil {
		return err
	}
	cutoff := m.now().AddDate(0, 0, -p.Days)
	logins, err := m.deleteBefore(ctx, tableLoginLogs, cutoff)
	if err != nil {
		return err
	}
	ops, err := m.deleteBefore(ctx, tableOperationLogs, cutoff)
	if err != nil {
		return err
	}
	m.logger.Info("audit logs cleaned up", zap.Int("days", p.Days), zap.Time("cutoff", cutoff),
		zap.Int64("loginLogs", logins), zap.Int64("operationLogs", ops))
	return nil
}

// retentionDays is the configured default window, falling back to
// defaultRetainDays when no provider is wired or it fails.
func (m *Module) retentionDays(ctx context.Context) int {
	if m.retention == nil {
		return defaultRetainDays
	}
	days, err := m.retention(ctx)
	if err != nil || days < minRetainDays || days > maxRetainDays {
		m.logger.Warn("audit retention setting unusable, using default",
			zap.Int("days", days), zap.Error(err), zap.Int("default", defaultRetainDays))
		return defaultRetainDays
	}
	return days
}

// deleteBefore removes rows of table (a constant) created before cutoff.
func (m *Module) deleteBefore(ctx context.Context, table string, cutoff time.Time) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		// The derived table lets MySQL limit a subquery of the table it
		// deletes from; PostgreSQL accepts the same statement.
		res := m.db.WithContext(ctx).Exec(`DELETE FROM `+table+` WHERE id IN (
			SELECT id FROM (SELECT id FROM `+table+` WHERE created_at < ? LIMIT ?) AS batch)`, cutoff, cleanupBatchSize)
		if res.Error != nil {
			return total, fmt.Errorf("clean up %s: %w", table, res.Error)
		}
		total += res.RowsAffected
		if res.RowsAffected < cleanupBatchSize {
			return total, nil
		}
	}
}
