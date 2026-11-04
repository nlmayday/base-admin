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

const tableDictItems = "sys_dict_items"

type dictItemView struct {
	ID        ids.ID    `json:"id"`
	TypeID    ids.ID    `json:"typeId"`
	Label     string    `json:"label"`
	Value     string    `json:"value"`
	Sort      int       `json:"sort"`
	Status    int16     `json:"status"`
	TagType   string    `json:"tagType"`
	Remark    string    `json:"remark"`
	CreatedAt time.Time `json:"createdAt"`
	Version   int       `json:"version"`
}

type dictItemRow struct {
	ID        int64
	TypeID    int64
	TypeCode  string
	Label     string
	Value     string
	Sort      int
	Status    int16
	TagType   string
	Remark    string
	CreatedAt time.Time
	Version   int
}

func (r dictItemRow) view() dictItemView {
	return dictItemView{ID: ids.ID(r.ID), TypeID: ids.ID(r.TypeID), Label: r.Label, Value: r.Value, Sort: r.Sort,
		Status: r.Status, TagType: r.TagType, Remark: r.Remark, CreatedAt: r.CreatedAt, Version: r.Version}
}

// dictItemFilter documents the item list query.
type dictItemFilter struct {
	Keyword string `form:"keyword"`
	Status  int16  `form:"status"`
}

const dictItemColumns = `i.id, i.type_id, t.code AS type_code, i.label, i.value, i.sort, i.status, i.tag_type,
	i.remark, i.created_at, i.version`

// maxItemsPerType bounds the unpaged item list.
const maxItemsPerType = 500

func (m *Module) listDictItems(c *gin.Context) (any, error) {
	typeID, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	status, err := optionalStatus(c)
	if err != nil {
		return nil, err
	}
	db := m.db.WithContext(c.Request.Context())
	if _, err := loadDictType(db, typeID); err != nil {
		return nil, err
	}
	q := db.Table("sys_dict_items AS i").Joins("JOIN sys_dict_types t ON t.id = i.type_id").
		Select(dictItemColumns).Where("i.type_id = ?", typeID)
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + escapeLike(kw) + "%"
		q = q.Where("LOWER(i.label) LIKE LOWER(?) OR LOWER(i.value) LIKE LOWER(?)", like, like)
	}
	if status != 0 {
		q = q.Where("i.status = ?", status)
	}
	var rows []dictItemRow
	if err := q.Order("i.sort, i.id").Limit(maxItemsPerType).Scan(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]dictItemView, len(rows))
	for i, r := range rows {
		views[i] = r.view()
	}
	return views, nil
}

func loadDictItem(tx *gorm.DB, id int64) (dictItemRow, error) {
	var rows []dictItemRow
	if err := tx.Table("sys_dict_items AS i").Joins("JOIN sys_dict_types t ON t.id = i.type_id").
		Select(dictItemColumns).Where("i.id = ?", id).Scan(&rows).Error; err != nil {
		return dictItemRow{}, err
	}
	if len(rows) == 0 {
		return dictItemRow{}, httpx.NotFound("字典项不存在")
	}
	return rows[0], nil
}

type dictItemRequest struct {
	Label   string `json:"label" binding:"required,max=64"`
	Value   string `json:"value" binding:"required,max=64"`
	Sort    int    `json:"sort" binding:"min=0,max=99999"`
	Status  int16  `json:"status" binding:"omitempty,oneof=1 2"`
	TagType string `json:"tagType" binding:"omitempty,oneof=primary success info warning danger"`
	Remark  string `json:"remark" binding:"max=255"`
	Version *int   `json:"version"`
}

func (r *dictItemRequest) normalize() error {
	r.Label = strings.TrimSpace(r.Label)
	r.Value = strings.TrimSpace(r.Value)
	if r.Label == "" {
		return fieldError("label", "不能为空")
	}
	if r.Value == "" {
		return fieldError("value", "不能为空")
	}
	if r.Status == 0 {
		r.Status = actor.StatusEnabled
	}
	return nil
}

var errDuplicateItemValue = fieldError("value", "该字典下已存在相同的字典值")

func (m *Module) createDictItem(c *gin.Context) (any, error) {
	typeID, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var req dictItemRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if err := req.normalize(); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	db := m.db.WithContext(ctx)
	dictType, err := loadDictType(db, typeID)
	if err != nil {
		return nil, err
	}
	uid := actor.From(ctx).UserID()
	id, err := database.InsertID(db, `INSERT INTO sys_dict_items (type_id, label, value, sort, status, tag_type, remark, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		typeID, req.Label, req.Value, req.Sort, req.Status, req.TagType, req.Remark, uid, uid)
	if err != nil {
		switch {
		case database.IsUniqueViolation(err):
			return nil, errDuplicateItemValue
		case database.IsForeignKeyViolation(err):
			return nil, httpx.NotFound("字典类型不存在")
		}
		return nil, err
	}
	m.invalidateDict(ctx, dictType.Code)
	row, err := loadDictItem(db, id)
	return row.view(), err
}

func (m *Module) updateDictItem(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var req dictItemRequest
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
	current, err := loadDictItem(db, id)
	if err != nil {
		return nil, err
	}
	err = updateVersioned(db, tableDictItems, id, *req.Version, map[string]any{
		"label": req.Label, "value": req.Value, "sort": req.Sort, "status": req.Status,
		"tag_type": req.TagType, "remark": req.Remark, "updated_by": actor.From(ctx).UserID(),
	}, errDuplicateItemValue)
	if err != nil {
		return nil, err
	}
	m.invalidateDict(ctx, current.TypeCode)
	row, err := loadDictItem(db, id)
	return row.view(), err
}

func (m *Module) deleteDictItem(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	db := m.db.WithContext(ctx)
	current, err := loadDictItem(db, id)
	if err != nil {
		return nil, err
	}
	if err := db.Exec("DELETE FROM sys_dict_items WHERE id = ?", id).Error; err != nil {
		return nil, err
	}
	m.invalidateDict(ctx, current.TypeCode)
	return nil, nil
}
