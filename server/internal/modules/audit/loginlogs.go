package audit

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/core/export"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

type loginLogView struct {
	ID        ids.ID    `json:"id"`
	Kind      string    `json:"kind"`
	Username  string    `json:"username"`
	UserID    *ids.ID   `json:"userId"`
	Success   bool      `json:"success"`
	Reason    string    `json:"reason"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"userAgent"`
	RequestID string    `json:"requestId"`
	CreatedAt time.Time `json:"createdAt"`
}

type loginLogRow struct {
	ID        int64
	Kind      string
	Username  string
	UserID    *int64
	Success   bool
	Reason    string
	IP        string
	UserAgent string
	RequestID string
	CreatedAt time.Time
}

const loginLogColumns = "l.id, l.kind, l.username, l.user_id, l.success, l.reason, l.ip, l.user_agent, l.request_id, l.created_at"

type loginFilter struct {
	Username string `form:"username" json:"username" binding:"max=64"`
	Success  string `form:"success" json:"success"`
	IP       string `form:"ip" json:"ip" binding:"max=64"`
	Kind     string `form:"kind" json:"kind" binding:"omitempty,oneof=login logout"`
	timeRange
}

// loginLogQuery binds the filter and returns the filtered query.
func (m *Module) loginLogQuery(c *gin.Context) (*gorm.DB, error) {
	var f loginFilter
	if err := c.ShouldBindQuery(&f); err != nil {
		return nil, httpx.BadRequest("查询参数错误").WithCause(err)
	}
	return applyLoginFilter(c.Request.Context(), m.db, f)
}

func applyLoginFilter(ctx context.Context, db *gorm.DB, f loginFilter) (*gorm.DB, error) {
	q := db.WithContext(ctx).Table(tableLoginLogs + " AS l")
	if v := strings.TrimSpace(f.Username); v != "" {
		q = q.Where("LOWER(l.username) LIKE LOWER(?)", "%"+escapeLike(v)+"%")
	}
	if v := strings.TrimSpace(f.IP); v != "" {
		q = q.Where("l.ip LIKE ?", escapeLike(v)+"%")
	}
	if f.Kind != "" {
		q = q.Where("l.kind = ?", f.Kind)
	}
	success, err := parseSuccess(f.Success)
	if err != nil {
		return nil, err
	}
	if success != nil {
		q = q.Where("l.success = ?", *success)
	}
	return f.apply(q, "l.created_at")
}

func (m *Module) listLoginLogs(c *gin.Context) (any, error) {
	pq, err := httpx.ParsePage(c)
	if err != nil {
		return nil, err
	}
	q, err := m.loginLogQuery(c)
	if err != nil {
		return nil, err
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []loginLogRow
	if err := q.Select(loginLogColumns).Order("l.created_at DESC, l.id DESC").
		Offset(pq.Offset()).Limit(pq.PageSize).Scan(&rows).Error; err != nil {
		return nil, err
	}
	list := make([]loginLogView, len(rows))
	for i, r := range rows {
		list[i] = toLoginLogView(r)
	}
	return httpx.Page[loginLogView]{List: list, Total: total, Page: pq.Page, PageSize: pq.PageSize}, nil
}

func toLoginLogView(r loginLogRow) loginLogView {
	return loginLogView{
		ID: ids.ID(r.ID), Kind: r.Kind, Username: r.Username, UserID: optionalID(r.UserID),
		Success: r.Success, Reason: r.Reason, IP: r.IP, UserAgent: r.UserAgent,
		RequestID: r.RequestID, CreatedAt: r.CreatedAt,
	}
}

var kindLabels = map[string]string{"login": "登录", "logout": "退出"}

func (m *Module) exportLoginLogs(c *gin.Context) {
	q, err := m.loginLogQuery(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	maxRows, err := export.Check(c.Request.Context(), m.db, q)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var rows []loginLogRow
	if err := q.Select(loginLogColumns).Order("l.created_at DESC, l.id DESC").
		Limit(maxRows).Scan(&rows).Error; err != nil {
		httpx.Fail(c, err)
		return
	}
	header := []string{"ID", export.TimeHeader("时间"), "类型", "用户名", "结果", "原因", "IP", "浏览器", "请求ID"}
	writeCSV(c, "login-logs", header, len(rows), func(i int) []string {
		r := rows[i]
		return []string{
			itoa(r.ID), export.Time(r.CreatedAt), kindLabels[r.Kind], csvCell(r.Username),
			boolLabel(r.Success), csvCell(reasonLabel(r.Reason)), csvCell(r.IP), csvCell(r.UserAgent),
			csvCell(r.RequestID),
		}
	})
}
