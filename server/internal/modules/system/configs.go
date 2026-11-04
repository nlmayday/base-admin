package system

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

const (
	tableConfigs       = "sys_configs"
	maxConfigKeyLen    = 128
	defaultConfigGroup = "default"
)

type configView struct {
	ID          ids.ID    `json:"id"`
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	ValueType   string    `json:"valueType"`
	GroupName   string    `json:"groupName"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Editable    bool      `json:"editable"`
	Builtin     bool      `json:"builtin"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Version     int       `json:"version"`
}

type configRow struct {
	ID          int64
	Key         string
	Value       string
	ValueType   string
	GroupName   string
	Name        string
	Description string
	Editable    bool
	Builtin     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int
}

func (r configRow) view() configView {
	return configView{ID: ids.ID(r.ID), Key: r.Key, Value: r.Value, ValueType: r.ValueType, GroupName: r.GroupName,
		Name: r.Name, Description: r.Description, Editable: r.Editable, Builtin: r.Builtin,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: r.Version}
}

// configFilter documents the list query.
type configFilter struct {
	Keyword  string `form:"keyword"`
	Group    string `form:"group"`
	Page     int    `form:"page"`
	PageSize int    `form:"pageSize"`
	// IncludeBuiltin also lists the built-in settings (read-only here; they
	// are edited in 基础设置). Hidden by default so the two pages do not look
	// like two places to change the same thing.
	IncludeBuiltin bool `form:"includeBuiltin" doc:"为 true 时同时列出内置参数（只读，在「基础设置」中修改）"`
}

// includeBuiltin reads the includeBuiltin query flag (default false).
func includeBuiltin(c *gin.Context) bool {
	v, err := strconv.ParseBool(c.DefaultQuery("includeBuiltin", "false"))
	return err == nil && v
}

func (m *Module) listConfigs(c *gin.Context) (any, error) {
	pq, err := httpx.ParsePage(c)
	if err != nil {
		return nil, err
	}
	q := m.db.WithContext(c.Request.Context()).Table(tableConfigs)
	if !includeBuiltin(c) {
		q = q.Where("NOT builtin")
	}
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + escapeLike(kw) + "%"
		q = q.Where(`LOWER("key") LIKE LOWER(?) OR LOWER(name) LIKE LOWER(?)`, like, like)
	}
	if g := strings.TrimSpace(c.Query("group")); g != "" {
		q = q.Where("group_name = ?", g)
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []configRow
	if err := q.Order(`group_name, "key"`).Offset(pq.Offset()).Limit(pq.PageSize).Scan(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]configView, len(rows))
	for i, r := range rows {
		views[i] = r.view()
	}
	return httpx.Page[configView]{List: views, Total: total, Page: pq.Page, PageSize: pq.PageSize}, nil
}

func (m *Module) configGroups(c *gin.Context) (any, error) {
	groups := []string{}
	q := m.db.WithContext(c.Request.Context()).Table(tableConfigs).Distinct("group_name").Order("group_name")
	if !includeBuiltin(c) {
		q = q.Where("NOT builtin")
	}
	err := q.Pluck("group_name", &groups).Error
	return groups, err
}

// loadConfig reads one config; lock adds FOR UPDATE inside a transaction.
func loadConfig(tx *gorm.DB, id int64, lock bool) (configRow, error) {
	sql := "SELECT * FROM sys_configs WHERE id = ?"
	if lock {
		sql += " FOR UPDATE"
	}
	var rows []configRow
	if err := tx.Raw(sql, id).Scan(&rows).Error; err != nil {
		return configRow{}, err
	}
	if len(rows) == 0 {
		return configRow{}, httpx.NotFound("参数不存在")
	}
	return rows[0], nil
}

type configRequest struct {
	// Key is set on create only; it is immutable afterwards.
	Key         string `json:"key"`
	Value       string `json:"value"`
	ValueType   string `json:"valueType"`
	GroupName   string `json:"groupName" binding:"max=64"`
	Name        string `json:"name" binding:"required,max=64"`
	Description string `json:"description" binding:"max=255"`
	Version     *int   `json:"version"`
}

