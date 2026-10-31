package iam

import (
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

type positionView struct {
	ID      ids.ID `json:"id"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	Sort    int    `json:"sort"`
	Status  int16  `json:"status"`
	Remark  string `json:"remark"`
	Version int    `json:"version"`
}

type positionRow struct {
	ID      int64
	Code    string
	Name    string
	Sort    int
	Status  int16
	Remark  string
	Version int
}

func (r positionRow) view() positionView {
	return positionView{ID: ids.ID(r.ID), Code: r.Code, Name: r.Name, Sort: r.Sort, Status: r.Status, Remark: r.Remark, Version: r.Version}
}

func (m *Module) listPositions(c *gin.Context) (any, error) {
	pq, err := httpx.ParsePage(c)
	if err != nil {
		return nil, err
	}
	q := m.db.WithContext(c.Request.Context()).Table("sys_positions")
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + escapeLike(kw) + "%"
		q = q.Where("code ILIKE ? OR name ILIKE ?", like, like)
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []positionRow
	if err := q.Order("sort, id").Offset(pq.Offset()).Limit(pq.PageSize).Scan(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]positionView, len(rows))
	for i, r := range rows {
		views[i] = r.view()
	}
	return httpx.Page[positionView]{List: views, Total: total, Page: pq.Page, PageSize: pq.PageSize}, nil
}

func (m *Module) positionOptions(c *gin.Context) (any, error) {
	return loadOptions(c.Request.Context(), m.db, "sys_positions")
}

type positionRequest struct {
	Code    string `json:"code"`
	Name    string `json:"name" binding:"required,max=64"`
	Sort    int    `json:"sort"`
	Status  int16  `json:"status" binding:"omitempty,oneof=1 2"`
	Remark  string `json:"remark" binding:"max=255"`
	Version *int   `json:"version"`
}

func (m *Module) loadPosition(tx *gorm.DB, id int64) (positionView, error) {
	var rows []positionRow
	if err := tx.Table("sys_positions").Where("id = ?", id).Scan(&rows).Error; err != nil {
		return positionView{}, err
	}
	if len(rows) == 0 {
		return positionView{}, httpx.NotFound("岗位不存在")
	}
	return rows[0].view(), nil
}

func (m *Module) createPosition(c *gin.Context) (any, error) {
	var req positionRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if !codePattern.MatchString(req.Code) {
		return nil, fieldError("code", "编码须为小写字母开头的 2–32 位小写字母、数字或下划线")
	}
	if req.Status == 0 {
		req.Status = actor.StatusEnabled
	}
	ctx := c.Request.Context()
	uid := actor.From(ctx).UserID()
	var id int64
	res := m.db.WithContext(ctx).Raw(`INSERT INTO sys_positions (code, name, sort, status, remark, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`, req.Code, req.Name, req.Sort, req.Status, req.Remark, uid, uid).Scan(&id)
	if res.Error != nil {
		if database.IsUniqueViolation(res.Error) {
			return nil, fieldError("code", "岗位编码已存在")
		}
		return nil, res.Error
	}
	return m.loadPosition(m.db.WithContext(ctx), id)
}

func (m *Module) updatePosition(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var req positionRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if req.Version == nil {
		return nil, fieldError("version", "不能为空")
	}
	if req.Status == 0 {
		req.Status = actor.StatusEnabled
	}
	ctx := c.Request.Context()
	db := m.db.WithContext(ctx)
	if _, err := m.loadPosition(db, id); err != nil {
		return nil, err
	}
	err = updateVersioned(db, "sys_positions", id, *req.Version, map[string]any{
		"name": req.Name, "sort": req.Sort, "status": req.Status, "remark": req.Remark,
		"updated_by": actor.From(ctx).UserID(),
	}, "")
	if err != nil {
		return nil, err
	}
	return m.loadPosition(db, id)
}

func (m *Module) deletePosition(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	return nil, database.RunTx(c.Request.Context(), m.db, func(tx *gorm.DB) error {
		if _, err := m.loadPosition(tx, id); err != nil {
			return err
		}
		assigned, err := exists(tx, "SELECT 1 FROM sys_user_positions WHERE position_id = ?", id)
		if err != nil {
			return err
		}
		if assigned {
			return inUse("岗位已分配给用户，不能删除")
		}
		return tx.Exec("DELETE FROM sys_positions WHERE id = ?", id).Error
	})
}
