package audit

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

// handle adapts a function returning (data, error) to a Gin handler.
func handle(fn func(c *gin.Context) (any, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := fn(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, data)
	}
}

func fieldError(field, msg string) error {
	return httpx.Validation([]httpx.FieldError{{Field: field, Message: msg}})
}

// escapeLike escapes LIKE wildcards in user input.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// timeRange is the optional [startTime, endTime] filter (RFC 3339).
type timeRange struct {
	StartTime string `form:"startTime" json:"startTime"`
	EndTime   string `form:"endTime" json:"endTime"`
}

// apply adds the created_at bounds of tr to q.
func (tr timeRange) apply(q *gorm.DB, column string) (*gorm.DB, error) {
	start, err := parseTime("startTime", tr.StartTime)
	if err != nil {
		return nil, err
	}
	end, err := parseTime("endTime", tr.EndTime)
	if err != nil {
		return nil, err
	}
	if start != nil && end != nil && end.Before(*start) {
		return nil, fieldError("endTime", "结束时间不能早于开始时间")
	}
	if start != nil {
		q = q.Where(column+" >= ?", *start)
	}
	if end != nil {
		q = q.Where(column+" <= ?", *end)
	}
	return q, nil
}

func parseTime(field, v string) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, fieldError(field, "时间格式不正确，应为 RFC 3339")
	}
	return &t, nil
}

// parseSuccess converts the "true"/"false" filter.
func parseSuccess(v string) (*bool, error) {
	switch v {
	case "":
		return nil, nil
	case "true", "1":
		b := true
		return &b, nil
	case "false", "0":
		b := false
		return &b, nil
	default:
		return nil, fieldError("success", "取值必须是 true 或 false")
	}
}

func optionalID(p *int64) *ids.ID {
	if p == nil {
		return nil
	}
	id := ids.ID(*p)
	return &id
}

// csvCell neutralizes spreadsheet formula injection (design doc 17, #10).
func csvCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func boolLabel(ok bool) string {
	if ok {
		return "成功"
	}
	return "失败"
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// writeCSV streams a CSV download with a UTF-8 BOM. Every cell produced by
// rowFn must already be passed through csvCell when it holds user data.
func writeCSV(c *gin.Context, prefix string, header []string, n int, rowFn func(i int) []string) {
	filename := fmt.Sprintf("%s-%s.csv", prefix, time.Now().Format("20060102-150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	// UTF-8 BOM so spreadsheet tools detect the encoding of Chinese text.
	_, _ = c.Writer.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(c.Writer)
	_ = w.Write(header)
	for i := range n {
		_ = w.Write(rowFn(i))
	}
	w.Flush()
}
