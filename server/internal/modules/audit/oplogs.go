package audit

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/core/export"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

type opLogView struct {
	ID           ids.ID    `json:"id"`
	UserID       *ids.ID   `json:"userId"`
	Username     string    `json:"username"`
	Nickname     string    `json:"nickname"`
	Module       string    `json:"module"`
	Method       string    `json:"method"`
	Route        string    `json:"route"`
	Path         string    `json:"path"`
	Summary      string    `json:"summary"`
	Target       string    `json:"target"`
	Status       int       `json:"status"`
	Code         int       `json:"code"`
	Success      bool      `json:"success"`
	DurationMs   int       `json:"durationMs"`
	IP           string    `json:"ip"`
	UserAgent    string    `json:"userAgent"`
	RequestID    string    `json:"requestId"`
	ErrorMessage string    `json:"errorMessage"`
	CreatedAt    time.Time `json:"createdAt"`
}

// opLogDetail adds the captured request body to the list view.
type opLogDetail struct {
	opLogView
	RequestBody string `json:"requestBody"`
}

type opLogRow struct {
	ID           int64
	UserID       *int64
	Username     string
	Nickname     string
	Module       string
	Method       string
	Route        string
	Path         string
	Summary      string
	Target       string
	Status       int
	Code         int
	Success      bool
	DurationMs   int
	IP           string
	UserAgent    string
	RequestID    string
	ErrorMessage string
	RequestBody  string
	CreatedAt    time.Time
}

// The user is joined for display only; logs keep the id so they survive
// renamed or deleted users.
const opLogColumns = `o.id, o.user_id, COALESCE(u.username, '') AS username, COALESCE(u.nickname, '') AS nickname,
	o.module, o.method, o.route, o.path, o.summary, o.target, o.status, o.code, o.success, o.duration_ms,
	o.ip, o.user_agent, o.request_id, o.error_message, o.created_at`

type opFilter struct {
	// User matches the username or nickname.
	User   string `form:"user" json:"user" binding:"max=64"`
	Module string `form:"module" json:"module" binding:"max=32"`
	Method string `form:"method" json:"method" binding:"omitempty,oneof=GET POST PUT PATCH DELETE"`
	// Success is "true" or "false".
	Success string `form:"success" json:"success"`
	// Path matches the request path or the operation summary.
	Path string `form:"path" json:"path" binding:"max=128"`
	// Target is the operation's object, matched exactly (player:123).
	Target string `form:"target" json:"target" binding:"max=255"`
	timeRange
}

const opLogOrder = "o.created_at DESC, o.id DESC"

// baseOpQuery has no joins: the logs are counted and paged on their own
// table (with its created_at index); withUser joins the user afterwards.
func baseOpQuery(ctx context.Context, db *gorm.DB) *gorm.DB {
	return db.WithContext(ctx).Table(tableOperationLogs + " AS o")
}

// withUser selects the log columns of q with the user's names.
func withUser(q *gorm.DB) *gorm.DB {
	return q.Joins("LEFT JOIN sys_users u ON u.id = o.user_id").Select(opLogColumns)
}

func (m *Module) opLogQuery(c *gin.Context) (*gorm.DB, error) {
	var f opFilter
	if err := c.ShouldBindQuery(&f); err != nil {
		return nil, httpx.BadRequest("查询参数错误").WithCause(err)
	}
	return applyOpFilter(baseOpQuery(c.Request.Context(), m.db), f)
}

