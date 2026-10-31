package file

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
)

// File error codes (6xxxx, docs/p1-p3-plan.md).
const (
	CodeCategoryInUse   = 60001
	CodeTypeNotAllowed  = 60002
	CodeTooLarge        = 60003
	CodeContentMismatch = 60004
	CodeLinkInvalid     = 60005
	CodeLinkExpired     = 60006
	CodeStorageFailed   = 60007
	CodeEmptyFile       = 60008
)

var errVersionConflict = httpx.Conflict("数据已被他人修改，请刷新")

func fieldError(field, msg string) error {
	return httpx.Validation([]httpx.FieldError{{Field: field, Message: msg}})
}

func typeNotAllowed(msg string) *httpx.Error {
	return httpx.NewError(http.StatusUnprocessableEntity, CodeTypeNotAllowed, msg)
}

func tooLarge(maxBytes int64) *httpx.Error {
	return httpx.NewError(http.StatusRequestEntityTooLarge, CodeTooLarge,
		"文件大小超过限制（最大 "+humanSize(maxBytes)+"）")
}

func storageFailed(cause error) *httpx.Error {
	return httpx.NewError(http.StatusServiceUnavailable, CodeStorageFailed, "存储服务暂不可用，请稍后重试").WithCause(cause)
}

// humanSize formats bytes as B, KB, MB or GB with up to one decimal.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	v := float64(n) / unit
	i := 0
	for v >= unit && i < len(units)-1 {
		v /= unit
		i++
	}
	s := fmt.Sprintf("%.1f", v)
	return strings.TrimSuffix(s, ".0") + " " + units[i]
}

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

// updateVersioned applies sets to one live row if its version still
// matches, incrementing the version (design doc 8.1 optimistic locking).
func updateVersioned(tx *gorm.DB, table string, id int64, version int, sets map[string]any, uniqueField, uniqueMsg string) error {
	sets["version"] = gorm.Expr("version + 1")
	sets["updated_at"] = gorm.Expr("now()")
	res := tx.Table(table).Where("id = ? AND version = ?", id, version).Updates(sets)
	if res.Error != nil {
		if uniqueMsg != "" && database.IsUniqueViolation(res.Error) {
			return fieldError(uniqueField, uniqueMsg)
		}
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errVersionConflict
	}
	return nil
}

// escapeLike escapes LIKE wildcards in user input.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
