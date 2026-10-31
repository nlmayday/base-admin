package system

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

type configChangeView struct {
	ID ids.ID `json:"id"`
	// OldValue is null for the creation of the config.
	OldValue      *string   `json:"oldValue"`
	NewValue      string    `json:"newValue"`
	ChangedBy     *ids.ID   `json:"changedBy"`
	ChangedByName string    `json:"changedByName"`
	ChangedAt     time.Time `json:"changedAt"`
}

// nullableID maps the zero id (no actor) to SQL NULL.
func nullableID(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}

type configChangeRow struct {
	ID            int64
	OldValue      *string
	NewValue      string
	ChangedBy     *int64
	ChangedByName string
	ChangedAt     time.Time
}

// insertChange records a value change in the caller's transaction.
func insertChange(tx *gorm.DB, configID int64, key string, oldValue *string, newValue string, uid int64) error {
	return tx.Exec(`INSERT INTO sys_config_changes (config_id, key, old_value, new_value, changed_by)
		VALUES (?, ?, ?, ?, ?)`, configID, key, oldValue, newValue, nullableID(uid)).Error
}

func (m *Module) listConfigChanges(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	pq, err := httpx.ParsePage(c)
	if err != nil {
		return nil, err
	}
	db := m.db.WithContext(c.Request.Context())
	if _, err := loadConfig(db, id, false); err != nil {
		return nil, err
	}
	var total int64
	if err := db.Raw("SELECT count(*) FROM sys_config_changes WHERE config_id = ?", id).Scan(&total).Error; err != nil {
		return nil, err
	}
	var rows []configChangeRow
	err = db.Raw(`SELECT ch.id, ch.old_value, ch.new_value, ch.changed_by, ch.changed_at,
			COALESCE(NULLIF(u.nickname, ''), u.username, '') AS changed_by_name
		FROM sys_config_changes ch LEFT JOIN sys_users u ON u.id = ch.changed_by
		WHERE ch.config_id = ? ORDER BY ch.changed_at DESC, ch.id DESC LIMIT ? OFFSET ?`,
		id, pq.PageSize, pq.Offset()).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	views := make([]configChangeView, len(rows))
	for i, r := range rows {
		views[i] = configChangeView{ID: ids.ID(r.ID), OldValue: r.OldValue, NewValue: r.NewValue,
			ChangedByName: r.ChangedByName, ChangedAt: r.ChangedAt}
		if r.ChangedBy != nil {
			by := ids.ID(*r.ChangedBy)
			views[i].ChangedBy = &by
		}
	}
	return httpx.Page[configChangeView]{List: views, Total: total, Page: pq.Page, PageSize: pq.PageSize}, nil
}
