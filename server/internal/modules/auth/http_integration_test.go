//go:build integration

package auth

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/testinfra"
)

func TestLoginSetsHardenedCookies(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("Alice")
	c := h.newClient()

	resp := c.login("alice") // usernames are case-insensitive
	require.Equal(t, http.StatusOK, resp.Status)
	assert.JSONEq(t, `{"userId":`+itoa(uid)+`}`, string(resp.Data))

	var sid, xsrf *http.Cookie
	for _, ck := range (&http.Response{Header: resp.Header}).Cookies() {
		switch ck.Name {
		case "sid":
			sid = ck
		case CSRFCookie:
			xsrf = ck
		}
	}
	require.NotNil(t, sid)
	require.NotNil(t, xsrf)
	assert.True(t, sid.HttpOnly, "session cookie must be HttpOnly")
	assert.False(t, xsrf.HttpOnly, "the XSRF cookie must be readable by JS")
	assert.Equal(t, http.SameSiteLaxMode, sid.SameSite)
	assert.Equal(t, "/", sid.Path)

	stored, err := h.rdb.Exists(context.Background(), sessionKey(sid.Value)).Result()
	require.NoError(t, err)
	assert.Zero(t, stored, "Redis stores only the token hash")
	assert.Equal(t, http.StatusOK, c.ping())
}

func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("bob")
	require.NoError(t, h.db.Exec("UPDATE sys_users SET status = 2 WHERE id = ?", uid).Error)
	h.createUser("carl")

	for name, body := range map[string]gin.H{
		"unknown user":   {"username": "nobody", "password": testPass},
		"wrong password": {"username": "carl", "password": "wrong-password"},
		"disabled user":  {"username": "bob", "password": testPass},
	} {
		resp := h.newClient().do(http.MethodPost, "/api/v1/auth/login", body)
		assert.Equal(t, http.StatusUnauthorized, resp.Status, name)
		assert.Equal(t, CodeBadCredentials, resp.Code, name)
	}
}

func TestLoginRateLimitPerAccount(t *testing.T) {
	h := newHarness(t)
	h.createUser("dora")
	c := h.newClient()
	for range h.cfg.LoginMaxPerAccount {
		resp := c.do(http.MethodPost, "/api/v1/auth/login", gin.H{"username": "dora", "password": "nope-nope"})
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	}
	resp := c.login("dora")
	assert.Equal(t, http.StatusTooManyRequests, resp.Status, "even the right password is refused while limited")
	assert.Equal(t, httpx.CodeTooManyRequests, resp.Code)
}

func TestCSRFProtection(t *testing.T) {
	h := newHarness(t)
	h.createUser("eve")
	c := h.newClient()
	c.mustLogin("eve")

	cases := map[string]func(*http.Request){
		"missing token":  func(r *http.Request) { r.Header.Del(CSRFHeader) },
		"wrong token":    func(r *http.Request) { r.Header.Set(CSRFHeader, "forged") },
		"foreign origin": func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") },
		"no origin":      func(r *http.Request) { r.Header.Del("Origin") },
		"bearer beside the session cookie": func(r *http.Request) {
			r.Header.Del(CSRFHeader)
			r.Header.Set("Authorization", "Bearer bak_x")
		},
	}
	for name, mutate := range cases {
		resp := c.do(http.MethodPost, "/api/v1/ping", gin.H{}, mutate)
		assert.Equal(t, http.StatusForbidden, resp.Status, name)
		assert.Equal(t, CodeCSRF, resp.Code, name)
	}
	referer := func(r *http.Request) { r.Header.Del("Origin"); r.Header.Set("Referer", origin+"/system/user") }
	assert.Equal(t, http.StatusOK, c.do(http.MethodPost, "/api/v1/ping", gin.H{}, referer).Status, "Referer fallback")
	assert.Equal(t, http.StatusOK, c.do(http.MethodPost, "/api/v1/ping", gin.H{}).Status)

	// An API key alone (no session cookie) is exempt from the CSRF check;
	// the route's own authentication then decides.
	keyOnly := h.newClient().do(http.MethodPost, "/api/v1/ping", gin.H{}, func(r *http.Request) {
		r.Header.Del("Origin")
		r.Header.Set("Authorization", "Bearer bak_x")
	})
	assert.NotEqual(t, CodeCSRF, keyOnly.Code, "bearer without a session cookie skips the CSRF check")

	foreignLogin := h.newClient().do(http.MethodPost, "/api/v1/auth/login", gin.H{"username": "eve", "password": testPass},
		func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") })
	assert.Equal(t, http.StatusForbidden, foreignLogin.Status, "login CSRF is blocked by the origin check")
}

func TestLogoutEndsSession(t *testing.T) {
	h := newHarness(t)
	h.createUser("finn")
	c := h.newClient()
	c.mustLogin("finn")
	token := c.cookie("sid")

	require.Equal(t, http.StatusOK, c.do(http.MethodPost, "/api/v1/auth/logout", nil).Status)
	assert.Empty(t, c.cookie("sid"), "cookie cleared")
	replay := h.newClient()
	replay.http.Jar.SetCookies(mustURL(h.server.URL), []*http.Cookie{{Name: "sid", Value: token}})
	assert.Equal(t, http.StatusUnauthorized, replay.ping(), "the old token is dead server-side")
}

func TestLoginReplacesPresentedSession(t *testing.T) {
	h := newHarness(t)
	h.createUser("gail")
	c := h.newClient()
	c.mustLogin("gail")
	first := c.cookie("sid")
	c.mustLogin("gail")
	n, err := h.rdb.Exists(context.Background(), sessionKey(HashToken(first))).Result()
	require.NoError(t, err)
	assert.Zero(t, n, "session fixation: the previous session is destroyed")
}

func TestAbsoluteAndIdleExpiry(t *testing.T) {
	h := newHarness(t)
	h.createUser("hank")
	c := h.newClient()
	c.mustLogin("hank")

	h.clock.now = h.clock.now.Add(h.cfg.TouchInterval + time.Second)
	require.Equal(t, http.StatusOK, c.ping())
	ttl, err := h.rdb.TTL(context.Background(), sessionKey(c.session().Hash)).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, h.cfg.IdleTimeout-time.Minute, "activity slides the idle expiry")

	h.clock.now = h.clock.now.Add(h.cfg.AbsoluteTimeout)
	assert.Equal(t, http.StatusUnauthorized, c.ping(), "absolute expiry cannot be extended")
}

