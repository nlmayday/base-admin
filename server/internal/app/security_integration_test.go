//go:build integration

package app

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression tests for the security review findings (design doc 17).

type org struct{ hq, sales, ops string }

func newOrg(t *testing.T, admin *apiClient) org {
	hq := idOf(t, admin.ok(http.MethodPost, "/api/v1/departments", gin.H{"name": "总部"}))
	return org{
		hq:    hq,
		sales: idOf(t, admin.ok(http.MethodPost, "/api/v1/departments", gin.H{"name": "销售部", "parentId": hq})),
		ops:   idOf(t, admin.ok(http.MethodPost, "/api/v1/departments", gin.H{"name": "运营部", "parentId": hq})),
	}
}

func TestNewUsersMustChangePassword(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()
	admin.ok(http.MethodPost, "/api/v1/users", gin.H{"username": "fresh", "password": "fresh-pass-1"})
	resp := h.client().login("fresh", "fresh-pass-1")
	require.Equal(t, http.StatusOK, resp.Status)
	assert.Contains(t, resp.Body, `"mustChangePassword":true`)
}

func TestH1_CannotWidenOwnRoleScope(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()
	o := newOrg(t, admin)
	mgr, role := h.actor(admin, "mgr", o.sales, "dept", nil, []string{"iam:user:view", "iam:role:view", "iam:role:update", "iam:role:create"})

	resp := mgr.do(http.MethodPost, "/api/v1/roles", gin.H{"code": "sneaky", "name": "x", "dataScope": "custom",
		"deptIds": []string{o.ops}})
	assert.Equal(t, http.StatusForbidden, resp.Status, "a new role cannot cover departments outside the caller's scope: %s", resp.Body)

	resp = mgr.do(http.MethodPut, "/api/v1/roles/"+role, gin.H{"name": "mgr", "dataScope": "custom",
		"deptIds": []string{o.hq, o.sales, o.ops}, "version": 0})
	assert.Equal(t, http.StatusForbidden, resp.Status, resp.Body)

	resp = mgr.do(http.MethodPut, "/api/v1/roles/"+role, gin.H{"name": "mgr", "dataScope": "custom",
		"deptIds": []string{o.sales}, "version": 0})
	assert.Equal(t, http.StatusOK, resp.Status, "a scope within its own is fine: %s", resp.Body)
}

func TestH2_CannotAssignWiderRole(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()
	o := newOrg(t, admin)
	wide := idOf(t, admin.ok(http.MethodPost, "/api/v1/roles", gin.H{"code": "wide", "name": "wide", "dataScope": "custom",
		"deptIds": []string{o.hq, o.sales, o.ops}}))
	admin.ok(http.MethodPut, "/api/v1/roles/"+wide+"/permissions", gin.H{"codes": []string{"iam:user:view"}})
	mgr, role := h.actor(admin, "mgr", o.sales, "dept", nil, []string{"iam:user:view", "iam:user:update", "iam:user:create"})

	var me struct {
		User struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
		} `json:"user"`
	}
	mgr.ok(http.MethodGet, "/api/v1/me", nil).decode(t, &me)
	resp := mgr.do(http.MethodPut, "/api/v1/users/"+me.User.ID, gin.H{"deptId": o.sales,
		"roleIds": []string{role, wide}, "version": me.User.Version})
	assert.Equal(t, http.StatusForbidden, resp.Status, "assigning a wider role to itself: %s", resp.Body)

	resp = mgr.do(http.MethodPost, "/api/v1/users", gin.H{"username": "accomplice", "password": "accomplice-1",
		"deptId": o.sales, "roleIds": []string{wide}})
	assert.Equal(t, http.StatusForbidden, resp.Status, "or to a new account: %s", resp.Body)
}

func TestH3_CannotTakeOverMorePrivilegedPeer(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()
	o := newOrg(t, admin)
	support, _ := h.actor(admin, "support", o.sales, "dept", nil, []string{"iam:user:view", "iam:user:reset-password", "iam:user:update"})
	h.actor(admin, "victim", o.sales, "dept", nil, []string{"iam:user:view", "iam:role:view", "iam:role:grant"})
	plain := idOf(t, admin.ok(http.MethodPost, "/api/v1/users", gin.H{"username": "plain", "password": "plain-pass-1", "deptId": o.sales}))

	var victimID string
	require.NoError(t, h.db.Raw("SELECT id::text FROM sys_users WHERE username = 'victim'").Scan(&victimID).Error)
	resp := support.do(http.MethodPut, "/api/v1/users/"+victimID+"/password", gin.H{"password": "taken-over-1"})
	assert.Equal(t, http.StatusForbidden, resp.Status, "reset-password takeover: %s", resp.Body)
	resp = support.do(http.MethodPut, "/api/v1/users/"+victimID, gin.H{"deptId": o.sales, "roleIds": []string{}, "version": 0})
	assert.Equal(t, http.StatusForbidden, resp.Status, "stripping a stronger peer's roles: %s", resp.Body)

	support.ok(http.MethodPut, "/api/v1/users/"+plain+"/password", gin.H{"password": "plain-reset-1"})
}

