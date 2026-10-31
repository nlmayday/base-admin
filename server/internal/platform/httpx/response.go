package httpx

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"base-admin/server/internal/platform/requestid"
)

// Envelope is the uniform response body for every API response.
type Envelope struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data"`
	RequestID string `json:"requestId"`
}

// OK writes a successful response.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{
		Code:      CodeOK,
		Message:   "success",
		Data:      data,
		RequestID: requestid.From(c.Request.Context()),
	})
}

// Fail writes an error response and aborts the handler chain. The original
// error is attached to the context so the access log can record it.
func Fail(c *gin.Context, err error) {
	if err == nil {
		err = errors.New("httpx.Fail called with nil error")
	}
	apiErr := AsError(err)
	_ = c.Error(err)
	c.AbortWithStatusJSON(apiErr.Status, Envelope{
		Code:      apiErr.Code,
		Message:   apiErr.Message,
		Data:      apiErr.Details,
		RequestID: requestid.From(c.Request.Context()),
	})
}

// NoRoute answers unknown routes. Unregistered routes are always 404, never
// 405, so the route table is not disclosed.
func NoRoute(c *gin.Context) {
	Fail(c, NotFound("资源不存在"))
}

// Page is a paged list response.
type Page[T any] struct {
	List     []T   `json:"list"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

// PageQuery is a validated page request.
type PageQuery struct {
	Page     int
	PageSize int
}

// Offset returns the row offset of the page.
func (q PageQuery) Offset() int { return (q.Page - 1) * q.PageSize }

// Page limits.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// ParsePage reads ?page and ?pageSize (defaults 1 and 20, max 100).
func ParsePage(c *gin.Context) (PageQuery, error) {
	q := PageQuery{Page: 1, PageSize: DefaultPageSize}
	var fields []FieldError
	if v := c.Query("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			fields = append(fields, FieldError{Field: "page", Message: "必须是正整数"})
		}
		q.Page = n
	}
	if v := c.Query("pageSize"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > MaxPageSize {
			fields = append(fields, FieldError{Field: "pageSize", Message: "必须在 1 到 " + strconv.Itoa(MaxPageSize) + " 之间"})
		}
		q.PageSize = n
	}
	if len(fields) > 0 {
		return PageQuery{}, Validation(fields)
	}
	return q, nil
}

// PathID parses a positive integer path parameter.
func PathID(c *gin.Context, name string) (int64, error) {
	n, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || n <= 0 {
		return 0, NotFound("资源不存在")
	}
	return n, nil
}
