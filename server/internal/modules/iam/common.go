package iam

import (
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
)

// IAM error codes (3xxxx, docs/api-p0.md).
const (
	CodeGrantDependency = 30001
	CodeProtected       = 30002
	CodeInUse           = 30003
)

var (
	errVersionConflict = httpx.Conflict("数据已被他人修改，请刷新")
	usernamePattern    = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,32}$`)
	codePattern        = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)
)

func protected(msg string) *httpx.Error {
	return httpx.NewError(http.StatusConflict, CodeProtected, msg)
}

func inUse(msg string) *httpx.Error {
	return httpx.NewError(http.StatusConflict, CodeInUse, msg)
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
// incrementing the version (design doc 8.1 optimistic locking).
func updateVersioned(tx *gorm.DB, table string, id int64, version int, sets map[string]any, uniqueMsg string) error {
	sets["version"] = gorm.Expr("version + 1")
	sets["updated_at"] = gorm.Expr("now()")
	res := tx.Table(table).Where("id = ? AND version = ?", id, version).Updates(sets)
	if res.Error != nil {
		if uniqueMsg != "" && database.IsUniqueViolation(res.Error) {
			return httpx.Conflict(uniqueMsg)
		}
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errVersionConflict
	}
	return nil
}

// exists reports whether a query matches at least one row.
func exists(tx *gorm.DB, query string, args ...any) (bool, error) {
	var found bool
	err := tx.Raw("SELECT EXISTS ("+query+")", args...).Scan(&found).Error
	return found, err
}
