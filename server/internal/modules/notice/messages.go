package notice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/jobs"
	"base-admin/server/internal/core/message"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

// Personal messages: business modules send them to one user through
// message.Sender (Send below); the recipient reads them next to the
// notices.

var _ message.Sender = (*Module)(nil)

// sendBatch bounds the rows of one INSERT.
const sendBatch = 500

// Send implements message.Sender. Messages to users that do not exist or
// are deleted are skipped.
func (m *Module) Send(ctx context.Context, tx *gorm.DB, msgs ...message.Message) error {
	for _, msg := range msgs {
		if err := msg.Validate(); err != nil {
			return err
		}
	}
	db := tx
	if db == nil {
		db = m.db
	}
	db = db.WithContext(ctx)
	for start := 0; start < len(msgs); start += sendBatch {
		batch := msgs[start:min(start+sendBatch, len(msgs))]
		// One SELECT per message, so a message to a missing or deleted user
		// yields no row; both databases accept this form.
		rows := make([]string, len(batch))
		args := make([]any, 0, len(batch)*5)
		for i, msg := range batch {
			rows[i] = "SELECT u.id, ?, ?, ?, ? FROM sys_users u WHERE u.id = ? AND u.deleted_at IS NULL"
			args = append(args, msg.Category, strings.TrimSpace(msg.Title), msg.Content, msg.Link, msg.UserID)
		}
		err := db.Exec(`INSERT INTO sys_user_messages (user_id, category, title, content, link) `+
			strings.Join(rows, " UNION ALL "), args...).Error
		if err != nil {
			return fmt.Errorf("send messages: %w", err)
		}
	}
	return nil
}

func (m *Module) messageRoutes(r *module.Router) {
	g := r.Group("/me/messages")
	g.GET("", access.Authenticated(), handle(m.listMessages)).
		Doc(module.Doc{Summary: "我的个人消息", Query: messageFilter{}, Response: httpx.Page[messageView]{}})
	g.GET("/:id", access.Authenticated(), handle(m.readMessage)).
		Doc(module.Doc{Summary: "个人消息详情", Description: "同时标记为已读", Response: messageView{}})
	g.POST("/read-all", access.Authenticated(), handle(m.readAllMessages)).
		Doc(module.Doc{Summary: "个人消息全部标记为已读", Response: readAllView{}})
}

type messageView struct {
	ID        ids.ID     `json:"id"`
	Category  string     `json:"category" doc:"来源，如 crm.reminder"`
	Title     string     `json:"title"`
	Content   string     `json:"content"`
	Link      string     `json:"link" doc:"点击后打开的前端路由，空表示没有"`
	Read      bool       `json:"read"`
	ReadAt    *time.Time `json:"readAt"`
	CreatedAt time.Time  `json:"createdAt"`
}

type messageRow struct {
	ID        int64
	Category  string
	Title     string
	Content   string
	Link      string
	ReadAt    *time.Time
	CreatedAt time.Time
}

func (r messageRow) view() messageView {
	return messageView{
		ID: ids.ID(r.ID), Category: r.Category, Title: r.Title, Content: r.Content, Link: r.Link,
		Read: r.ReadAt != nil, ReadAt: r.ReadAt, CreatedAt: r.CreatedAt,
	}
}

type messageFilter struct {
	Unread bool   `form:"unread" doc:"只看未读"`
	Title  string `form:"title" binding:"max=128" doc:"标题（模糊匹配）"`
}

const messageColumns = "id, category, title, content, link, read_at, created_at"

// myMessages starts a query over the caller's messages.
func (m *Module) myMessages(c *gin.Context) *gorm.DB {
	uid := actor.From(c.Request.Context()).UserID()
	return m.db.WithContext(c.Request.Context()).Table("sys_user_messages").Where("user_id = ?", uid)
}