func TestM1_CannotWidenScopeByMovingSelf(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()
	o := newOrg(t, admin)
	mgr, role := h.actor(admin, "mgr", o.sales, "dept_and_children", nil, []string{"iam:user:view", "iam:user:update"})
	extra := idOf(t, admin.ok(http.MethodPost, "/api/v1/roles", gin.H{"code": "hq_only", "name": "hq", "dataScope": "custom",
		"deptIds": []string{o.hq}}))
	var uid string
	require.NoError(t, h.db.Raw("SELECT id::text FROM sys_users WHERE username = 'mgr'").Scan(&uid).Error)
	admin.ok(http.MethodPut, "/api/v1/users/"+uid, gin.H{"deptId": o.sales, "roleIds": []string{role, extra}, "version": 0})

	resp := mgr.do(http.MethodPut, "/api/v1/users/"+uid, gin.H{"deptId": o.hq, "roleIds": []string{role, extra}, "version": 1})
	assert.Equal(t, http.StatusForbidden, resp.Status, "moving itself to hq would cover ops: %s", resp.Body)
}

func TestM2_StaleVersionHasNoRevocationSideEffect(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()
	target := idOf(t, admin.ok(http.MethodPost, "/api/v1/users", gin.H{"username": "target", "password": "target-pass-1"}))
	h.activate("target")
	victim := h.client()
	require.Equal(t, http.StatusOK, victim.login("target", "target-pass-1").Status)

	resp := admin.do(http.MethodPut, "/api/v1/users/"+target+"/status", gin.H{"status": 2, "version": 9999})
	assert.Equal(t, http.StatusConflict, resp.Status, resp.Body)
	victim.ok(http.MethodGet, "/api/v1/me", nil)
}

func TestM4_DepartmentMovesAreSerialized(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()
	o := newOrg(t, admin)

	// Hold the tree lock from another connection: a move must wait for it,
	// which is what prevents two concurrent moves from forming a cycle.
	holder := h.db.Begin()
	require.NoError(t, holder.Exec("SELECT pg_advisory_xact_lock(hashtext('base-admin:dept-tree'))").Error)
	done := make(chan int, 1)
	go func() {
		done <- admin.do(http.MethodPut, "/api/v1/departments/"+o.sales, gin.H{"name": "x", "parentId": o.ops, "version": 0}).Status
	}()
	select {
	case s := <-done:
		_ = holder.Rollback()
		t.Fatalf("move finished (%d) while the tree lock was held", s)
	case <-time.After(300 * time.Millisecond):
	}
	require.NoError(t, holder.Rollback().Error)
	assert.Equal(t, http.StatusOK, <-done)

	// With moves serialized, the reverse move sees the new path and is refused.
	resp := admin.do(http.MethodPut, "/api/v1/departments/"+o.ops, gin.H{"name": "运营部", "parentId": o.sales, "version": 0})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Status, resp.Body)

	var badPaths int64
	require.NoError(t, h.db.Raw(`SELECT count(*) FROM sys_departments c JOIN sys_departments p ON p.id = c.parent_id
		WHERE c.path <> p.path || c.id || '/'`).Scan(&badPaths).Error)
	assert.Zero(t, badPaths, "materialized paths stay consistent")
}

func TestL1_OptionsCannotProbePhoneNumbers(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()
	o := newOrg(t, admin)
	admin.ok(http.MethodPost, "/api/v1/users", gin.H{"username": "rep", "password": "rep-pass-123", "deptId": o.sales, "phone": "13812345678"})
	viewer, _ := h.actor(admin, "viewer", o.sales, "dept", nil, []string{"iam:position:view"})

	var opts []struct {
		Username string `json:"username"`
	}
	viewer.ok(http.MethodGet, "/api/v1/users/options?keyword=1381234", nil).decode(t, &opts)
	assert.Empty(t, opts)

	var roles []struct {
		Code string `json:"code"`
	}
	viewer.ok(http.MethodGet, "/api/v1/roles/options", nil).decode(t, &roles)
	for _, r := range roles {
		assert.NotEqual(t, "super_admin", r.Code)
	}
}

// Ending sessions follows the same anti-escalation rule as editing users.
func TestSessionRevokerCannotKickStrongerAdmin(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()
	o := newOrg(t, admin)
	revoker, _ := h.actor(admin, "revoker", o.sales, "dept", nil, []string{"auth:session:view", "auth:session:revoke"})
	h.actor(admin, "stronger", o.sales, "dept", nil, []string{"iam:user:view", "iam:role:view", "iam:role:grant"})
	h.actor(admin, "peer", o.sales, "dept", nil, []string{"auth:session:view"})

	var strongerID, peerID string
	require.NoError(t, h.db.Raw("SELECT id::text FROM sys_users WHERE username = 'stronger'").Scan(&strongerID).Error)
	require.NoError(t, h.db.Raw("SELECT id::text FROM sys_users WHERE username = 'peer'").Scan(&peerID).Error)

	resp := revoker.do(http.MethodDelete, "/api/v1/sessions/users/"+strongerID, nil)
	assert.Equal(t, http.StatusForbidden, resp.Status, "kicking a more privileged admin: %s", resp.Body)
	revoker.ok(http.MethodDelete, "/api/v1/sessions/users/"+peerID, nil)

	resp = revoker.do(http.MethodDelete, "/api/v1/sessions/gen:5", nil)
	assert.Equal(t, http.StatusNotFound, resp.Status, "malformed session ids never reach Redis")
}
