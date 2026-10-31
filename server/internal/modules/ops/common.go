package ops

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
)

var errVersionConflict = httpx.Conflict("数据已被他人修改，请刷新")

func errHandlerMissing(key string) *httpx.Error {
	return httpx.NewError(http.StatusConflict, CodeHandlerMissing, "任务处理器未注册："+key)
}

func errStopping() *httpx.Error {
	return httpx.NewError(http.StatusServiceUnavailable, CodeSchedulerStopping, "调度器正在停止，请稍后再试")
}

func errMonitor() *httpx.Error {
	return httpx.NewError(http.StatusServiceUnavailable, CodeMonitorUnavailable, "无法获取监控数据")
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

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// escapeLike escapes LIKE wildcards in user input.
func escapeLike(s string) string { return likeEscaper.Replace(s) }
