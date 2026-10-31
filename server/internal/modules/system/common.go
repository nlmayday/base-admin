package system

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
)

// System error codes (5xxxx, docs/p1-p3-plan.md section 2).
const (
	// CodeProtected marks a built-in or read-only config.
	CodeProtected = 50001
)

var errVersionConflict = httpx.Conflict("数据已被他人修改，请刷新")

func protected(msg string) *httpx.Error {
	return httpx.NewError(http.StatusConflict, CodeProtected, msg)
}

func fieldError(field, msg string) error {
	return httpx.Validation([]httpx.FieldError{{Field: field, Message: msg}})
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

// updateVersioned applies sets to one row if its version still matches,
// incrementing the version (design doc 8.1 optimistic locking). table is
// always a package constant.
func updateVersioned(tx *gorm.DB, table string, id int64, version int, sets map[string]any, uniqueErr error) error {
	sets["version"] = gorm.Expr("version + 1")
	sets["updated_at"] = gorm.Expr("now()")
	res := tx.Table(table).Where("id = ? AND version = ?", id, version).Updates(sets)
	if res.Error != nil {
		if uniqueErr != nil && database.IsUniqueViolation(res.Error) {
			return uniqueErr
		}
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errVersionConflict
	}
	return nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// optionalStatus parses the ?status filter (1 or 2); empty means no filter.
func optionalStatus(c *gin.Context) (int16, error) {
	switch c.Query("status") {
	case "":
		return 0, nil
	case "1":
		return 1, nil
	case "2":
		return 2, nil
	default:
		return 0, fieldError("status", "取值必须是以下之一：1 2")
	}
}
