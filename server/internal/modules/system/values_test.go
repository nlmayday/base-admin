package system

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeValue(t *testing.T) {
	cases := []struct {
		name, typ, in, want string
		ok                  bool
	}{
		{"string kept as is", TypeString, "  hello ", "  hello ", true},
		{"empty string", TypeString, "", "", true},
		{"integer", TypeNumber, " 42 ", "42", true},
		{"float", TypeNumber, "-3.5e2", "-3.5e2", true},
		{"not a number", TypeNumber, "12a", "", false},
		{"empty number", TypeNumber, "", "", false},
		{"infinity", TypeNumber, "Inf", "", false},
		{"nan", TypeNumber, "NaN", "", false},
		{"true", TypeBoolean, "true", "true", true},
		{"false trimmed", TypeBoolean, " false\n", "false", true},
		{"boolean must be lowercase", TypeBoolean, "TRUE", "", false},
		{"boolean rejects 1", TypeBoolean, "1", "", false},
		{"json object", TypeJSON, ` {"a": [1, 2]} `, `{"a": [1, 2]}`, true},
		{"json scalar", TypeJSON, "null", "null", true},
		{"broken json", TypeJSON, `{"a":`, "", false},
		{"empty json", TypeJSON, "", "", false},
		{"unknown type", "date", "2024-01-01", "", false},
		{"too long", TypeString, strings.Repeat("x", maxConfigValueLen+1), "", false},
		{"max length", TypeString, strings.Repeat("字", maxConfigValueLen), strings.Repeat("字", maxConfigValueLen), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, msg := normalizeValue(tc.typ, tc.in)
			if tc.ok {
				assert.Empty(t, msg)
				assert.Equal(t, tc.want, got)
			} else {
				assert.NotEmpty(t, msg)
			}
		})
	}
}

func TestValidValueType(t *testing.T) {
	for _, v := range []string{TypeString, TypeNumber, TypeBoolean, TypeJSON} {
		assert.True(t, validValueType(v), v)
	}
	for _, v := range []string{"", "String", "int", "bool"} {
		assert.False(t, validValueType(v), v)
	}
}

func TestValidConfigKey(t *testing.T) {
	for _, k := range []string{"site.name", "a", "upload.max_size_mb", "a1.b2_c.d"} {
		assert.True(t, validConfigKey(k), k)
	}
	for _, k := range []string{"", "Site.name", "1a", "a..b", "a.", ".a", "a-b", "a.1b", "a b",
		strings.Repeat("a", maxConfigKeyLen+1)} {
		assert.False(t, validConfigKey(k), k)
	}
}

func TestDictCodePattern(t *testing.T) {
	for _, c := range []string{"ab", "customer_level", "a1"} {
		assert.True(t, dictCodePattern.MatchString(c), c)
	}
	for _, c := range []string{"a", "Ab", "1a", "a-b", "a.b", strings.Repeat("a", 65)} {
		assert.False(t, dictCodePattern.MatchString(c), c)
	}
}
