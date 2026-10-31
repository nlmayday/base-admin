package audit

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

// maxBodyBytes is the largest request body captured in the operation log.
const maxBodyBytes = 4096

// Markers stored instead of a body that cannot be captured safely. A body
// that cannot be parsed cannot be redacted, so it is never stored raw.
const (
	bodyTooLarge = "[请求体超过 4 KB，未记录]"
	bodyInvalid  = "[请求体不是有效的 JSON，未记录]"
	redacted     = "***"
)

// sensitiveKey matches JSON keys whose values are never logged.
var sensitiveKey = regexp.MustCompile(
	`(?i)passw(or)?d|pwd|token|secret|captcha|credential|authorization|cookie|otp|` +
		`private_?key|access_?key|api_?key|signing_?key`)

// sensitiveLabel matches the key or name of a key/value pair (such as a
// system parameter {"key":"smtp.password","value":"…"} or {"name":"SMTP 密码",
// "value":"…"}) whose value must not be logged.
var sensitiveLabel = regexp.MustCompile(sensitiveKey.String() + `|密码|密钥|口令|令牌|凭据`)

// labelFields are the sibling fields that name a "value" field.
var labelFields = []string{"key", "name", "code"}

// readCloser restores a partially read body for the handler.
type readCloser struct {
	io.Reader
	io.Closer
}

// captureBody reads a JSON request body of at most maxBodyBytes and
// restores the full body for the handler. It returns the redacted body,
// a marker when the body could not be captured safely, or "" when there
// is no JSON body (multipart and other content types are not captured).
func captureBody(r *http.Request) string {
	if r.Body == nil || r.Body == http.NoBody || !isJSON(r.Header.Get("Content-Type")) {
		return ""
	}
	orig := r.Body
	data, err := io.ReadAll(io.LimitReader(orig, maxBodyBytes+1))
	// The remaining bytes (and any read error) still come from orig, so
	// body size limits keep working for the handler.
	r.Body = readCloser{Reader: io.MultiReader(bytes.NewReader(data), orig), Closer: orig}
	switch {
	case len(data) > maxBodyBytes:
		return bodyTooLarge
	case err != nil:
		return bodyInvalid
	case len(bytes.TrimSpace(data)) == 0:
		return ""
	}
	out, ok := redactJSON(data)
	if !ok {
		return bodyInvalid
	}
	return truncate(out, maxBodyBytes)
}

func isJSON(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mt == "application/json" || strings.HasSuffix(mt, "+json")
}

// redactJSON replaces the values of sensitive keys at any depth with "***".
// It reports false when data is not a single valid JSON value.
func redactJSON(data []byte) (string, bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return "", false
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(redactValue(v)); err != nil {
		return "", false
	}
	return strings.TrimSuffix(buf.String(), "\n"), true
}

// redactValue returns a redacted copy of v; v itself is not modified.
func redactValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		hideValue := namesSecret(t)
		for k, val := range t {
			if sensitiveKey.MatchString(k) || (hideValue && k == "value") {
				out[k] = redacted
			} else {
				out[k] = redactValue(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redactValue(val)
		}
		return out
	default:
		return v
	}
}

// namesSecret reports whether the object's key, name or code says that its
// "value" field holds a secret.
func namesSecret(obj map[string]any) bool {
	for _, f := range labelFields {
		if label, ok := obj[f].(string); ok && sensitiveLabel.MatchString(label) {
			return true
		}
	}
	return false
}

// clean makes s safe for a varchar(max) column: valid UTF-8, no NUL bytes
// (PostgreSQL rejects them) and at most max characters.
func clean(s string, maxChars int) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	if strings.IndexByte(s, 0) >= 0 {
		s = strings.ReplaceAll(s, "\x00", "")
	}
	if utf8.RuneCountInString(s) <= maxChars {
		return s
	}
	return string([]rune(s)[:maxChars])
}

// truncate cuts s to at most maxBytes without splitting a UTF-8 sequence.
func truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
