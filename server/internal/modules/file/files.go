package file

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
	"base-admin/server/internal/platform/storage"
)

// uncategorized is the categoryId filter value for files without one.
const uncategorized = "none"

type fileView struct {
	ID           ids.ID    `json:"id"`
	OriginalName string    `json:"originalName"`
	Ext          string    `json:"ext"`
	Kind         string    `json:"kind"`
	MimeType     string    `json:"mimeType"`
	Size         int64     `json:"size"`
	Sha256       string    `json:"sha256"`
	Storage      string    `json:"storage"`
	CategoryID   *ids.ID   `json:"categoryId"`
	CategoryName string    `json:"categoryName"`
	UploadedBy   *ids.ID   `json:"uploadedBy"`
	UploaderName string    `json:"uploaderName"`
	Version      int       `json:"version"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type fileRow struct {
	ID           int64
	CategoryID   *int64
	CategoryName *string
	OriginalName string
	ObjectKey    string
	Storage      string
	Size         int64
	MimeType     string
	Ext          string
	Kind         string
	Sha256       string
	UploadedBy   *int64
	UploaderName *string
	Version      int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

const fileColumns = `f.id, f.category_id, c.name AS category_name, f.original_name, f.object_key, f.storage, f.size,
	f.mime_type, f.ext, f.kind, f.sha256, f.uploaded_by, COALESCE(NULLIF(u.nickname, ''), u.username) AS uploader_name,
	f.version, f.created_at, f.updated_at`

func optionalID(n *int64) *ids.ID {
	if n == nil {
		return nil
	}
	id := ids.ID(*n)
	return &id
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (r fileRow) view() fileView {
	return fileView{
		ID: ids.ID(r.ID), OriginalName: r.OriginalName, Ext: r.Ext, Kind: r.Kind, MimeType: r.MimeType,
		Size: r.Size, Sha256: r.Sha256, Storage: r.Storage, CategoryID: optionalID(r.CategoryID),
		CategoryName: deref(r.CategoryName), UploadedBy: optionalID(r.UploadedBy), UploaderName: deref(r.UploaderName),
		Version: r.Version, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// liveFiles selects non-deleted files with their category and uploader.
func liveFiles(db *gorm.DB) *gorm.DB {
	return db.Table("sys_files AS f").
		Joins("LEFT JOIN sys_file_categories c ON c.id = f.category_id").
		Joins("LEFT JOIN sys_users u ON u.id = f.uploaded_by").
		Where("f.deleted_at IS NULL")
}

func (m *Module) loadFileRow(db *gorm.DB, id int64) (fileRow, error) {
	var rows []fileRow
	if err := liveFiles(db).Select(fileColumns).Where("f.id = ?", id).Scan(&rows).Error; err != nil {
		return fileRow{}, err
	}
	if len(rows) == 0 {
		return fileRow{}, httpx.NotFound("文件不存在")
	}
	return rows[0], nil
}

func (m *Module) loadFile(db *gorm.DB, id int64) (fileView, error) {
	row, err := m.loadFileRow(db, id)
	if err != nil {
		return fileView{}, err
	}
	return row.view(), nil
}

type fileFilter struct {
	Page     int    `form:"page"`
	PageSize int    `form:"pageSize"`
	Keyword  string `form:"keyword"`
	// CategoryID is a category id, "none" for uncategorized, or empty.
	CategoryID string `form:"categoryId"`
	// Kind is image, document, media, archive or other.
	Kind string `form:"kind"`
}

func applyFileFilter(q *gorm.DB, f fileFilter) (*gorm.DB, error) {
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		q = q.Where("LOWER(f.original_name) LIKE LOWER(?)", "%"+escapeLike(kw)+"%")
	}
	switch f.CategoryID {
	case "":
	case uncategorized:
		q = q.Where("f.category_id IS NULL")
	default:
		id, err := ids.Parse(f.CategoryID)
		if err != nil {
			return nil, fieldError("categoryId", "分类参数无效")
		}
		q = q.Where("f.category_id = ?", int64(id))
	}
	if f.Kind != "" {
		if !slices.Contains(allKinds, f.Kind) {
			return nil, fieldError("kind", "取值必须是以下之一："+strings.Join(allKinds, " "))
		}
		q = q.Where("f.kind = ?", f.Kind)
	}
	return q, nil
}

func (m *Module) listFiles(c *gin.Context) (any, error) {
	pq, err := httpx.ParsePage(c)
	if err != nil {
		return nil, err
	}
	f := fileFilter{Keyword: c.Query("keyword"), CategoryID: c.Query("categoryId"), Kind: c.Query("kind")}
	q, err := applyFileFilter(liveFiles(m.db.WithContext(c.Request.Context())), f)
	if err != nil {
		return nil, err
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []fileRow
	if err := q.Select(fileColumns).Order("f.created_at DESC, f.id DESC").
		Offset(pq.Offset()).Limit(pq.PageSize).Scan(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]fileView, len(rows))
	for i, r := range rows {
		views[i] = r.view()
	}
	return httpx.Page[fileView]{List: views, Total: total, Page: pq.Page, PageSize: pq.PageSize}, nil
}

func (m *Module) getFile(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	return m.loadFile(m.db.WithContext(c.Request.Context()), id)
}

type updateFileRequest struct {
	OriginalName string `json:"originalName" binding:"required,max=255"`
	// CategoryID null moves the file out of any category.
	CategoryID *ids.ID `json:"categoryId"`
	Version    *int    `json:"version"`
}

// updateFile renames and/or moves a file. The extension is kept: it
// decides the stored content type.
func (m *Module) updateFile(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var req updateFileRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if req.Version == nil {
		return nil, fieldError("version", "不能为空")
	}
	name := sanitizeName(req.OriginalName)
	if name == "" {
		return nil, fieldError("originalName", "文件名无效")
	}
	db := m.db.WithContext(c.Request.Context())
	row, err := m.loadFileRow(db, id)
	if err != nil {
		return nil, err
	}
	var category *int64
	if req.CategoryID != nil {
		n := int64(*req.CategoryID)
		category = &n
	}
	err = updateVersioned(db.Where("deleted_at IS NULL"), "sys_files", id, *req.Version, map[string]any{
		"original_name": sanitizeName(withExt(name, row.Ext)), "category_id": category,
	}, "", "")
	if err != nil {
		if database.IsForeignKeyViolation(err) {
			return nil, fieldError("categoryId", "分类不存在")
		}
		return nil, err
	}
	return m.loadFile(db, id)
}

// deleteFile soft-deletes the row, then removes the object and the row.
// The soft delete commits first, so a storage or connection failure never
// leaves a live row without its object; whatever is left over is finished
// by the file.purge-deleted job.
func (m *Module) deleteFile(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	var keys []string
	err = database.RunTx(ctx, m.db, func(tx *gorm.DB) error {
		if err := tx.Raw(`SELECT object_key FROM sys_files WHERE id = ? AND deleted_at IS NULL FOR UPDATE`, id).
			Scan(&keys).Error; err != nil {
			return err
		}
		if len(keys) == 0 {
			return httpx.NotFound("文件不存在")
		}
		return tx.Exec(`UPDATE sys_files SET deleted_at = CURRENT_TIMESTAMP(6), updated_at = CURRENT_TIMESTAMP(6)
			WHERE id = ?`, id).Error
	})
	if err != nil {
		return nil, err
	}
	purgeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if err := m.purge(purgeCtx, id, keys[0]); err != nil {
		m.log.Warn("file object not deleted yet; the purge job will retry",
			zap.Int64("file", id), zap.String("key", keys[0]), zap.Error(err))
	}
	return nil, nil
}

// purge deletes the object of a soft-deleted file, then the row.
func (m *Module) purge(ctx context.Context, id int64, key string) error {
	if err := m.deleteObject(ctx, key); err != nil {
		return err
	}
	return m.db.WithContext(ctx).Exec(`DELETE FROM sys_files WHERE id = ? AND deleted_at IS NOT NULL`, id).Error
}

func (m *Module) deleteObject(ctx context.Context, key string) error {
	if err := m.storage.Delete(ctx, key); err != nil && !errors.Is(err, storage.ErrNotFound) {
		return storageFailed(err)
	}
	return nil
}
