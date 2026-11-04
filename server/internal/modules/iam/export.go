package iam

import (
	"encoding/csv"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"base-admin/server/internal/core/export"
	"base-admin/server/internal/platform/httpx"
)

func (m *Module) exportUsers(c *gin.Context) {
	var f userFilter
	if err := c.ShouldBindQuery(&f); err != nil {
		httpx.Fail(c, httpx.BadRequest("查询参数错误").WithCause(err))
		return
	}
	q, err := applyUserFilter(scopedUsers(c.Request.Context(), m.db), f)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	maxRows, err := export.Check(c.Request.Context(), m.db, q)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var rows []userRow
	if err := q.Select(userColumns).Order("u.id").Limit(maxRows).Scan(&rows).Error; err != nil {
		httpx.Fail(c, err)
		return
	}

	filename := fmt.Sprintf("users-%s.csv", time.Now().Format("20060102-150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	// UTF-8 BOM so spreadsheet tools detect the encoding of Chinese text.
	_, _ = c.Writer.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"ID", "用户名", "昵称", "邮箱", "手机", "部门", "状态", export.TimeHeader("创建时间")})
	for _, r := range rows {
		status := "启用"
		if r.Status != 1 {
			status = "停用"
		}
		_ = w.Write([]string{
			itoa(r.ID), csvCell(r.Username), csvCell(r.Nickname), csvCell(r.Email), csvCell(r.Phone),
			csvCell(r.DeptName), status, export.Time(r.CreatedAt),
		})
	}
	w.Flush()
}

// csvCell neutralizes spreadsheet formula injection.
func csvCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
