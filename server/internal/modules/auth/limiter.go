package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// reserveScript counts an attempt against both keys before any credential is
// checked, so concurrent bursts cannot exceed the budget (check-then-act
// races are impossible). It returns 1 when the attempt is over a limit.
var reserveScript = redis.NewScript(`
local blocked = 0
for i, key in ipairs(KEYS) do
  local n = redis.call('INCR', key)
  if n == 1 then redis.call('PEXPIRE', key, ARGV[1]) end
  if n > tonumber(ARGV[i + 1]) then blocked = 1 end
end
return blocked`)

// loginLimiter bounds credential guesses per client IP and per account in
// fixed windows (design doc 6.6).
type loginLimiter struct {
	rdb        *redis.Client
	window     time.Duration
	maxIP      int64
	maxAccount int64
}

// reserve records one attempt and reports whether it must be refused.
func (l *loginLimiter) reserve(ctx context.Context, ip, account string) (bool, error) {
	blocked, err := reserveScript.Run(ctx, l.rdb, []string{loginIPKey(ip), loginAccountKey(account)},
		l.window.Milliseconds(), l.maxIP, l.maxAccount).Int()
	if err != nil {
		return false, fmt.Errorf("reserve login attempt: %w", err)
	}
	return blocked == 1, nil
}

// reset clears the account budget after a successful attempt.
func (l *loginLimiter) reset(ctx context.Context, account string) error {
	return l.rdb.Del(ctx, loginAccountKey(account)).Err()
}

// captchaIssueLimit bounds captcha images per client IP and minute: drawing
// one costs CPU and a Redis key, and a login page needs only a few.
const (
	captchaIssueLimit  = 30
	captchaIssueWindow = time.Minute
)

func captchaIPKey(ip string) string { return "rl:captcha:ip:" + ip }

// reserveCaptcha records one captcha request and reports whether it must be
// refused.
func (l *loginLimiter) reserveCaptcha(ctx context.Context, ip string) (bool, error) {
	blocked, err := reserveScript.Run(ctx, l.rdb, []string{captchaIPKey(ip)},
		captchaIssueWindow.Milliseconds(), captchaIssueLimit).Int()
	if err != nil {
		return false, fmt.Errorf("reserve captcha: %w", err)
	}
	return blocked == 1, nil
}
