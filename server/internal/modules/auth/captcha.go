package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mojocn/base64Captcha"
	"github.com/redis/go-redis/v9"
)

const captchaTTL = 2 * time.Minute

func captchaKey(id string) string   { return "auth:captcha:" + id }
func failKey(account string) string { return "auth:fail:" + account }

// The hard lock is per account and client IP, so an attacker cannot lock a
// user out from everywhere; the account-wide counter above only demands a
// captcha.
func lockKey(account, ip string) string   { return "auth:lock:" + account + ":" + ip }
func failIPKey(account, ip string) string { return "auth:failip:" + account + ":" + ip }
func lockPattern(account string) string   { return "auth:lock:" + escapeGlob(account) + ":*" }
func failIPPattern(account string) string { return "auth:failip:" + escapeGlob(account) + ":*" }

// escapeGlob quotes Redis SCAN pattern metacharacters in user input.
func escapeGlob(s string) string {
	return strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`, `]`, `\]`).Replace(s)
}

// captchas issues one-time image captchas stored in Redis.
type captchas struct {
	rdb    *redis.Client
	driver *base64Captcha.DriverDigit
}

func newCaptchas(rdb *redis.Client) *captchas {
	return &captchas{rdb: rdb, driver: base64Captcha.NewDriverDigit(48, 140, 4, 0.5, 40)}
}

// issue returns a new captcha id and its PNG as a data URI.
func (c *captchas) issue(ctx context.Context) (string, string, error) {
	id, question, answer := c.driver.GenerateIdQuestionAnswer()
	item, err := c.driver.DrawCaptcha(question)
	if err != nil {
		return "", "", fmt.Errorf("draw captcha: %w", err)
	}
	if err := c.rdb.Set(ctx, captchaKey(id), answer, captchaTTL).Err(); err != nil {
		return "", "", fmt.Errorf("store captcha: %w", err)
	}
	return id, item.EncodeB64string(), nil
}

// verify consumes the captcha: each id can be checked once.
func (c *captchas) verify(ctx context.Context, id, code string) (bool, error) {
	if id == "" || code == "" {
		return false, nil
	}
	answer, err := c.rdb.GetDel(ctx, captchaKey(id)).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check captcha: %w", err)
	}
	got := strings.TrimSpace(code)
	return subtle.ConstantTimeCompare([]byte(got), []byte(answer)) == 1, nil
}

// lockout tracks login failures. The account-wide count (all IPs) drives
// the captcha requirement; the count per account and IP drives the
// temporary lock, which therefore only blocks the client that failed.
type lockout struct {
	rdb *redis.Client
}

// locked reports whether the account is locked for this IP and for how long.
func (l *lockout) locked(ctx context.Context, account, ip string) (time.Duration, error) {
	ttl, err := l.rdb.TTL(ctx, lockKey(account, ip)).Result()
	if err != nil {
		return 0, fmt.Errorf("check lock: %w", err)
	}
	return max(ttl, 0), nil
}

// failures returns the account-wide failure count.
func (l *lockout) failures(ctx context.Context, account string) (int64, error) {
	n, err := l.rdb.Get(ctx, failKey(account)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return n, err
}

// recordFailure counts a failed attempt and locks the account for this IP
// when the policy threshold is reached. It returns the account-wide count.
func (l *lockout) recordFailure(ctx context.Context, account, ip string, p Policy) (int64, error) {
	window := time.Duration(p.LockoutMinutes) * time.Minute
	pipe := l.rdb.TxPipeline()
	total := pipe.Incr(ctx, failKey(account))
	pipe.Expire(ctx, failKey(account), window)
	perIP := pipe.Incr(ctx, failIPKey(account, ip))
	pipe.Expire(ctx, failIPKey(account, ip), window)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("count login failure: %w", err)
	}
	if p.LockoutThreshold > 0 && perIP.Val() >= int64(p.LockoutThreshold) {
		pipe := l.rdb.TxPipeline()
		pipe.Set(ctx, lockKey(account, ip), 1, window)
		pipe.Del(ctx, failIPKey(account, ip))
		if _, err := pipe.Exec(ctx); err != nil {
			return total.Val(), fmt.Errorf("lock account: %w", err)
		}
	}
	return total.Val(), nil
}

// clear resets the counters after a successful login from ip.
func (l *lockout) clear(ctx context.Context, account, ip string) error {
	return l.rdb.Del(ctx, failKey(account), failIPKey(account, ip), lockKey(account, ip)).Err()
}

// unlock lifts every lock on the account and resets its counters
// (administrator action).
func (l *lockout) unlock(ctx context.Context, account string) error {
	keys := []string{failKey(account)}
	for _, pattern := range []string{lockPattern(account), failIPPattern(account)} {
		iter := l.rdb.Scan(ctx, 0, pattern, 100).Iterator()
		for iter.Next(ctx) {
			keys = append(keys, iter.Val())
		}
		if err := iter.Err(); err != nil {
			return fmt.Errorf("find account locks: %w", err)
		}
	}
	return l.rdb.Del(ctx, keys...).Err()
}

// captchaRequired applies the policy to the account's failure count.
func captchaRequired(p Policy, failures int64) bool {
	switch p.CaptchaMode {
	case CaptchaAlways:
		return true
	case CaptchaAfterFailures:
		return failures >= int64(p.CaptchaAfterFailures)
	default:
		return false
	}
}