func TestPermissionLevel(t *testing.T) {
	h := newHarness(t)
	h.createUser("ivy")
	c := h.newClient()
	assert.Equal(t, http.StatusUnauthorized, c.do(http.MethodGet, "/api/v1/secret", nil).Status)
	c.mustLogin("ivy")
	resp := c.do(http.MethodGet, "/api/v1/secret", nil)
	assert.Equal(t, http.StatusForbidden, resp.Status)
	assert.Equal(t, CodeForbidden, resp.Code)
}

func TestMustChangePasswordFlow(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("jack")
	require.NoError(t, h.db.Exec("UPDATE sys_users SET must_change_password = true WHERE id = ?", uid).Error)
	other := h.newClient()
	other.mustLogin("jack")
	c := h.newClient()
	c.mustLogin("jack")

	blocked := c.do(http.MethodGet, "/api/v1/ping", nil)
	assert.Equal(t, http.StatusForbidden, blocked.Status)
	assert.Equal(t, CodePasswordChangeRequired, blocked.Code)
	assert.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/v1/me", nil).Status, "/me stays available")

	wrongOld := c.do(http.MethodPut, "/api/v1/me/password", gin.H{"oldPassword": "bad", "newPassword": "new-password-1"})
	assert.Equal(t, http.StatusUnprocessableEntity, wrongOld.Status)
	same := c.do(http.MethodPut, "/api/v1/me/password", gin.H{"oldPassword": testPass, "newPassword": testPass})
	assert.Equal(t, http.StatusUnprocessableEntity, same.Status)

	oldSid := c.cookie("sid")
	ok := c.do(http.MethodPut, "/api/v1/me/password", gin.H{"oldPassword": testPass, "newPassword": "new-password-1"})
	require.Equal(t, http.StatusOK, ok.Status)
	assert.NotEqual(t, oldSid, c.cookie("sid"), "a fresh session is issued")
	assert.Equal(t, http.StatusOK, c.ping(), "this device stays logged in and is unblocked")
	assert.Equal(t, http.StatusUnauthorized, other.ping(), "other devices are logged out")
	assert.Equal(t, h.pgGen(uid), c.session().Gen)

	relogin := h.newClient().do(http.MethodPost, "/api/v1/auth/login", gin.H{"username": "jack", "password": "new-password-1"})
	assert.Equal(t, http.StatusOK, relogin.Status)
}

func TestSessionGenerationNeverDecreasesInDatabase(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("kate")
	_, err := h.core.Revoker.Revoke(context.Background(), uid, RevokeSpec{})
	require.NoError(t, err)
	if testinfra.IsMySQL(t) {
		// No trigger on MySQL (it would need SUPER with binary logging):
		// the migrations create none, and revocations keep raising the
		// generation (GREATEST in the UPDATE).
		var triggers int
		require.NoError(t, h.db.Raw("SELECT count(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA = DATABASE()").Scan(&triggers).Error)
		assert.Zero(t, triggers)
		before := h.pgGen(uid)
		_, err = h.core.Revoker.Revoke(context.Background(), uid, RevokeSpec{})
		require.NoError(t, err)
		assert.Greater(t, h.pgGen(uid), before)
		return
	}
	err = h.db.Exec("UPDATE sys_users SET session_gen = 1 WHERE id = ?", uid).Error
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "must not decrease"), err.Error())
}

func TestConcurrentSnapshotRebuildsAreMerged(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("liam")
	ctx := context.Background()
	ver, err := h.core.Versions.Current(ctx)
	require.NoError(t, err)

	results := make(chan error, 20)
	for range 20 {
		go func() {
			_, err := h.core.Snapshots.Load(ctx, ver, uid)
			results <- err
		}()
	}
	for range 20 {
		require.NoError(t, <-results)
	}
	keys, err := h.rdb.Keys(ctx, "auth:snapshot:*").Result()
	require.NoError(t, err)
	assert.Len(t, keys, 1)
}

func TestLoginBurstCannotExceedBudget(t *testing.T) {
	h := newHarness(t)
	h.createUser("burst")
	results := make(chan int, 40)
	for range 40 {
		go func() {
			results <- h.newClient().do(http.MethodPost, "/api/v1/auth/login",
				gin.H{"username": "burst", "password": "wrong-password"}).Status
		}()
	}
	processed := 0
	for range 40 {
		if <-results == http.StatusUnauthorized {
			processed++
		}
	}
	assert.Equal(t, h.cfg.LoginMaxPerAccount, processed, "attempts are reserved before verification, so a burst gets exactly the budget")
}

func TestPasswordChangeIsRateLimited(t *testing.T) {
	h := newHarness(t)
	h.createUser("pat")
	c := h.newClient()
	c.mustLogin("pat")
	var last int
	for range h.cfg.LoginMaxPerAccount + 1 {
		last = c.do(http.MethodPut, "/api/v1/me/password", gin.H{"oldPassword": "guess-guess", "newPassword": "new-password-1"}).Status
	}
	assert.Equal(t, http.StatusTooManyRequests, last)
}