// normalize validates the request and stores the normalized value.
func (r *configRequest) normalize() error {
	r.Name = strings.TrimSpace(r.Name)
	r.GroupName = strings.TrimSpace(r.GroupName)
	if r.GroupName == "" {
		r.GroupName = defaultConfigGroup
	}
	if r.ValueType == "" {
		r.ValueType = TypeString
	}
	var fields []httpx.FieldError
	if r.Name == "" {
		fields = append(fields, httpx.FieldError{Field: "name", Message: "不能为空"})
	}
	if !validValueType(r.ValueType) {
		fields = append(fields, httpx.FieldError{Field: "valueType", Message: "取值必须是以下之一：string number boolean json"})
	} else if v, msg := normalizeValue(r.ValueType, r.Value); msg != "" {
		fields = append(fields, httpx.FieldError{Field: "value", Message: msg})
	} else {
		r.Value = v
	}
	if len(fields) > 0 {
		return httpx.Validation(fields)
	}
	return nil
}

func validConfigKey(key string) bool {
	return utf8.RuneCountInString(key) <= maxConfigKeyLen && configKeyPattern.MatchString(key)
}

func (m *Module) createConfig(c *gin.Context) (any, error) {
	var req configRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if !validConfigKey(req.Key) {
		return nil, fieldError("key", "参数键须为小写字母开头，由小写字母、数字、下划线和点组成，最长 128 位")
	}
	if err := req.normalize(); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	uid := actor.From(ctx).UserID()
	var id int64
	err := database.RunTx(ctx, m.db, func(tx *gorm.DB) error {
		var err error
		id, err = database.InsertID(tx, `INSERT INTO sys_configs ("key", value, value_type, group_name, name, description, created_by, updated_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			req.Key, req.Value, req.ValueType, req.GroupName, req.Name, req.Description, uid, uid)
		if err != nil {
			if database.IsUniqueViolation(err) {
				return fieldError("key", "参数键已存在")
			}
			return err
		}
		return insertChange(tx, id, req.Key, nil, req.Value, uid)
	})
	if err != nil {
		return nil, err
	}
	m.values.clear()
	row, err := loadConfig(m.db.WithContext(ctx), id, false)
	return row.view(), err
}

func (m *Module) updateConfig(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var req configRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if req.Version == nil {
		return nil, fieldError("version", "不能为空")
	}
	ctx := c.Request.Context()
	uid := actor.From(ctx).UserID()
	err = database.RunTx(ctx, m.db, func(tx *gorm.DB) error {
		current, err := loadConfig(tx, id, true)
		if err != nil {
			return err
		}
		if err := checkWritable(current, "修改"); err != nil {
			return err
		}
		if err := req.normalize(); err != nil {
			return err
		}
		err = updateVersioned(tx, tableConfigs, id, *req.Version, map[string]any{
			"value": req.Value, "value_type": req.ValueType, "group_name": req.GroupName,
			"name": req.Name, "description": req.Description, "updated_by": uid,
		}, nil)
		if err != nil || current.Value == req.Value {
			return err
		}
		return insertChange(tx, id, current.Key, &current.Value, req.Value, uid)
	})
	if err != nil {
		return nil, err
	}
	m.values.clear()
	row, err := loadConfig(m.db.WithContext(ctx), id, false)
	return row.view(), err
}

func (m *Module) deleteConfig(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	err = database.RunTx(c.Request.Context(), m.db, func(tx *gorm.DB) error {
		current, err := loadConfig(tx, id, true)
		if err != nil {
			return err
		}
		if err := checkWritable(current, "删除"); err != nil {
			return err
		}
		return tx.Exec("DELETE FROM sys_configs WHERE id = ?", id).Error
	})
	if err != nil {
		return nil, err
	}
	m.values.clear()
	return nil, nil
}

// checkWritable protects built-in settings (managed through /settings with
// typed validation) and read-only configs.
func checkWritable(row configRow, verb string) error {
	if row.Builtin {
		return protected("内置参数不能在此" + verb + "，请前往「基础设置」")
	}
	if !row.Editable {
		return protected("该参数为只读参数，不能" + verb)
	}
	return nil
}
