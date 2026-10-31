package iam

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

type deptView struct {
	ID           ids.ID      `json:"id"`
	ParentID     *ids.ID     `json:"parentId"`
	Name         string      `json:"name"`
	LeaderUserID *ids.ID     `json:"leaderUserId"`
	Sort         int         `json:"sort"`
	Status       int16       `json:"status"`
	Version      int         `json:"version"`
	Children     []*deptView `json:"children"`
}

type deptRow struct {
	ID           int64
	ParentID     *int64
	Path         string
	Name         string
	LeaderUserID *int64
	Sort         int
	Status       int16
	Version      int
}

func scopedDepts(ctx context.Context, db *gorm.DB) *gorm.DB {
	return actor.Scope(ctx, db.WithContext(ctx).Table("sys_departments AS d"), deptScope)
}

// deptTree returns visible departments as a forest; a department whose
// parent is not visible becomes a root.
func (m *Module) deptTree(c *gin.Context) (any, error) {
	var rows []deptRow
	if err := scopedDepts(c.Request.Context(), m.db).Select("d.*").Order("d.sort, d.id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	nodes := make(map[int64]*deptView, len(rows))
	for _, r := range rows {
		nodes[r.ID] = toDeptView(r)
	}
	roots := []*deptView{}
	for _, r := range rows {
		node := nodes[r.ID]
		if r.ParentID != nil {
			if parent, ok := nodes[*r.ParentID]; ok {
				parent.Children = append(parent.Children, node)
				continue
			}
		}
		roots = append(roots, node)
	}
	return roots, nil
}

func toDeptView(r deptRow) *deptView {
	return &deptView{
		ID: ids.ID(r.ID), ParentID: optionalID(r.ParentID), Name: r.Name, LeaderUserID: optionalID(r.LeaderUserID),
		Sort: r.Sort, Status: r.Status, Version: r.Version, Children: []*deptView{},
	}
}

func loadVisibleDept(ctx context.Context, tx *gorm.DB, id int64) (deptRow, error) {
	var rows []deptRow
	if err := scopedDepts(ctx, tx).Select("d.*").Where("d.id = ?", id).Scan(&rows).Error; err != nil {
		return deptRow{}, err
	}
	if len(rows) == 0 {
		return deptRow{}, httpx.NotFound("部门不存在")
	}
	return rows[0], nil
}

type deptRequest struct {
	ParentID     *ids.ID `json:"parentId"`
	Name         string  `json:"name" binding:"required,max=64"`
	LeaderUserID *ids.ID `json:"leaderUserId"`
	Sort         int     `json:"sort"`
	Status       int16   `json:"status" binding:"omitempty,oneof=1 2"`
	Version      *int    `json:"version"`
}

func (m *Module) createDept(c *gin.Context) (any, error) {
	var req deptRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if req.Status == 0 {
		req.Status = actor.StatusEnabled
	}
	ctx := c.Request.Context()
	var row deptRow
	err := database.RunTx(ctx, m.db, func(tx *gorm.DB) error {
		if err := lockDeptTree(tx); err != nil {
			return err
		}
		parentPath, err := m.parentPath(ctx, tx, req.ParentID)
		if err != nil {
			return err
		}
		if err := requireLeader(ctx, tx, req.LeaderUserID); err != nil {
			return err
		}
		var id int64
		if err := tx.Raw(`INSERT INTO sys_departments (parent_id, name, leader_user_id, sort, status, created_by, updated_by)
			VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`, int64Ptr(req.ParentID), req.Name, int64Ptr(req.LeaderUserID),
			req.Sort, req.Status, actor.From(ctx).UserID(), actor.From(ctx).UserID()).Scan(&id).Error; err != nil {
			return err
		}
		path := fmt.Sprintf("%s%d/", parentPath, id)
		if err := tx.Exec("UPDATE sys_departments SET path = ? WHERE id = ?", path, id).Error; err != nil {
			return err
		}
		return tx.Table("sys_departments").Where("id = ?", id).Scan(&row).Error
	})
	if err != nil {
		return nil, err
	}
	return toDeptView(row), nil
}

// parentPath returns the path new children of parentID start with. Only
// callers with the "all" scope may create root departments.
func (m *Module) parentPath(ctx context.Context, tx *gorm.DB, parentID *ids.ID) (string, error) {
	if parentID == nil {
		if !actor.From(ctx).Snapshot.DataScope.All {
			return "", fieldError("parentId", "必须选择数据权限范围内的上级部门")
		}
		return "/", nil
	}
	parent, err := loadVisibleDept(ctx, tx, int64(*parentID))
	if err != nil {
		return "", fieldError("parentId", "上级部门不存在或不在数据权限范围内")
	}
	return parent.Path, nil
}

func requireLeader(ctx context.Context, tx *gorm.DB, leader *ids.ID) error {
	if leader == nil {
		return nil
	}
	if err := requireVisibleUser(ctx, tx, int64(*leader)); err != nil {
		return fieldError("leaderUserId", "负责人不存在")
	}
	return nil
}

func (m *Module) updateDept(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var req deptRequest
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
	var row deptRow
	// Moving a department changes data scopes, so the auth version moves too.
	err = m.core.Versions.Mutate(ctx, func(tx *gorm.DB) error {
		if err := lockDeptTree(tx); err != nil {
			return err
		}
		current, err := loadVisibleDept(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := requireLeader(ctx, tx, req.LeaderUserID); err != nil {
			return err
		}
		if err := m.moveDept(ctx, tx, current, req.ParentID); err != nil {
			return err
		}
		err = updateVersioned(tx, "sys_departments", id, *req.Version, map[string]any{
			"parent_id": int64Ptr(req.ParentID), "name": req.Name, "leader_user_id": int64Ptr(req.LeaderUserID),
			"sort": req.Sort, "status": req.Status, "updated_by": actor.From(ctx).UserID(),
		}, "")
		if err != nil {
			return err
		}
		return tx.Table("sys_departments").Where("id = ?", id).Scan(&row).Error
	})
	if err != nil {
		return nil, err
	}
	return toDeptView(row), nil
}

// moveDept rewrites the paths of the department and all its descendants
// when its parent changes (design doc 7.4).
func (m *Module) moveDept(ctx context.Context, tx *gorm.DB, current deptRow, newParent *ids.ID) error {
	if sameParent(current.ParentID, newParent) {
		return nil
	}
	parentPath, err := m.parentPath(ctx, tx, newParent)
	if err != nil {
		return err
	}
	if len(parentPath) >= len(current.Path) && parentPath[:len(current.Path)] == current.Path {
		return fieldError("parentId", "不能移动到自身或下级部门之下")
	}
	newPath := fmt.Sprintf("%s%d/", parentPath, current.ID)
	return tx.Exec(`UPDATE sys_departments SET path = ? || substr(path, ?) WHERE path LIKE ?`,
		newPath, len(current.Path)+1, escapeLike(current.Path)+"%").Error
}

func sameParent(a *int64, b *ids.ID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == int64(*b)
}

func (m *Module) deleteDept(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	return nil, m.core.Versions.Mutate(ctx, func(tx *gorm.DB) error {
		if err := lockDeptTree(tx); err != nil {
			return err
		}
		if _, err := loadVisibleDept(ctx, tx, id); err != nil {
			return err
		}
		children, err := exists(tx, "SELECT 1 FROM sys_departments WHERE parent_id = ?", id)
		if err != nil {
			return err
		}
		users, err := exists(tx, "SELECT 1 FROM sys_users WHERE dept_id = ? AND deleted_at IS NULL", id)
		if err != nil {
			return err
		}
		if children || users {
			return inUse("部门下存在子部门或用户，不能删除")
		}
		// Soft-deleted users may still reference it; detach them first.
		if err := tx.Exec("UPDATE sys_users SET dept_id = NULL WHERE dept_id = ?", id).Error; err != nil {
			return err
		}
		return tx.Exec("DELETE FROM sys_departments WHERE id = ?", id).Error
	})
}
