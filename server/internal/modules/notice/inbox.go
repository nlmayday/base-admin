package notice

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/attachment"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

// The inbox: published notices as every signed-in user sees them, with the
// caller's read marks. Data scope does not apply; a published notice is for
// everyone.

func (m *Module) inboxRoutes(r *module.Router) {
	g := r.Group("/me/notices")
	g.GET("", access.Authenticated(), handle(m.listInbox)).Doc(module.Doc{Summary: "我的消息", Query: inboxFilter{}, Response: httpx.Page[inboxView]{}})
	g.GET("/unread-count", access.Authenticated(), handle(m.unreadCount)).Doc(module.Doc{Summary: "未读消息数", Description: "公告与个人消息合计，并分别给出", Response: unreadView{}})
	g.GET("/:id", access.Authenticated(), handle(m.readInbox)).Doc(module.Doc{Summary: "消息详情", Description: "同时标记为已读", Response: inboxView{}})
	g.POST("/read-all", access.Authenticated(), handle(m.readAll)).Doc(module.Doc{Summary: "全部标记为已读", Response: readAllView{}})
}

type inboxView struct {
	ID          ids.ID           `json:"id"`
	Title       string           `json:"title"`
	Type        int16            `json:"type"`
	Content     string           `json:"content,omitempty" doc:"列表中不返回"`
	Attachments []attachment.Ref `json:"attachments,omitempty" doc:"附件；列表中不返回"`
	PublishedAt time.Time        `json:"publishedAt"`
	Read        bool             `json:"read"`
	ReadAt      *time.Time       `json:"readAt"`
	Publisher   string           `json:"publisher" doc:"发布人昵称"`
}

type inboxRow struct {
	ID          int64
	Title       string
	Type        int16
	Content     string
	Attachments string
	PublishedAt time.Time
	ReadAt      *time.Time
	Publisher   *string
}

func (r inboxRow) view(withContent bool) inboxView {
	v := inboxView{
		ID: ids.ID(r.ID), Title: r.Title, Type: r.Type, PublishedAt: r.PublishedAt,
		Read: r.ReadAt != nil, ReadAt: r.ReadAt,
	}
	if withContent {
		v.Content, v.Attachments = r.Content, decodeAttachments(r.Attachments)
	}
	if r.Publisher != nil {
		v.Publisher = *r.Publisher
	}
	return v
}

type inboxFilter struct {
	Unread bool   `form:"unread" doc:"只看未读"`
	Title  string `form:"title" binding:"max=128" doc:"标题（模糊匹配）"`
}

type unreadView struct {
	// Count is the bell's number: unread notices plus personal messages.
	Count    int64 `json:"count" doc:"未读总数（公告 + 个人消息）"`
	Notices  int64 `json:"notices" doc:"未读公告数"`
	Messages int64 `json:"messages" doc:"未读个人消息数"`
}

type readAllView struct {
	Marked int64 `json:"marked" doc:"本次标记的条数"`
}

// inbox starts a query over published notices joined with the caller's
// read marks and the publisher's nickname.
func (m *Module) inbox(c *gin.Context) *gorm.DB {
	uid := actor.From(c.Request.Context()).UserID()
	return m.db.WithContext(c.Request.Context()).Table(noticeTable+" AS t").
		Joins("LEFT JOIN sys_notice_reads r ON r.notice_id = t.id AND r.user_id = ?", uid).
		Joins("LEFT JOIN sys_users u ON u.id = t.updated_by").
		Where("t.published_at IS NOT NULL")
}

const inboxColumns = `t.id, t.title, t.type, t.content, t.attachments AS attachments, t.published_at,
	r.read_at, u.nickname AS publisher`

func (m *Module) listInbox(c *gin.Context) (any, error) {
	pq, err := httpx.ParsePage(c)
	if err != nil {
		return nil, err
	}
	var f inboxFilter
	if err := c.ShouldBindQuery(&f); err != nil {
		return nil, httpx.BadRequest("查询参数错误").WithCause(err)
	}
	q := m.inbox(c)
	if f.Unread {
		q = q.Where("r.read_at IS NULL")
	}
	if f.Title != "" {
		q = q.Where("LOWER(t.title) LIKE LOWER(?)", "%"+escapeLike(f.Title)+"%")
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []inboxRow
	err = q.Select(inboxColumns).Order("t.published_at DESC, t.id DESC").
		Offset(pq.Offset()).Limit(pq.PageSize).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	views := make([]inboxView, len(rows))
	for i, r := range rows {
		views[i] = r.view(false)
	}
	return httpx.Page[inboxView]{List: views, Total: total, Page: pq.Page, PageSize: pq.PageSize}, nil
}

func (m *Module) unreadCount(c *gin.Context) (any, error) {
	var notices int64
	if err := m.inbox(c).Where("r.read_at IS NULL").Count(&notices).Error; err != nil {
		return nil, err
	}
	messages, err := m.unreadMessages(c)
	if err != nil {
		return nil, err
	}
	return unreadView{Count: notices + messages, Notices: notices, Messages: messages}, nil
}

// readInbox returns one published notice and marks it read.
func (m *Module) readInbox(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var rows []inboxRow
	if err := m.inbox(c).Select(inboxColumns).Where("t.id = ?", id).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, httpx.NotFound("消息不存在或已撤回")
	}
	row := rows[0]
	if row.ReadAt == nil {
		now := time.Now()
		err := m.db.WithContext(c.Request.Context()).Exec(`INSERT INTO sys_notice_reads (notice_id, user_id, read_at)
			VALUES (?, ?, ?) `+database.OnConflictDoNothing(m.db, "sys_notice_reads", "user_id"),
			id, actor.From(c.Request.Context()).UserID(), now).Error
		if err != nil {
			return nil, err
		}
		row.ReadAt = &now
	}
	return row.view(true), nil
}

func (m *Module) readAll(c *gin.Context) (any, error) {
	// Only unread notices are inserted, so the affected rows count the
	// newly marked ones on both databases; the conflict clause covers a
	// concurrent read of the same notice.
	uid := actor.From(c.Request.Context()).UserID()
	res := m.db.WithContext(c.Request.Context()).Exec(`INSERT INTO sys_notice_reads (notice_id, user_id)
		SELECT t.id, ? FROM sys_notices t WHERE t.published_at IS NOT NULL
		AND NOT EXISTS (SELECT 1 FROM sys_notice_reads r WHERE r.notice_id = t.id AND r.user_id = ?) `+
		database.OnConflictDoNothing(m.db, "sys_notice_reads", "user_id"), uid, uid)
	if res.Error != nil {
		return nil, res.Error
	}
	return readAllView{Marked: res.RowsAffected}, nil
}
