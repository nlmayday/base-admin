//go:build integration

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/testinfra/dbtest"
)

// recorder keeps the login events.
type recorder struct {
	mu     sync.Mutex
	events []LoginEvent
}

func (r *recorder) RecordLogin(_ context.Context, e LoginEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) last() LoginEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.events[len(r.events)-1]
}

// grantRole gives uid a new role with the given code.
func (h *harness) grantRole(uid int64, code string) {
	h.t.Helper()
	var rid int64
	var existing []int64
	require.NoError(h.t, h.db.Raw(`SELECT id FROM sys_roles WHERE code = ?`, code).Scan(&existing).Error)
	if len(existing) > 0 {
		rid = existing[0]
	} else {
		rid = dbtest.InsertID(h.t, h.db, `INSERT INTO sys_roles (code, name, data_scope) VALUES (?, ?, 'all')`, code, code)
	}
	require.NoError(h.t, h.db.Exec(`INSERT INTO sys_user_roles (user_id, role_id) VALUES (?, ?)`, uid, rid).Error)
}

func TestLoginIPAllowlist(t *testing.T) {
	h := newHarness(t)
	rec := &recorder{}
	h.core.observer = rec
	h.createUser("alice")
	h.grantRole(h.createUser("boss"), "ops_admin")
	h.setPolicy(`ip_allowlist = '["10.0.0.0/8"]', login_exempt_roles = '["ops_admin"]'`)
	c := h.newClient()

	r := c.login("alice")
	assert.Equal(t, http.StatusForbidden, r.Status)
	assert.Equal(t, CodeLoginIPDenied, r.Code)
	assert.Equal(t, ReasonIPDenied, rec.last().Reason)
	assert.Equal(t, "alice", rec.last().Username)
	assert.Equal(t, CodeLoginIPDenied, wrongLogin(c, "alice", nil).Code, "refused before the password: nothing is learnt about it")
	assert.Equal(t, CodeLoginIPDenied, c.login("ghost").Code, "unknown accounts look the same")

	assert.Equal(t, http.StatusOK, h.newClient().login("boss").Status, "an exempt role skips the allowlist")
	assert.Empty(t, rec.last().Reason)

	h.setPolicy(`ip_allowlist = '["10.0.0.0/8", "127.0.0.1"]'`)
	assert.Equal(t, http.StatusOK, h.newClient().login("alice").Status, "an allowed IP")

	h.setPolicy(`ip_allowlist = '[]'`)
	assert.Equal(t, http.StatusOK, h.newClient().login("alice").Status, "an empty allowlist does not restrict")
}

func TestLoginTimeWindow(t *testing.T) {
	h := newHarness(t)
	rec := &recorder{}
	h.core.observer = rec
	h.createUser("tina")
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	h.setPolicy(`login_time_start = '08:00', login_time_end = '23:00', login_time_zone = 'Asia/Shanghai'`)

	h.clock.now = time.Date(2026, 10, 31, 12, 0, 0, 0, shanghai)
	assert.Equal(t, http.StatusOK, h.newClient().login("tina").Status, "inside the window")

	h.clock.now = time.Date(2026, 10, 31, 23, 30, 0, 0, shanghai)
	r := h.newClient().login("tina")
	assert.Equal(t, http.StatusForbidden, r.Status)
	assert.Equal(t, CodeLoginTimeDenied, r.Code)
	assert.JSONEq(t, `{"start":"08:00","end":"23:00","timeZone":"Asia/Shanghai"}`, string(r.Data))
	assert.Equal(t, ReasonTimeWindow, rec.last().Reason)
}

