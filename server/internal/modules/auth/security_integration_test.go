//go:build integration

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/httpx"
)

func (h *harness) setPolicy(sql string, args ...any) {
	h.t.Helper()
	require.NoError(h.t, h.db.Exec("UPDATE sys_security_policy SET "+sql+" WHERE id = 1", args...).Error)
	h.core.policies.invalidate()
}

func wrongLogin(c *client, user string, extra gin.H) response {
	body := gin.H{"username": user, "password": "wrong-password"}
	for k, v := range extra {
		body[k] = v
	}
	return c.do(http.MethodPost, "/api/v1/auth/login", body)
}

func captchaFor(t *testing.T, h *harness) (string, string) {
	t.Helper()
	id, _, err := h.core.captchas.issue(context.Background())
	require.NoError(t, err)
	answer, err := h.rdb.Get(context.Background(), captchaKey(id)).Result()
	require.NoError(t, err)
	return id, answer
}

func TestCaptchaRequiredAfterFailures(t *testing.T) {
	h := newHarness(t)
	h.setPolicy("captcha_after_failures = 2, lockout_threshold = 0")
	h.createUser("cara")
	c := h.newClient()

	first := wrongLogin(c, "cara", nil)
	assert.JSONEq(t, `{"captchaRequired":false}`, string(first.Data))
	second := wrongLogin(c, "cara", nil)
	assert.JSONEq(t, `{"captchaRequired":true}`, string(second.Data), "the response tells the UI to show the captcha")

	noCaptcha := c.login("cara")
	assert.Equal(t, CodeCaptchaRequired, noCaptcha.Code, "even the right password needs the captcha now")

	id, answer := captchaFor(t, h)
	resp := c.do(http.MethodPost, "/api/v1/auth/login", gin.H{"username": "cara", "password": testPass,
		"captchaId": id, "captchaCode": answer})
	require.Equal(t, http.StatusOK, resp.Status)

	reused := c.do(http.MethodPost, "/api/v1/auth/login", gin.H{"username": "cara", "password": testPass,
		"captchaId": id, "captchaCode": answer})
	assert.Equal(t, http.StatusOK, reused.Status, "success clears the failure count, so no captcha is needed")
}

