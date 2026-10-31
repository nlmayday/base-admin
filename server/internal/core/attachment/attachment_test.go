package attachment

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const good = "/api/v1/files/a/2026/10/30/0123456789abcdef0123456789abcdef.pdf"

func TestValidURL(t *testing.T) {
	for u, want := range map[string]bool{
		good: true,
		"/files/a/2026/10/30/0123456789abcdef0123456789abcdef.png": true,
		"javascript:alert(1)": false,
		"https://evil.example/files/a/2026/10/30/0123456789abcdef0123456789abcdef.pdf": false,
		"/api/v1/files/a/2026/10/30/../../etc/passwd":                                  false,
		"/api/v1/files/a/2026/10/30/0123.pdf":                                          false,
		"/api/v1/files/1/download":                                                     false,
	} {
		assert.Equal(t, want, ValidURL(u), u)
	}
}

func TestValidate(t *testing.T) {
	out, err := Validate(nil)
	require.NoError(t, err)
	assert.NotNil(t, out, "never nil, so the column stores []")

	out, err = Validate([]Ref{{Name: " 制度.pdf ", URL: good, Size: 10}})
	require.NoError(t, err)
	assert.Equal(t, "制度.pdf", out[0].Name)

	_, err = Validate([]Ref{{Name: "x", URL: "javascript:alert(1)"}})
	assert.ErrorContains(t, err, "附件地址无效")
	_, err = Validate([]Ref{{Name: " ", URL: good}})
	assert.Error(t, err)
	_, err = Validate([]Ref{{Name: strings.Repeat("长", 256), URL: good}})
	assert.Error(t, err)
	_, err = Validate(make([]Ref, MaxRefs+1))
	assert.ErrorContains(t, err, "最多")
}

func TestHelpers(t *testing.T) {
	assert.Empty(t, URLProblem(""))
	assert.Empty(t, URLProblem(good))
	assert.NotEmpty(t, URLProblem("https://evil.example/x.png"))
	assert.Empty(t, RefsProblem(nil))
	assert.NotEmpty(t, RefsProblem([]Ref{{Name: "x", URL: "javascript:1"}}))

	assert.Equal(t, "[]", JSON(nil))
	raw := JSON([]Ref{{Name: " a.pdf ", URL: good, Size: 3}})
	assert.Equal(t, `[{"name":"a.pdf","url":"`+good+`","size":3}]`, raw)
	assert.Equal(t, []Ref{{Name: "a.pdf", URL: good, Size: 3}}, Decode(&raw))
	bad := "{"
	assert.Equal(t, []Ref{}, Decode(&bad))
	assert.Equal(t, []Ref{}, Decode(nil))
	null := "null"
	assert.Equal(t, []Ref{}, Decode(&null))
}
