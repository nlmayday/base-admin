package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckPassword(t *testing.T) {
	strict := Policy{PasswordMinLength: 10, PasswordRequireUpper: true, PasswordRequireLower: true,
		PasswordRequireDigit: true, PasswordRequireSymbol: true}
	letters := Policy{PasswordMinLength: 10, PasswordRequireLetter: true, PasswordRequireDigit: true}
	cases := []struct {
		policy Policy
		pw     string
		want   string
	}{
		{defaultPolicy, "abcdefg1", ""},
		{defaultPolicy, "abcdefgh", "密码须包含数字"},
		{defaultPolicy, "abc1", "密码至少 8 位"},
		{strict, "Abcdefgh1!", ""},
		{strict, "abcdefgh1!", "密码须包含大写字母"},
		{strict, "Abc1!", "密码至少 10 位"},
		{strict, "abc1", "密码至少 10 位，且须包含大写字母、特殊字符"},
		{strict, "ABCDEFGHIJ", "密码须包含小写字母、数字、特殊字符"},
		{Policy{PasswordMinLength: 8}, "密码密码密码密码", ""},
		// 至少 10 位，含字母（大小写不限）和数字.
		{letters, "abcdefghi1", ""},
		{letters, "ABCDEFGHI1", ""},
		{letters, "1234567890", "密码须包含字母"},
		{letters, "密码密码密码密码密码1", "密码须包含字母"},
		{letters, "abcdefghij", "密码须包含数字"},
		{letters, "ab1", "密码至少 10 位"},
	}
	for _, tc := range cases {
		got := tc.policy.CheckPassword(tc.pw)
		if tc.want == "" {
			assert.Empty(t, got, tc.pw)
		} else {
			assert.Contains(t, got, tc.want, tc.pw)
		}
	}
}

func TestCaptchaRequired(t *testing.T) {
	assert.False(t, captchaRequired(Policy{CaptchaMode: CaptchaOff}, 100))
	assert.True(t, captchaRequired(Policy{CaptchaMode: CaptchaAlways}, 0))
	p := Policy{CaptchaMode: CaptchaAfterFailures, CaptchaAfterFailures: 3}
	assert.False(t, captchaRequired(p, 2))
	assert.True(t, captchaRequired(p, 3))
}