func TestCaptchaIsSingleUse(t *testing.T) {
	h := newHarness(t)
	id, answer := captchaFor(t, h)
	ok, err := h.core.captchas.verify(context.Background(), id, answer)
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = h.core.captchas.verify(context.Background(), id, answer)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestCaptchaEndpointAndLoginOptions(t *testing.T) {
	h := newHarness(t)
	c := h.newClient()
	resp := c.do(http.MethodGet, "/api/v1/auth/captcha", nil)
	require.Equal(t, http.StatusOK, resp.Status)
	var cv captchaView
	require.NoError(t, json.Unmarshal(resp.Data, &cv))
	assert.NotEmpty(t, cv.ID)
	assert.Contains(t, cv.Image, "data:image/png;base64,")

	assert.JSONEq(t, `{"captchaAlways":false}`, string(c.do(http.MethodGet, "/api/v1/auth/login-options", nil).Data))
	h.setPolicy("captcha_mode = 'always'")
	assert.JSONEq(t, `{"captchaAlways":true}`, string(c.do(http.MethodGet, "/api/v1/auth/login-options", nil).Data))
}

func TestLockoutAndUnlock(t *testing.T) {
	h := newHarness(t)
	h.setPolicy("captcha_mode = 'off', lockout_threshold = 3, lockout_minutes = 10")
	h.createUser("lou")
	c := h.newClient()
	for range 3 {
		require.Equal(t, CodeBadCredentials, wrongLogin(c, "lou", nil).Code)
	}
	locked := c.login("lou")
	assert.Equal(t, http.StatusForbidden, locked.Status)
	assert.Equal(t, CodeAccountLocked, locked.Code, "the right password is refused while locked")

	// The lock only binds the client that failed: the same account still
	// logs in from another address.
	other := h.newClient()
	fromElsewhere := other.do(http.MethodPost, "/api/v1/auth/login", gin.H{"username": "lou", "password": testPass},
		func(r *http.Request) { r.Header.Set("X-Forwarded-For", "203.0.113.9") })
	assert.Equal(t, http.StatusOK, fromElsewhere.Status, "an attacker cannot lock the user out everywhere")

	require.NoError(t, h.core.lockout.unlock(context.Background(), "lou"))
	assert.Equal(t, http.StatusOK, c.login("lou").Status)
}

// Failures from one address make every address solve a captcha, while the
// hard lock stays on the failing address.
func TestFailuresElsewhereRequireCaptcha(t *testing.T) {
	h := newHarness(t)
	h.setPolicy("captcha_mode = 'after_failures', captcha_after_failures = 2, lockout_threshold = 3, lockout_minutes = 10")
	h.createUser("cleo")
	attacker := h.newClient()
	for range 2 {
		require.Equal(t, CodeBadCredentials, wrongLogin(attacker, "cleo", nil).Code)
	}
	user := h.newClient()
	viaOtherIP := func(r *http.Request) { r.Header.Set("X-Forwarded-For", "198.51.100.7") }
	noCaptcha := user.do(http.MethodPost, "/api/v1/auth/login", gin.H{"username": "cleo", "password": testPass}, viaOtherIP)
	assert.Equal(t, CodeCaptchaRequired, noCaptcha.Code)

	id, code := captchaFor(t, h)
	ok := user.do(http.MethodPost, "/api/v1/auth/login",
		gin.H{"username": "cleo", "password": testPass, "captchaId": id, "captchaCode": code}, viaOtherIP)
	assert.Equal(t, http.StatusOK, ok.Status)
}

func TestUnlockEscapesPatterns(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := Policy{LockoutThreshold: 1, LockoutMinutes: 10}
	_, err := h.core.lockout.recordFailure(ctx, "bob", "10.0.0.1", p)
	require.NoError(t, err)
	// "b*" must not match bob's keys.
	require.NoError(t, h.core.lockout.unlock(ctx, "b*"))
	locked, err := h.core.lockout.locked(ctx, "bob", "10.0.0.1")
	require.NoError(t, err)
	assert.Positive(t, locked)
}

func TestPasswordPolicyOnChange(t *testing.T) {
	h := newHarness(t)
	h.setPolicy("password_min_length = 12, password_require_upper = true")
	h.createUser("pam")
	c := h.newClient()
	c.mustLogin("pam")
	weak := c.do(http.MethodPut, "/api/v1/me/password", gin.H{"oldPassword": testPass, "newPassword": "lower-12"})
	assert.Equal(t, http.StatusUnprocessableEntity, weak.Status)
	assert.Contains(t, string(weak.Data), "至少 12 位")
	ok := c.do(http.MethodPut, "/api/v1/me/password", gin.H{"oldPassword": testPass, "newPassword": "Uppercase-1234"})
	assert.Equal(t, http.StatusOK, ok.Status)

	err := h.core.ResetPassword(context.Background(), 1, "short", nil)
	assert.Equal(t, httpx.CodeValidation, httpx.AsError(err).Code)
}

func TestOnlineSessionsListAndRevoke(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("sam")
	a, b := h.newClient(), h.newClient()
	a.mustLogin("sam")
	b.mustLogin("sam")

	all, err := h.core.Sessions.Active(context.Background(), h.clock.now)
	require.NoError(t, err)
	require.Len(t, all, 2)
	valid := (&Module{core: h.core}).validSessions(context.Background(), all)
	assert.Len(t, valid, 2)

	require.NoError(t, h.core.Sessions.Delete(context.Background(), b.session()))
	assert.Equal(t, http.StatusUnauthorized, b.ping(), "single-session revoke")
	assert.Equal(t, http.StatusOK, a.ping())

	_, err = h.core.Revoker.Revoke(context.Background(), uid, RevokeSpec{})
	require.NoError(t, err)
	all, err = h.core.Sessions.Active(context.Background(), h.clock.now)
	require.NoError(t, err)
	assert.Empty(t, (&Module{core: h.core}).validSessions(context.Background(), all),
		"revoked-by-generation sessions are hidden even when their keys survive")
}

func TestPolicyUpdateVersioned(t *testing.T) {
	h := newHarness(t)
	p, err := h.core.policies.get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 8, p.PasswordMinLength)
	res := h.db.Exec(`UPDATE sys_security_policy SET password_min_length = 9, version = version + 1 WHERE id = 1 AND version = ?`, p.Version)
	require.NoError(t, res.Error)
	assert.EqualValues(t, 1, res.RowsAffected)
	res = h.db.Exec(`UPDATE sys_security_policy SET password_min_length = 10, version = version + 1 WHERE id = 1 AND version = ?`, p.Version)
	assert.EqualValues(t, 0, res.RowsAffected, "stale version")
	assert.Error(t, h.db.Exec("UPDATE sys_security_policy SET password_min_length = 4 WHERE id = 1").Error, "check constraint")
}

func TestCaptchaIssuingIsRateLimited(t *testing.T) {
	h := newHarness(t)
	c := h.newClient()
	for range captchaIssueLimit {
		require.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/v1/auth/captcha", nil).Status)
	}
	assert.Equal(t, http.StatusTooManyRequests, c.do(http.MethodGet, "/api/v1/auth/captcha", nil).Status)
}
