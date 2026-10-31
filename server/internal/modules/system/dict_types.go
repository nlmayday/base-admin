package system

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

const tableDictTypes = "sys_dict_types"

type dictTypeView struct {
	ID        ids.ID    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Status    int16     `json:"status"`
	Remark    string    `json:"remark"`
	ItemCount int       `json:"itemCount"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Version   int       `json:"version"`
}

type dictTypeRow struct {
	ID        int64
	Code      string
	Name      string
	Status    int16
	Remark    string
	ItemCount int
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
}

func (r dictTypeRow) view() dictTypeView {
	return dictTypeView{ID: ids.ID(r.ID), Code: r.Code, Name: r.Name, Status: r.Status, Remark: r.Remark,
		ItemCount: r.ItemCount, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: r.Version}
}

// dictTypeFilter documents the list query.
type dictTypeFilter struct {
	Keyword  string `form:"keyword"`
	Status   int16  `form:"status"`
	Page     int    `form:"page"`
	PageSize int    `form:"pageSize"`
}

const dictTypeColumns = `t.id, t.code, t.name, t.status, t.remark, t.created_at, t.updated_at, t.version,
	(SELECT count(*) FROM sys_dict_items i WHERE i.type_id = t.id) AS item_count`

func (m *Module) listDictTypes(c *gin.Context) (any, error) {
	pq, err := httpx.ParsePage(c)
	if err != nil {
		return nil, err
	}
	status, err := optionalStatus(c)
	if err != nil {
		return nil, err
	}
	q := m.db.WithContext(c.Request.Context()).Table("sys_dict_types AS t")
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + escapeLike(kw) + "%"
		q = q.Where("t.code ILIKE ? OR t.name ILIKE ?", like, like)
	}
	if status != 0 {
		q = q.Where("t.status = ?", status)
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []dictTypeRow
	if err := q.Select(dictTypeColumns).Order("t.id DESC").Offset(pq.Offset()).Limit(pq.PageSize).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]dictTypeView, len(rows))
	for i, r := range rows {
		views[i] = r.view()
	}
	return httpx.Page[dictTypeView]{List: views, Total: total, Page: pq.Page, PageSize: pq.PageSize}, nil
}

func loadDictType(tx *gorm.DB, id int64) (dictTypeRow, error) {
	var rows []dictTypeRow
	if err := tx.Table("sys_dict_types AS t").Select(dictTypeColumns).Where("t.id = ?", id).Scan(&rows).Error; err != nil {
		return dictTypeRow{}, err
	}
	if len(rows) == 0 {
		return dictTypeRow{}, httpx.NotFound("字典类型不存在")
	}
	return rows[0], nil
}

type dictTypeRequest struct {
	// Code is set on create only; it is immutable afterwards.
	Code    string `json:"code"`
	Name    string `json:"name" binding:"required,max=64"`
	Status  int16  `json:"status" binding:"omitempty,oneof=1 2"`
	Remark  string `json:"remark" binding:"max=255"`
	Version *int   `json:"version"`
}

func (r *dictTypeRequest) normalize() error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return fieldError("name", "不能为空")
	}
	if r.Status == 0 {
		r.Status = actor.StatusEnabled
	}
	return nil
}

func (m *Module) createDictType(c *gin.Context) (any, error) {
	var req dictTypeRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if !dictCodePattern.MatchString(req.Code) {
		return nil, fieldError("code", "编码须为小写字母开头的 2–64 位小写字母、数字或下划线")
	}
	if err := req.normalize(); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	uid := actor.From(ctx).UserID()
	var id int64
	res := m.db.WithContext(ctx).Raw(`INSERT INTO sys_dict_types (code, name, status, remark, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id`, req.Code, req.Name, req.Status, req.Remark, uid, uid).Scan(&id)
	if res.Error != nil {
		if database.IsUniqueViolation(res.Error) {
			return nil, fieldError("code", "字典编码已存在")
		}
		return nil, res.Error
	}
	m.invalidateDict(ctx, req.Code)
	row, err := loadDictType(m.db.WithContext(ctx), id)
	return row.view(), err
}

func (m *Module) updateDictType(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var req dictTypeRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if req.Version == nil {
		return nil, fieldError("version", "不能为空")
	}
	if err := req.normalize(); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	db := m.db.WithContext(ctx)
	current, err := loadDictType(db, id)
	if err != nil {
		return nil, err
	}
	err = updateVersioned(db, tableDictTypes, id, *req.Version, map[string]any{
		"name": req.Name, "status": req.Status, "remark": req.Remark, "updated_by": actor.From(ctx).UserID(),
	}, nil)
	if err != nil {
		return nil, err
	}
	m.invalidateDict(ctx, current.Code)
	row, err := loadDictType(db, id)
	return row.view(), err
}

func (m *Module) deleteDictType(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	var code string
	err = database.RunTx(ctx, m.db, func(tx *gorm.DB) error {
		row, err := loadDictType(tx, id)
		if err != nil {
			return err
		}
		code = row.Code
		// Items are removed by ON DELETE CASCADE.
		return tx.Exec("DELETE FROM sys_dict_types WHERE id = ?", id).Error
	})
	if err != nil {
		return nil, err
	}
	m.invalidateDict(ctx, code)
	return nil, nil
}
