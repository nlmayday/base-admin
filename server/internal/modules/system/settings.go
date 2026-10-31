package system

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
)

// settingsView is the typed settings plus an aggregate version: the sum of
// the underlying config versions, which grows with every change.
type settingsView struct {
	SiteName                string   `json:"siteName"`
	LogoURL                 string   `json:"logoUrl"`
	UploadAllowedExtensions []string `json:"uploadAllowedExtensions"`
	UploadMaxSizeMB         int      `json:"uploadMaxSizeMB"`
	AuditRetentionDays      int      `json:"auditRetentionDays"`
	Version                 int      `json:"version"`
}

type settingsRequest struct {
	SiteName                string   `json:"siteName"`
	LogoURL                 string   `json:"logoUrl"`
	UploadAllowedExtensions []string `json:"uploadAllowedExtensions" binding:"max=200"`
	UploadMaxSizeMB         int      `json:"uploadMaxSizeMB"`
	AuditRetentionDays      int      `json:"auditRetentionDays"`
	Version                 *int     `json:"version"`
}

func (r settingsRequest) settings() Settings {
	return Settings{SiteName: r.SiteName, LogoURL: r.LogoURL, UploadAllowedExtensions: r.UploadAllowedExtensions,
		UploadMaxSizeMB: r.UploadMaxSizeMB, AuditRetentionDays: r.AuditRetentionDays}
}

// PublicSettings is the only settings data exposed without a session.
type PublicSettings struct {
	SiteName string `json:"siteName"`
	LogoURL  string `json:"logoUrl"`
}

type settingRow struct {
	ID      int64
	Key     string
	Value   string
	Version int
}

// loadSettingRows reads the built-in setting rows by key; lock adds FOR
// UPDATE inside a transaction.
func loadSettingRows(tx *gorm.DB, lock bool) (map[string]settingRow, error) {
	sql := "SELECT id, key, value, version FROM sys_configs WHERE key IN ? ORDER BY id"
	if lock {
		sql += " FOR UPDATE"
	}
	var rows []settingRow
	if err := tx.Raw(sql, settingKeys()).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]settingRow, len(rows))
	for _, r := range rows {
		out[r.Key] = r
	}
	for _, key := range settingKeys() {
		if _, ok := out[key]; !ok {
			return nil, fmt.Errorf("built-in setting %s is missing; was the module seeded?", key)
		}
	}
	return out, nil
}

func buildSettingsView(rows map[string]settingRow) (settingsView, error) {
	values := make(map[string]string, len(rows))
	version := 0
	for k, r := range rows {
		values[k] = r.Value
		version += r.Version
	}
	s, err := settingsFromValues(values)
	if err != nil {
		return settingsView{}, err
	}
	return settingsView{SiteName: s.SiteName, LogoURL: s.LogoURL, UploadAllowedExtensions: s.UploadAllowedExtensions,
		UploadMaxSizeMB: s.UploadMaxSizeMB, AuditRetentionDays: s.AuditRetentionDays, Version: version}, nil
}

func (m *Module) getSettings(c *gin.Context) (any, error) {
	rows, err := loadSettingRows(m.db.WithContext(c.Request.Context()), false)
	if err != nil {
		return nil, err
	}
	return buildSettingsView(rows)
}

func (m *Module) publicSettings(c *gin.Context) (any, error) {
	ctx := c.Request.Context()
	name, err := m.GetString(ctx, KeySiteName)
	if err != nil {
		return nil, err
	}
	logo, err := m.GetString(ctx, KeySiteLogoURL)
	if err != nil {
		return nil, err
	}
	if validateLogoURL(logo) != "" {
		logo = ""
	}
	return PublicSettings{SiteName: name, LogoURL: logo}, nil
}

func (m *Module) updateSettings(c *gin.Context) (any, error) {
	var req settingsRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if req.Version == nil {
		return nil, fieldError("version", "不能为空")
	}
	next, fields := req.settings().normalized()
	if len(fields) > 0 {
		return nil, httpx.Validation(fields)
	}
	ctx := c.Request.Context()
	uid := actor.From(ctx).UserID()
	var view settingsView
	err := database.RunTx(ctx, m.db, func(tx *gorm.DB) error {
		rows, err := loadSettingRows(tx, true)
		if err != nil {
			return err
		}
		current, err := buildSettingsView(rows)
		if err != nil {
			return err
		}
		if current.Version != *req.Version {
			return errVersionConflict
		}
		if err := applySettingValues(tx, rows, next.storedValues(), uid); err != nil {
			return err
		}
		if rows, err = loadSettingRows(tx, false); err != nil {
			return err
		}
		view, err = buildSettingsView(rows)
		return err
	})
	if err != nil {
		return nil, err
	}
	m.values.clear()
	return view, nil
}

// applySettingValues writes changed values with a history row each.
func applySettingValues(tx *gorm.DB, rows map[string]settingRow, values map[string]string, uid int64) error {
	for _, key := range settingKeys() {
		row, next := rows[key], values[key]
		if row.Value == next {
			continue
		}
		err := tx.Exec(`UPDATE sys_configs SET value = ?, version = version + 1, updated_at = now(), updated_by = ?
			WHERE id = ?`, next, nullableID(uid), row.ID).Error
		if err != nil {
			return err
		}
		old := row.Value
		if err := insertChange(tx, row.ID, key, &old, next, uid); err != nil {
			return err
		}
	}
	return nil
}