func TestLoginGuards(t *testing.T) {
	h := newHarness(t)
	rec := &recorder{}
	h.core.observer = rec
	blocked, plain := h.createUser("blocked"), h.createUser("plain")
	h.createUser("fine")
	var seen []LoginAttempt
	h.core.AddLoginGuard(LoginGuardFunc(func(_ context.Context, a LoginAttempt) error {
		seen = append(seen, a)
		switch a.UserID {
		case blocked:
			return httpx.NewError(http.StatusForbidden, 90001, "合同已到期，不能登录")
		case plain:
			return errors.New("crm says no")
		}
		return nil
	}))

	c := h.newClient()
	r := c.login("blocked")
	assert.Equal(t, http.StatusForbidden, r.Status)
	assert.Equal(t, 90001, r.Code, "the guard's error reaches the client")
	assert.Equal(t, ReasonDenied, rec.last().Reason)
	assert.Equal(t, blocked, rec.last().UserID)
	assert.Equal(t, http.StatusUnauthorized, c.ping(), "no session was created")

	r = h.newClient().login("plain")
	assert.Equal(t, CodeLoginDenied, r.Code, "other errors become a generic refusal")

	assert.Equal(t, http.StatusOK, h.newClient().login("fine").Status)
	require.Len(t, seen, 3)
	assert.Equal(t, "127.0.0.1", seen[2].IP)

	assert.Equal(t, CodeBadCredentials, wrongLogin(h.newClient(), "blocked", nil).Code, "guards run only after the password check")
	assert.Len(t, seen, 3)
}

func TestSavePolicyRestrictions(t *testing.T) {
	h := newHarness(t)
	h.clock.now = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	admin := h.createUser("root")
	h.grantRole(admin, SuperAdminRole)
	h.grantRole(h.createUser("someone"), "auditor")
	c := h.newClient()
	c.mustLogin("root")

	var view struct {
		Policy
		ClientIP string `json:"clientIp"`
	}
	r := c.do(http.MethodGet, "/api/v1/security/policy", nil)
	require.Equal(t, http.StatusOK, r.Status)
	require.NoError(t, json.Unmarshal(r.Data, &view))
	assert.Equal(t, "127.0.0.1", view.ClientIP)
	assert.Equal(t, []string{}, view.IPAllowlist)
	assert.Equal(t, "Asia/Shanghai", view.LoginTimeZone)

	save := func(mutate func(p *Policy)) response {
		p := view.Policy
		mutate(&p)
		return c.do(http.MethodPut, "/api/v1/security/policy", p)
	}
	fieldOf := func(r response) string {
		var d struct {
			Fields []httpx.FieldError `json:"fields"`
		}
		_ = json.Unmarshal(r.Data, &d)
		if len(d.Fields) == 0 {
			return ""
		}
		return d.Fields[0].Field
	}

	r = save(func(p *Policy) { p.IPAllowlist = []string{"10.0.0.0/33"} })
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status)
	assert.Equal(t, "ipAllowlist", fieldOf(r))
	r = save(func(p *Policy) { p.LoginExemptRoles = []string{"nobody"} })
	assert.Equal(t, "loginExemptRoles", fieldOf(r), "unknown role")
	r = save(func(p *Policy) { p.IPAllowlist = []string{"10.0.0.0/8"} })
	assert.Equal(t, "ipAllowlist", fieldOf(r), "would lock the saving administrator out")
	r = save(func(p *Policy) { p.LoginTimeStart, p.LoginTimeEnd = "00:00", "00:01" })
	assert.Equal(t, "loginTimeEnd", fieldOf(r), "outside the window now (20:00 in Shanghai)")

	r = save(func(p *Policy) {
		p.IPAllowlist = []string{"10.0.0.0/8", " 10.0.0.0/8 "}
		p.LoginExemptRoles = []string{SuperAdminRole, "auditor"}
		p.LoginTimeStart, p.LoginTimeEnd = "00:00", "00:01"
	})
	require.Equal(t, http.StatusOK, r.Status, string(r.Data))
	require.NoError(t, json.Unmarshal(r.Data, &view))
	assert.Equal(t, []string{"10.0.0.0/8"}, view.IPAllowlist)
	assert.Equal(t, []string{SuperAdminRole, "auditor"}, view.LoginExemptRoles)
	assert.Equal(t, "00:01", view.LoginTimeEnd)

	var roles []policyRole
	r = c.do(http.MethodGet, "/api/v1/security/roles", nil)
	require.NoError(t, json.Unmarshal(r.Data, &roles))
	assert.Contains(t, roles, policyRole{Code: "auditor", Name: "auditor"})

	other := h.newClient().login("someone")
	assert.Equal(t, http.StatusOK, other.Status, "auditor is exempt")
	assert.Equal(t, CodeLoginIPDenied, h.newClient().login("ghost").Code)
}
