package httpx

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

var registerOnce sync.Once

// RegisterJSONFieldNames makes validation errors report JSON field names
// instead of Go field names. It is safe to call more than once.
func RegisterJSONFieldNames() error {
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return errors.New("gin validator engine is not go-playground/validator")
	}
	registerOnce.Do(func() {
		v.RegisterTagNameFunc(jsonFieldName)
	})
	return nil
}

func jsonFieldName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	switch name {
	case "-":
		return ""
	case "":
		return f.Name
	default:
		return name
	}
}

// BindJSON decodes and validates the JSON body into dst, returning an API
// error (400, 413 or 422) on failure.
func BindJSON(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		return translateBindError(err)
	}
	return nil
}

func translateBindError(err error) error {
	if verrs, ok := errors.AsType[validator.ValidationErrors](err); ok {
		fields := make([]FieldError, 0, len(verrs))
		for _, fe := range verrs {
			fields = append(fields, FieldError{Field: fieldPath(fe), Message: ruleMessage(fe)})
		}
		return Validation(fields)
	}
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return PayloadTooLarge().WithCause(err)
	}
	return BadRequest("请求格式错误").WithCause(err)
}

// fieldPath drops the root struct name from the namespace, e.g.
// "createUserRequest.profile.email" becomes "profile.email".
func fieldPath(fe validator.FieldError) string {
	if _, rest, found := strings.Cut(fe.Namespace(), "."); found {
		return rest
	}
	return fe.Field()
}

func ruleMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "不能为空"
	case "min":
		return boundMessage(fe, "不能少于", "不能小于")
	case "max":
		return boundMessage(fe, "不能多于", "不能大于")
	case "len":
		return "长度必须为 " + fe.Param()
	case "email":
		return "邮箱格式不正确"
	case "oneof":
		return "取值必须是以下之一：" + fe.Param()
	case "url":
		return "地址格式不正确"
	default:
		return fmt.Sprintf("不符合 %q 规则", fe.Tag())
	}
}

// boundMessage words min/max by kind: characters for strings, items for
// lists, values for numbers.
func boundMessage(fe validator.FieldError, sizeWord, valueWord string) string {
	switch fe.Kind() {
	case reflect.String:
		return sizeWord + " " + fe.Param() + " 个字符"
	case reflect.Slice, reflect.Array, reflect.Map:
		return sizeWord + " " + fe.Param() + " 项"
	default:
		return valueWord + " " + fe.Param()
	}
}