func applyOpFilter(q *gorm.DB, f opFilter) (*gorm.DB, error) {
	if v := strings.TrimSpace(f.User); v != "" {
		like := "%" + escapeLike(v) + "%"
		q = q.Where("o.user_id IN (SELECT id FROM sys_users WHERE LOWER(username) LIKE LOWER(?) OR LOWER(nickname) LIKE LOWER(?))",
			like, like)
	}
	if v := strings.TrimSpace(f.Module); v != "" {
		q = q.Where("o.module = ?", v)
	}
	if f.Method != "" {
		q = q.Where("o.method = ?", f.Method)
	}
	if v := strings.TrimSpace(f.Path); v != "" {
		like := "%" + escapeLike(v) + "%"
		q = q.Where("(LOWER(o.path) LIKE LOWER(?) OR LOWER(o.summary) LIKE LOWER(?))", like, like)
	}
	if v := strings.TrimSpace(f.Target); v != "" {
		q = q.Where("o.target = ?", v)
	}
	success, err := parseSuccess(f.Success)
	if err != nil {
		return nil, err
	}
	if success != nil {
		q = q.Where("o.success = ?", *success)
	}
	return f.apply(q, "o.created_at")
}

func (m *Module) listOperationLogs(c *gin.Context) (any, error) {
	pq, err := httpx.ParsePage(c)
	if err != nil {
		return nil, err
	}
	q, err := m.opLogQuery(c)
	if err != nil {
		return nil, err
	}
	pc, err := capCount(q, pq)
	if err != nil {
		return nil, err
	}
	var rows []opLogRow
	if err := withUser(database.PageFirst(q, "o", opLogOrder, pq.Offset(), pc.limit)).Scan(&rows).Error; err != nil {
		return nil, err
	}
	list := make([]opLogView, len(rows))
	for i, r := range rows {
		list[i] = toOpLogView(r)
	}
	return httpx.Page[opLogView]{List: list, Total: pc.total, TotalCapped: pc.capped, Page: pq.Page, PageSize: pq.PageSize}, nil
}

func (m *Module) getOperationLog(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	var rows []opLogRow
	if err := withUser(baseOpQuery(c.Request.Context(), m.db)).Select(opLogColumns+", o.request_body").
		Where("o.id = ?", id).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, httpx.NotFound("日志不存在")
	}
	return opLogDetail{opLogView: toOpLogView(rows[0]), RequestBody: rows[0].RequestBody}, nil
}

func toOpLogView(r opLogRow) opLogView {
	return opLogView{
		ID: ids.ID(r.ID), UserID: optionalID(r.UserID), Username: r.Username, Nickname: r.Nickname,
		Module: r.Module, Method: r.Method, Route: r.Route, Path: r.Path, Summary: r.Summary, Target: r.Target,
		Status: r.Status, Code: r.Code, Success: r.Success, DurationMs: r.DurationMs, IP: r.IP,
		UserAgent: r.UserAgent, RequestID: r.RequestID, ErrorMessage: r.ErrorMessage, CreatedAt: r.CreatedAt,
	}
}

func (m *Module) exportOperationLogs(c *gin.Context) {
	q, err := m.opLogQuery(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	maxRows, err := export.Check(c.Request.Context(), m.db, q)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var rows []opLogRow
	if err := withUser(database.PageFirst(q, "o", opLogOrder, 0, maxRows)).Scan(&rows).Error; err != nil {
		httpx.Fail(c, err)
		return
	}
	header := []string{"ID", export.TimeHeader("时间"), "用户名", "昵称", "模块", "操作", "操作对象", "方法", "路径", "HTTP 状态",
		"业务码", "结果", "耗时(ms)", "IP", "请求ID", "错误信息"}
	writeCSV(c, "operation-logs", header, len(rows), func(i int) []string {
		r := rows[i]
		return []string{
			itoa(r.ID), export.Time(r.CreatedAt), csvCell(r.Username), csvCell(r.Nickname),
			csvCell(r.Module), csvCell(r.Summary), csvCell(r.Target), r.Method, csvCell(r.Path), strconv.Itoa(r.Status),
			strconv.Itoa(r.Code), boolLabel(r.Success), strconv.Itoa(r.DurationMs), csvCell(r.IP),
			csvCell(r.RequestID), csvCell(r.ErrorMessage),
		}
	})
}
