package notice

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
)

var errVersionConflict = httpx.Conflict("数据已被他人修改，请刷新")

// DictLookup validates values of dictionary-backed fields.
type DictLookup interface {
	HasDictValue(ctx context.Context, code, value string) (bool, error)
}

// checkDict rejects a value that is not an enabled item of dictionary code.
// Empty values and a nil lookup are accepted.
func checkDict(ctx context.Context, dict DictLookup, code, field, label string, v any) error {
	value := dictValue(v)
	if dict == nil || value == "" {
		return nil
	}
	ok, err := dict.HasDictValue(ctx, code, value)
	if err != nil {
		return fmt.Errorf("check %s: %w", field, err)
	}
	if !ok {
		return fieldError(field, "请选择有效的"+label)
	}
	return nil
}

// dictValue renders a field value as dictionary items store it; a nil
// pointer is "".
func dictValue(v any) string {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
	}
	return fmt.Sprint(rv.Interface())
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
// incrementing the version (design doc 8.1 optimistic locking). table is a
// constant, never user input.
func updateVersioned(tx *gorm.DB, table string, id int64, version int, sets map[string]any, onDuplicate func(constraint string) error) error {
	sets["version"] = gorm.Expr("version + 1")
	res := tx.Table(table).Where("id = ? AND version = ?", id, version).Updates(sets)
	if res.Error != nil {
		if constraint, ok := database.UniqueConstraint(res.Error); ok && onDuplicate != nil {
			return onDuplicate(constraint)
		}
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errVersionConflict
	}
	return nil
}

// uniqueField is a column with a unique index and its form field.
type uniqueField struct{ column, field, label string }

// duplicateError reports a unique violation as a 422 field error on the
// column it concerns, so the form shows it next to the input. (409 is kept
// for concurrent edits, which the client handles by reloading.)
func duplicateError(constraint string, fields []uniqueField, fallback string) error {
	var match *uniqueField
	for i := range fields {
		f := &fields[i]
		if strings.Contains(constraint, f.column) && (match == nil || len(f.column) > len(match.column)) {
			match = f
		}
	}
	if match == nil && len(fields) == 1 {
		match = &fields[0]
	}
	if match != nil {
		return fieldError(match.field, match.label+"已存在")
	}
	return httpx.NewError(http.StatusUnprocessableEntity, httpx.CodeValidation, fallback)
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
