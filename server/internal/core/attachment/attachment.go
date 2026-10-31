// Package attachment validates the attachment references business modules
// store: URLs returned by the file module's attachment upload
// ("<api prefix>/files/a/yyyy/mm/dd/<32 hex>.<ext>"). Accepting only these
// keeps arbitrary links (javascript:, other sites) out of the database.
package attachment

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxRefs bounds the attachments of one record.
const MaxRefs = 20

var urlPattern = regexp.MustCompile(`^(/[a-z0-9]+)*/files/a/\d{4}/\d{2}/\d{2}/[0-9a-f]{32}\.[a-z0-9]{1,16}$`)

// ValidURL reports whether u is an attachment URL.
func ValidURL(u string) bool { return urlPattern.MatchString(u) }

// Ref is one attachment of a record, stored as JSON (a jsonb column).
type Ref struct {
	Name string `json:"name" doc:"文件名"`
	URL  string `json:"url" doc:"上传附件返回的地址"`
	Size int64  `json:"size" doc:"字节数"`
}

// Validate checks refs from a request and returns them cleaned (names
// trimmed, never nil). The error message is user-facing.
func Validate(refs []Ref) ([]Ref, error) {
	if len(refs) > MaxRefs {
		return nil, fmt.Errorf("最多 %d 个附件", MaxRefs)
	}
	out := make([]Ref, 0, len(refs))
	for _, r := range refs {
		r.Name = strings.TrimSpace(r.Name)
		switch {
		case !ValidURL(r.URL):
			return nil, fmt.Errorf("附件地址无效，请重新上传")
		case r.Name == "" || utf8.RuneCountInString(r.Name) > 255:
			return nil, fmt.Errorf("附件名称不能为空且不超过 255 个字符")
		case r.Size < 0:
			return nil, fmt.Errorf("附件大小无效")
		}
		out = append(out, r)
	}
	return out, nil
}

// Helpers for generated modules: an image column stores one attachment URL
// (a string), an attachments column a JSON list of Ref (jsonb).

// URLProblem returns the user-facing problem with an image URL, or "" when
// it is empty or valid.
func URLProblem(u string) string {
	if u == "" || ValidURL(u) {
		return ""
	}
	return "图片地址无效，请重新上传"
}

// RefsProblem returns the user-facing problem with a list, or "".
func RefsProblem(refs []Ref) string {
	if _, err := Validate(refs); err != nil {
		return err.Error()
	}
	return ""
}

// JSON encodes refs for a jsonb column; nil becomes [].
func JSON(refs []Ref) string {
	clean, err := Validate(refs)
	if err != nil {
		clean = []Ref{} // callers validate first; never store invalid input
	}
	raw, _ := json.Marshal(clean)
	return string(raw)
}

// Decode reads a stored list; NULL, empty or malformed JSON is [].
func Decode(raw *string) []Ref {
	refs := []Ref{}
	if raw != nil && *raw != "" {
		if err := json.Unmarshal([]byte(*raw), &refs); err != nil || refs == nil {
			return []Ref{}
		}
	}
	return refs
}
