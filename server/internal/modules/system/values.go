package system

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Config value types.
const (
	TypeString  = "string"
	TypeNumber  = "number"
	TypeBoolean = "boolean"
	TypeJSON    = "json"
)

// maxConfigValueLen bounds config values (characters).
const maxConfigValueLen = 4000

var (
	configKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)
	dictCodePattern  = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)
)

func validValueType(t string) bool {
	switch t {
	case TypeString, TypeNumber, TypeBoolean, TypeJSON:
		return true
	default:
		return false
	}
}

// normalizeValue validates value against valueType and returns its stored
// form: numbers, booleans and JSON are trimmed, strings are kept as
// entered. The second result is a user-facing error message ("" if valid).
func normalizeValue(valueType, value string) (string, string) {
	if utf8.RuneCountInString(value) > maxConfigValueLen {
		return "", "不能多于 " + strconv.Itoa(maxConfigValueLen) + " 个字符"
	}
	switch valueType {
	case TypeString:
		return value, ""
	case TypeNumber:
		v := strings.TrimSpace(value)
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return "", "必须是有效的数字"
		}
		return v, ""
	case TypeBoolean:
		v := strings.TrimSpace(value)
		if v != "true" && v != "false" {
			return "", "必须是 true 或 false"
		}
		return v, ""
	case TypeJSON:
		v := strings.TrimSpace(value)
		if !json.Valid([]byte(v)) {
			return "", "必须是合法的 JSON"
		}
		return v, ""
	default:
		return "", "无效的值类型"
	}
}
