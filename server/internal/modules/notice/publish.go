package notice

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
)

// permNoticePublish allows publishing and revoking notices.
const permNoticePublish = "notice:notice:publish"

type versionRequest struct {
	Version *int `json:"version" doc:"当前版本，用于乐观锁"`
}

func (m *Module) publishNotice(c *gin.Context) (any, error) {
	return m.setPublished(c, true)
}

func (m *Module) revokeNotice(c *gin.Context) (any, error) {
	return m.setPublished(c, false)
}

// setPublished publishes (published_at = now) or revokes (back to a draft)
// a notice. Publishing clears the read marks so that users see it as new.
func (m *Module) setPublished(c *gin.Context, publish bool) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var req versionRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	if req.Version == nil {
		return nil, fieldError("version", "不能为空")
	}
	ctx := c.Request.Context()
	var view noticeView
	err = database.RunTx(ctx, m.db, func(tx *gorm.DB) error {
		current, err := loadNotice(ctx, tx, id)
		if err != nil {
			return err
		}
		if publish == (current.PublishedAt != nil) {
			if publish {
				return httpx.Conflict("通知公告已发布")
			}
			return httpx.Conflict("通知公告尚未发布")
		}
		publishedAt := gorm.Expr("NULL")
		if publish {
			publishedAt = gorm.Expr("now()")
		}
		sets := map[string]any{
			"published_at": publishedAt,
			"updated_by":   actor.From(ctx).UserID(),
			"updated_at":   gorm.Expr("now()"),
		}
		if err := updateVersioned(tx, noticeTable, id, *req.Version, sets, noticeDuplicate); err != nil {
			return err
		}
		if publish {
			if err := tx.Exec(`DELETE FROM sys_notice_reads WHERE notice_id = ?`, id).Error; err != nil {
				return err
			}
		}
		view, err = loadNotice(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}
