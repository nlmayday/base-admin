package audit

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCSVCell(t *testing.T) {
	cases := map[string]string{
		"=SUM(A1)":   "'=SUM(A1)",
		"+1":         "'+1",
		"-2":         "'-2",
		"@cmd":       "'@cmd",
		"\tx":        "'\tx",
		"\rx":        "'\rx",
		"plain":      "plain",
		"":           "",
		"a=b":        "a=b",
		"192.0.2.10": "192.0.2.10",
	}
	for in, want := range cases {
		assert.Equal(t, want, csvCell(in), in)
	}
}

func TestEscapeLike(t *testing.T) {
	assert.Equal(t, `50\%\_a\\b`, escapeLike(`50%_a\b`))
}

func TestParseSuccess(t *testing.T) {
	v, err := parseSuccess("")
	require.NoError(t, err)
	assert.Nil(t, v)
	v, err = parseSuccess("true")
	require.NoError(t, err)
	assert.True(t, *v)
	v, err = parseSuccess("0")
	require.NoError(t, err)
	assert.False(t, *v)
	_, err = parseSuccess("maybe")
	assert.Error(t, err)
}

func TestParseCleanupParams(t *testing.T) {
	cases := []struct {
		raw     string
		days    int
		wantErr bool
	}{
		{"", defaultRetainDays, false},
		{"null", defaultRetainDays, false},
		{"{}", defaultRetainDays, false},
		{`{"days":30}`, 30, false},
		{`{"days":0}`, 0, true},
		{`{"days":3}`, 0, true}, // below the retention floor
		{`{"days":-1}`, 0, true},
		{`{"days":"x"}`, 0, true},
		{`[1]`, 0, true},
	}
	for _, tc := range cases {
		p, err := parseCleanupParams(json.RawMessage(tc.raw), defaultRetainDays)
		if tc.wantErr {
			assert.Error(t, err, tc.raw)
			continue
		}
		require.NoError(t, err, tc.raw)
		assert.Equal(t, tc.days, p.Days, tc.raw)
	}
}

func TestParseCleanupParamsFallback(t *testing.T) {
	p, err := parseCleanupParams(nil, 30)
	require.NoError(t, err)
	assert.Equal(t, 30, p.Days)
	p, err = parseCleanupParams(json.RawMessage(`{"days":7}`), 30)
	require.NoError(t, err)
	assert.Equal(t, 7, p.Days, "explicit params override the setting")
}

func TestRetentionDays(t *testing.T) {
	m := New(Deps{})
	assert.Equal(t, defaultRetainDays, m.retentionDays(context.Background()))
	m = New(Deps{RetentionDays: func(context.Context) (int, error) { return 90, nil }})
	assert.Equal(t, 90, m.retentionDays(context.Background()))
	m = New(Deps{RetentionDays: func(context.Context) (int, error) { return 0, errors.New("boom") }})
	assert.Equal(t, defaultRetainDays, m.retentionDays(context.Background()))
}

func TestReasonLabel(t *testing.T) {
	assert.Equal(t, "用户名或密码错误", reasonLabel("bad_credentials"))
	assert.Equal(t, "账号已锁定", reasonLabel("locked"))
	assert.Equal(t, "custom", reasonLabel("custom"))
}
