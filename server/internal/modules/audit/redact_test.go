package audit

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"top level", `{"username":"bob","password":"hunter2"}`, `{"password":"***","username":"bob"}`},
		{"case insensitive", `{"newPassword":"a","OldPASSWORD":"b","accessToken":"c","clientSecret":"d","captchaCode":"e"}`,
			`{"OldPASSWORD":"***","accessToken":"***","captchaCode":"***","clientSecret":"***","newPassword":"***"}`},
		{"nested and arrays", `{"items":[{"token":"t","id":"1"}],"cfg":{"secret":{"deep":1}}}`,
			`{"cfg":{"secret":"***"},"items":[{"id":"1","token":"***"}]}`},
		{"keeps numbers exact", `{"id":12345678901234567890,"ratio":0.1}`, `{"id":12345678901234567890,"ratio":0.1}`},
		{"no html escaping", `{"name":"<a&b>"}`, `{"name":"<a&b>"}`},
		{"array root", `[{"password":"x"}]`, `[{"password":"***"}]`},
		{"scalar root", `"password"`, `"password"`},
		{"more secret names", `{"pwd":"a","apiKey":"b","privateKey":"c","Authorization":"d","otp":"e","credentials":"f"}`,
			`{"Authorization":"***","apiKey":"***","credentials":"***","otp":"***","privateKey":"***","pwd":"***"}`},
		{"value named by key", `{"key":"smtp.password","value":"hunter2","valueType":"string"}`,
			`{"key":"smtp.password","value":"***","valueType":"string"}`},
		{"value named in Chinese", `{"name":"SMTP 密码","value":"hunter2"}`, `{"name":"SMTP 密码","value":"***"}`},
		{"ordinary value kept", `{"key":"site.name","value":"Base Admin"}`, `{"key":"site.name","value":"Base Admin"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := redactJSON([]byte(tc.in))
			require.True(t, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRedactJSONRejectsInvalid(t *testing.T) {
	for _, in := range []string{`{"a":`, `{"a":1} {"b":2}`, `password=x`} {
		_, ok := redactJSON([]byte(in))
		assert.False(t, ok, in)
	}
}

func TestRedactValueDoesNotMutateInput(t *testing.T) {
	in := map[string]any{"password": "x", "nested": map[string]any{"token": "y"}}
	_ = redactValue(in)
	assert.Equal(t, "x", in["password"])
	assert.Equal(t, "y", in["nested"].(map[string]any)["token"])
}

func newJSONRequest(body, contentType string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	return r
}

func TestCaptureBodyRestoresBody(t *testing.T) {
	body := `{"username":"bob","password":"hunter2"}`
	r := newJSONRequest(body, "application/json; charset=utf-8")
	got := captureBody(r)
	assert.Equal(t, `{"password":"***","username":"bob"}`, got)
	rest, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	assert.Equal(t, body, string(rest), "the handler sees the original body")
	require.NoError(t, r.Body.Close())
}

func TestCaptureBodyTooLarge(t *testing.T) {
	payload := map[string]string{"password": "hunter2", "data": strings.Repeat("x", 5000)}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	r := newJSONRequest(string(raw), "application/json")
	assert.Equal(t, bodyTooLarge, captureBody(r), "a truncated body cannot be redacted, so it is not stored")
	rest, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	assert.Equal(t, string(raw), string(rest), "the full body is restored")
}

func TestCaptureBodyExactlyAtLimit(t *testing.T) {
	prefix, suffix := `{"a":"`, `"}`
	body := prefix + strings.Repeat("y", maxBodyBytes-len(prefix)-len(suffix)) + suffix
	require.Len(t, body, maxBodyBytes)
	got := captureBody(newJSONRequest(body, "application/json"))
	assert.Equal(t, body, got)
}

func TestCaptureBodySkipsNonJSON(t *testing.T) {
	cases := map[string]string{
		"multipart": "multipart/form-data; boundary=x",
		"form":      "application/x-www-form-urlencoded",
		"missing":   "",
	}
	for name, ct := range cases {
		t.Run(name, func(t *testing.T) {
			r := newJSONRequest("password=secret", ct)
			assert.Empty(t, captureBody(r))
			rest, _ := io.ReadAll(r.Body)
			assert.Equal(t, "password=secret", string(rest), "body untouched")
		})
	}
}

func TestCaptureBodyInvalidJSONIsNotStored(t *testing.T) {
	r := newJSONRequest(`{"password":"hunter2"`, "application/json")
	assert.Equal(t, bodyInvalid, captureBody(r))
}

func TestCaptureBodyEmpty(t *testing.T) {
	r := httptest.NewRequest(http.MethodDelete, "/x", nil)
	r.Header.Set("Content-Type", "application/json")
	assert.Empty(t, captureBody(r))
	assert.Empty(t, captureBody(newJSONRequest("  ", "application/json")))
	assert.Equal(t, "{}", captureBody(newJSONRequest("{}", "application/vnd.api+json")), "+json types are captured")
}

func TestCleanAndTruncate(t *testing.T) {
	assert.Equal(t, "ab", clean("a\x00b", 10))
	assert.Equal(t, "用户", clean("用户名", 2), "counts characters, not bytes")
	assert.Equal(t, "a�b", clean("a\xffb", 10))
	assert.Equal(t, "ab", truncate("ab用", 4), "never splits a UTF-8 sequence")
	assert.Equal(t, "ab用", truncate("ab用", 5))
}
