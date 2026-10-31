package file

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"base-admin/server/internal/core/jobs"
)

const (
	purgeJobKey = "file.purge-deleted"
	// purgeGrace leaves recent deletions to the request that made them.
	purgeGrace     = 10 * time.Minute
	purgeBatchSize = 200
)

var _ jobs.Provider = (*Module)(nil)

// Jobs implements jobs.Provider.
func (m *Module) Jobs() []jobs.Handler {
	return []jobs.Handler{{
		Key:            purgeJobKey,
		Name:           "清理已删除文件",
		Description:    "删除已标记删除但存储对象尚未清除的文件（删除时存储暂时不可用等情况）",
		DefaultTimeout: 30 * time.Minute,
		Run:            m.purgeDeleted,
	}}
}

type deletedFile struct {
	ID        int64
	ObjectKey string
}

// purgeDeleted finishes deletions whose object could not be removed at the
// time. Rows whose object still fails are left for the next run.
func (m *Module) purgeDeleted(ctx context.Context, _ json.RawMessage) error {
	var lastID int64
	purged, failed := 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var batch []deletedFile
		err := m.db.WithContext(ctx).Raw(`SELECT id, object_key FROM sys_files
			WHERE deleted_at IS NOT NULL AND deleted_at < ? AND id > ? ORDER BY id LIMIT ?`,
			m.now().Add(-purgeGrace), lastID, purgeBatchSize).Scan(&batch).Error
		if err != nil {
			return fmt.Errorf("list deleted files: %w", err)
		}
		for _, f := range batch {
			lastID = f.ID
			if err := m.purge(ctx, f.ID, f.ObjectKey); err != nil {
				failed++
				m.log.Warn("purge deleted file failed", zap.Int64("file", f.ID), zap.Error(err))
				continue
			}
			purged++
		}
		if len(batch) < purgeBatchSize {
			break
		}
	}
	m.log.Info("deleted files purged", zap.Int("purged", purged), zap.Int("failed", failed))
	if failed > 0 {
		return fmt.Errorf("%d deleted files could not be purged; they are retried next run", failed)
	}
	return nil
}