func (m *Module) listMessages(c *gin.Context) (any, error) {
	pq, err := httpx.ParsePage(c)
	if err != nil {
		return nil, err
	}
	var f messageFilter
	if err := c.ShouldBindQuery(&f); err != nil {
		return nil, httpx.BadRequest("查询参数错误").WithCause(err)
	}
	q := m.myMessages(c)
	if f.Unread {
		q = q.Where("read_at IS NULL")
	}
	if f.Title != "" {
		q = q.Where("LOWER(title) LIKE LOWER(?)", "%"+escapeLike(f.Title)+"%")
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []messageRow
	err = q.Select(messageColumns).Order("created_at DESC, id DESC").
		Offset(pq.Offset()).Limit(pq.PageSize).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	views := make([]messageView, len(rows))
	for i, r := range rows {
		views[i] = r.view()
	}
	return httpx.Page[messageView]{List: views, Total: total, Page: pq.Page, PageSize: pq.PageSize}, nil
}

// readMessage returns one of the caller's messages and marks it read.
func (m *Module) readMessage(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	db := m.db.WithContext(c.Request.Context())
	uid := actor.From(c.Request.Context()).UserID()
	if err := db.Exec(`UPDATE sys_user_messages SET read_at = CURRENT_TIMESTAMP(6)
		WHERE id = ? AND user_id = ? AND read_at IS NULL`, id, uid).Error; err != nil {
		return nil, err
	}
	var rows []messageRow
	if err := db.Raw(`SELECT `+messageColumns+` FROM sys_user_messages WHERE id = ? AND user_id = ?`, id, uid).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, httpx.NotFound("消息不存在")
	}
	return rows[0].view(), nil
}

func (m *Module) readAllMessages(c *gin.Context) (any, error) {
	res := m.myMessages(c).Where("read_at IS NULL").Update("read_at", gorm.Expr("CURRENT_TIMESTAMP(6)"))
	if res.Error != nil {
		return nil, res.Error
	}
	return readAllView{Marked: res.RowsAffected}, nil
}

// unreadMessages counts the caller's unread messages.
func (m *Module) unreadMessages(c *gin.Context) (int64, error) {
	var n int64
	err := m.myMessages(c).Where("read_at IS NULL").Count(&n).Error
	return n, err
}

const (
	cleanupJobKey      = "notice.message-cleanup"
	defaultMessageDays = 90
	minMessageDays     = 7
	maxMessageDays     = 3650
	cleanupBatchSize   = 5000
)

// Jobs implements jobs.Provider.
func (m *Module) Jobs() []jobs.Handler {
	return []jobs.Handler{{
		Key:            cleanupJobKey,
		Name:           "清理个人消息",
		Description:    `删除已读超过 N 天的个人消息（默认 90 天，参数 {"days": N}），以及已删除用户的全部消息`,
		DefaultTimeout: 30 * time.Minute,
		Run:            m.cleanupMessages,
	}}
}

func parseMessageDays(raw json.RawMessage) (int, error) {
	p := struct {
		Days int `json:"days"`
	}{Days: defaultMessageDays}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) != 0 && !bytes.Equal(trimmed, []byte("null")) {
		if err := json.Unmarshal(trimmed, &p); err != nil {
			return 0, fmt.Errorf("invalid params: %w", err)
		}
	}
	if p.Days < minMessageDays || p.Days > maxMessageDays {
		return 0, fmt.Errorf("days must be between %d and %d, got %d", minMessageDays, maxMessageDays, p.Days)
	}
	return p.Days, nil
}

// cleanupMessages deletes messages read more than the configured days ago
// and the messages of deleted users, in batches.
func (m *Module) cleanupMessages(ctx context.Context, raw json.RawMessage) error {
	days, err := parseMessageDays(raw)
	if err != nil {
		return err
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		// The derived table lets MySQL limit a subquery of the table it
		// deletes from; PostgreSQL accepts the same statement.
		res := m.db.WithContext(ctx).Exec(`DELETE FROM sys_user_messages WHERE id IN (
			SELECT id FROM (SELECT msg.id FROM sys_user_messages msg JOIN sys_users u ON u.id = msg.user_id
			WHERE msg.read_at < ? OR u.deleted_at IS NOT NULL LIMIT ?) AS batch)`, cutoff, cleanupBatchSize)
		if res.Error != nil {
			return fmt.Errorf("clean up messages: %w", res.Error)
		}
		if res.RowsAffected < cleanupBatchSize {
			return nil
		}
	}
}
