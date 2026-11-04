package file

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

const categoryNameTaken = "分类名称已存在"

type categoryView struct {
	ID        ids.ID    `json:"id"`
	Name      string    `json:"name"`
	Sort      int       `json:"sort"`
	FileCount int64     `json:"fileCount"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
}

type categoryRow struct {
	ID        int64
	Name      string
	Sort      int
	FileCount int64
	Version   int
	CreatedAt time.Time
}

func (r categoryRow) view() categoryView {
	return categoryView{ID: ids.ID(r.ID), Name: r.Name, Sort: r.Sort, FileCount: r.FileCount, Version: r.Version, CreatedAt: r.CreatedAt}
}

// categoryList is the sidebar: categories with live file counts plus the
// "all" and "uncategorized" totals.
type categoryList struct {
	List          []categoryView `json:"list"`
	Total         int64          `json:"total"`
	Uncategorized int64          `json:"uncategorized"`
}

const categorySelect = `SELECT c.id, c.name, c.sort, c.version, c.created_at,
	(SELECT count(*) FROM sys_files f WHERE f.category_id = c.id AND f.deleted_at IS NULL) AS file_count
	FROM sys_file_categories c`

func (m *Module) listCategories(c *gin.Context) (any, error) {
	db := m.db.WithContext(c.Request.Context())
	var rows []categoryRow
	if err := db.Raw(categorySelect + " ORDER BY c.sort, c.id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	var totals struct {
		Total         int64
		Uncategorized int64
	}
	if err := db.Raw(`SELECT count(*) AS total, count(CASE WHEN category_id IS NULL THEN 1 END) AS uncategorized
		FROM sys_files WHERE deleted_at IS NULL`).Scan(&totals).Error; err != nil {
		return nil, err
	}
	views := make([]categoryView, len(rows))
	for i, r := range rows {
		views[i] = r.view()
	}
	return categoryList{List: views, Total: totals.Total, Uncategorized: totals.Uncategorized}, nil
}

func (m *Module) loadCategory(db *gorm.DB, id int64) (categoryView, error) {
	var rows []categoryRow
	if err := db.Raw(categorySelect+" WHERE c.id = ?", id).Scan(&rows).Error; err != nil {
		return categoryView{}, err
	}
	if len(rows) == 0 {
		return categoryView{}, httpx.NotFound("分类不存在")
	}
	return rows[0].view(), nil
}

type categoryRequest struct {
	Name    string `json:"name" binding:"required,max=64"`
	Sort    int    `json:"sort" binding:"min=0,max=9999"`
	Version *int   `json:"version"`
}

func bindCategory(c *gin.Context) (categoryRequest, error) {
	var req categoryRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return req, err
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return req, fieldError("name", "不能为空")
	}
	return req, nil
}

func (m *Module) createCategory(c *gin.Context) (any, error) {
	req, err := bindCategory(c)
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	uid := actor.From(ctx).UserID()
	db := m.db.WithContext(ctx)
	id, err := database.InsertID(db, `INSERT INTO sys_file_categories (name, sort, created_by, updated_by) VALUES (?, ?, ?, ?)`,
		req.Name, req.Sort, uid, uid)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return nil, fieldError("name", categoryNameTaken)
		}
		return nil, err
	}
	return m.loadCategory(db, id)
}

func (m *Module) updateCategory(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	req, err := bindCategory(c)
	if err != nil {
		return nil, err
	}
	if req.Version == nil {
		return nil, fieldError("version", "不能为空")
	}
	ctx := c.Request.Context()
	db := m.db.WithContext(ctx)
	if _, err := m.loadCategory(db, id); err != nil {
		return nil, err
	}
	err = updateVersioned(db, "sys_file_categories", id, *req.Version, map[string]any{
		"name": req.Name, "sort": req.Sort, "updated_by": actor.From(ctx).UserID(),
	}, "name", categoryNameTaken)
	if err != nil {
		return nil, err
	}
	return m.loadCategory(db, id)
}

// deleteCategory refuses categories that still hold live files. The row
// lock makes concurrent uploads into the category wait for the outcome;
// their foreign key check then fails cleanly.
func (m *Module) deleteCategory(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	return nil, database.RunTx(c.Request.Context(), m.db, func(tx *gorm.DB) error {
		var locked []int64
		if err := tx.Raw("SELECT id FROM sys_file_categories WHERE id = ? FOR UPDATE", id).Scan(&locked).Error; err != nil {
			return err
		}
		if len(locked) == 0 {
			return httpx.NotFound("分类不存在")
		}
		var inUse bool
		if err := tx.Raw("SELECT EXISTS (SELECT 1 FROM sys_files WHERE category_id = ? AND deleted_at IS NULL)", id).
			Scan(&inUse).Error; err != nil {
			return err
		}
		if inUse {
			return httpx.NewError(http.StatusConflict, CodeCategoryInUse, "分类下还有文件，请先移动或删除文件")
		}
		return tx.Exec("DELETE FROM sys_file_categories WHERE id = ?", id).Error
	})
}
