package auth

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"gorm.io/gorm"
)

// Captcha modes of the security policy.
const (
	CaptchaOff           = "off"
	CaptchaAfterFailures = "after_failures"
	CaptchaAlways        = "always"
)

// Policy is the administrator-editable security policy (one row in
// sys_security_policy).
type Policy struct {
	PasswordMinLength     int    `json:"passwordMinLength" binding:"min=8,max=72"`
	PasswordRequireUpper  bool   `json:"passwordRequireUpper"`
	PasswordRequireLower  bool   `json:"passwordRequireLower"`
	PasswordRequireDigit  bool   `json:"passwordRequireDigit"`
	PasswordRequireSymbol bool   `json:"passwordRequireSymbol"`
	CaptchaMode           string `json:"captchaMode" binding:"oneof=off after_failures always"`
	CaptchaAfterFailures  int    `json:"captchaAfterFailures" binding:"min=1,max=20"`
	LockoutThreshold      int    `json:"lockoutThreshold" binding:"min=0,max=100"`
	LockoutMinutes        int    `json:"lockoutMinutes" binding:"min=1,max=1440"`
	Version               int    `json:"version"`
}

// defaultPolicy is used only if the policy row cannot be read; it matches
// the migration defaults.
var defaultPolicy = Policy{
	PasswordMinLength: 8, PasswordRequireDigit: true, CaptchaMode: CaptchaAfterFailures,
	CaptchaAfterFailures: 3, LockoutThreshold: 5, LockoutMinutes: 15,
}

// CheckPassword returns a Chinese description of the first unmet rule, or
// "" when pw satisfies the policy.
func (p Policy) CheckPassword(pw string) string {
	var upper, lower, digit, symbol bool
	for _, r := range pw {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			symbol = true
		}
	}
	var missing []string
	if p.PasswordRequireUpper && !upper {
		missing = append(missing, "大写字母")
	}
	if p.PasswordRequireLower && !lower {
		missing = append(missing, "小写字母")
	}
	if p.PasswordRequireDigit && !digit {
		missing = append(missing, "数字")
	}
	if p.PasswordRequireSymbol && !symbol {
		missing = append(missing, "特殊字符")
	}
	switch {
	case len([]rune(pw)) < p.PasswordMinLength:
		return fmt.Sprintf("密码至少 %d 位%s", p.PasswordMinLength, describeClasses(missing, "，且须包含"))
	case len(missing) > 0:
		return "密码须包含" + strings.Join(missing, "、")
	default:
		return ""
	}
}

func describeClasses(missing []string, prefix string) string {
	if len(missing) == 0 {
		return ""
	}
	return prefix + strings.Join(missing, "、")
}

// policyStore reads the policy with a short in-process cache; updates on
// this instance invalidate it immediately, other instances within the TTL.
type policyStore struct {
	db  *gorm.DB
	ttl time.Duration

	mu     sync.Mutex
	cached Policy
	at     time.Time
}

const policyColumns = `password_min_length, password_require_upper, password_require_lower,
	password_require_digit, password_require_symbol, captcha_mode, captcha_after_failures,
	lockout_threshold, lockout_minutes, version`

func (s *policyStore) get(ctx context.Context) (Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.at.IsZero() && time.Since(s.at) < s.ttl {
		return s.cached, nil
	}
	var p Policy
	res := s.db.WithContext(ctx).Raw("SELECT " + policyColumns + " FROM sys_security_policy WHERE id = 1").Scan(&p)
	if res.Error != nil {
		return Policy{}, fmt.Errorf("load security policy: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		p = defaultPolicy
	}
	s.cached, s.at = p, time.Now()
	return p, nil
}

func (s *policyStore) invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.at = time.Time{}
}
