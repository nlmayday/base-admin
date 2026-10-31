//go:build integration

package app

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/module"
)

var pathParam = regexp.MustCompile(`:[a-zA-Z]+`)

func TestBootstrapAdminMustChangePassword(t *testing.T) {
	h := newAppHarness(t)
	c := h.client()
	resp := c.login("admin", bootstrapPassword)
	require.Equal(t, http.StatusOK, resp.Status, resp.Body)
	var me struct {
		IsSuperAdmin       bool     `json:"isSuperAdmin"`
		MustChangePassword bool     `json:"mustChangePassword"`
		Permissions        []string `json:"permissions"`
	}
	resp.decode(t, &me)
	assert.True(t, me.IsSuperAdmin)
	assert.True(t, me.MustChangePassword)
	assert.Contains(t, me.Permissions, "iam:user:view", "super admin gets every code expanded")
	assert.Equal(t, http.StatusForbidden, c.do(http.MethodGet, "/api/v1/users", nil).Status)

	c.ok(http.MethodPut, "/api/v1/me/password", gin.H{"oldPassword": bootstrapPassword, "newPassword": adminPassword})
	c.ok(http.MethodGet, "/api/v1/users", nil)
}

// TestEveryRouteIsProtected is the automatic authorization sweep from design
// doc section 13: it walks the live route registry, so new routes are
// covered without writing new tests.
func TestEveryRouteIsProtected(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()
	userID := idOf(t, admin.ok(http.MethodPost, "/api/v1/users", gin.H{
		"username": "nobody", "password": "nobody-pass-1", "deptId": nil}))
	require.NotEmpty(t, userID)
	h.activate("nobody")
	anonymous := h.client()
	noPerms := h.client()
	require.Equal(t, http.StatusOK, noPerms.login("nobody", "nobody-pass-1").Status)

	routes := h.app.registry.Routes()
	require.NotEmpty(t, routes)
	for _, r := range routes {
		path := pathParam.ReplaceAllString(r.Path, "1")
		switch r.Access.Kind() {
		case access.KindPublic:
			if strings.HasPrefix(r.Path, "/api/v1/devtools/agent/") {
				// The AI agent API authenticates by key (devtools agentAuth);
				// outside dev it refuses before looking at the key.
				assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden},
					anonymous.do(r.Method, path, gin.H{}).Status, "%s %s", r.Method, r.Path)
			}
			continue
		case access.KindAuthenticated:
			assert.Equal(t, http.StatusUnauthorized, anonymous.do(r.Method, path, gin.H{}).Status, "%s %s", r.Method, r.Path)
		case access.KindPermission:
			assert.Equal(t, http.StatusUnauthorized, anonymous.do(r.Method, path, gin.H{}).Status, "%s %s", r.Method, r.Path)
			resp := noPerms.do(r.Method, path, gin.H{})
			assert.Equal(t, http.StatusForbidden, resp.Status, "%s %s: %s", r.Method, r.Path, resp.Body)
		default:
			t.Errorf("route %s %s has invalid access", r.Method, r.Path)
		}
	}
	assertNoPublicBusinessRoutes(t, routes)
}

func assertNoPublicBusinessRoutes(t *testing.T, routes []module.Route) {
	var public []string
	for _, r := range routes {
		if r.Access.Kind() == access.KindPublic {
			public = append(public, r.Method+" "+r.Path)
		}
	}
	// Every public route is listed here on purpose: adding one must be a
	// reviewed change.
	assert.ElementsMatch(t, []string{
		"POST /api/v1/auth/login",
		"GET /api/v1/auth/captcha",
		"GET /api/v1/auth/login-options", "GET /api/v1/settings/public", "GET /api/v1/files/signed/:token",
		// AI agent API: key authentication instead of the session.
		"GET /api/v1/devtools/agent/guide", "GET /api/v1/devtools/agent/tables", "GET /api/v1/devtools/agent/fields",
		"GET /api/v1/devtools/agent/dicts", "POST /api/v1/devtools/agent/tables", "POST /api/v1/devtools/agent/preview",
		"POST /api/v1/devtools/agent/generate",
	}, public, "public routes must be reviewed explicitly")
}

type idResp struct {
	ID string `json:"id"`
}

// setupOrg creates two departments, a role with a dept scope and a user in
// each department. It returns ids by name.
func setupOrg(t *testing.T, admin *apiClient, codes []string) map[string]string {
	ids := map[string]string{}
	ids["hq"] = idOf(t, admin.ok(http.MethodPost, "/api/v1/departments", gin.H{"name": "总部", "parentId": nil}))
	ids["sales"] = idOf(t, admin.ok(http.MethodPost, "/api/v1/departments", gin.H{"name": "销售部", "parentId": ids["hq"]}))
	ids["ops"] = idOf(t, admin.ok(http.MethodPost, "/api/v1/departments", gin.H{"name": "运营部", "parentId": ids["hq"]}))

	ids["role"] = idOf(t, admin.ok(http.MethodPost, "/api/v1/roles", gin.H{"code": "sales_mgr", "name": "销售经理", "dataScope": "dept"}))
	admin.ok(http.MethodPut, "/api/v1/roles/"+ids["role"]+"/permissions", gin.H{"codes": codes})

	ids["manager"] = idOf(t, admin.ok(http.MethodPost, "/api/v1/users", gin.H{"username": "manager", "password": "manager-pass-1",
		"deptId": ids["sales"], "roleIds": []string{ids["role"]}}))
	ids["salesRep"] = idOf(t, admin.ok(http.MethodPost, "/api/v1/users", gin.H{"username": "rep", "password": "rep-pass-123",
		"deptId": ids["sales"], "nickname": "=HYPERLINK(\"x\")"}))
	ids["opsUser"] = idOf(t, admin.ok(http.MethodPost, "/api/v1/users", gin.H{"username": "opsuser", "password": "ops-pass-123",
		"deptId": ids["ops"]}))
	require.NoError(t, admin.h.db.Exec("UPDATE sys_users SET must_change_password = false").Error)
	return ids
}

func TestGrantValidation(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()
	role := idOf(t, admin.ok(http.MethodPost, "/api/v1/roles", gin.H{"code": "ops", "name": "运维"}))

	resp := admin.do(http.MethodPut, "/api/v1/roles/"+role+"/permissions", gin.H{"codes": []string{"iam:user:delete", "iam:ghost:view"}})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Status)
	assert.Equal(t, 30001, resp.Code)
	assert.JSONEq(t, `{"missing":["iam:user:view"],"unknown":["iam:ghost:view"]}`, string(resp.Data))

	admin.ok(http.MethodPut, "/api/v1/roles/"+role+"/permissions", gin.H{"codes": []string{"iam:user:delete", "iam:user:view"}})
	var got struct {
		Codes []string `json:"codes"`
	}
	admin.ok(http.MethodGet, "/api/v1/roles/"+role+"/permissions", nil).decode(t, &got)
	assert.Equal(t, []string{"iam:user:delete", "iam:user:view"}, got.Codes)

	var supers struct {
		List []struct {
			ID      string `json:"id"`
			Code    string `json:"code"`
			Builtin bool   `json:"builtin"`
		} `json:"list"`
	}
	admin.ok(http.MethodGet, "/api/v1/roles?keyword=super_admin", nil).decode(t, &supers)
	require.Len(t, supers.List, 1)
	resp = admin.do(http.MethodPut, "/api/v1/roles/"+supers.List[0].ID+"/permissions", gin.H{"codes": []string{}})
	assert.Equal(t, 30002, resp.Code, "built-in role is protected")
	assert.Equal(t, 30002, admin.do(http.MethodDelete, "/api/v1/roles/"+supers.List[0].ID, nil).Code)
}

func TestButtonsDifferPerRoleAndPermissionChangesApplyImmediately(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()
	ids := setupOrg(t, admin, []string{"iam:user:view", "iam:user:create"})

	mgr := h.client()
	require.Equal(t, http.StatusOK, mgr.login("manager", "manager-pass-1").Status)
	var me struct {
		Permissions []string `json:"permissions"`
	}
	mgr.ok(http.MethodGet, "/api/v1/me", nil).decode(t, &me)
	assert.Equal(t, []string{"iam:user:create", "iam:user:view"}, me.Permissions)

	var menus []struct {
		Path     string `json:"path"`
		Children []struct {
			Path      string `json:"path"`
			Name      string `json:"name"`
			Component string `json:"component"`
			Meta      struct {
				ShowParent bool `json:"showParent"`
			} `json:"meta"`
		} `json:"children"`
	}
	mgr.ok(http.MethodGet, "/api/v1/me/menus", nil).decode(t, &menus)
	require.Len(t, menus, 1)
	assert.Equal(t, "/system", menus[0].Path)
	require.Len(t, menus[0].Children, 1, "only the user page is visible")
	assert.Equal(t, "IamUser", menus[0].Children[0].Name)
	assert.Equal(t, "system/user/index", menus[0].Children[0].Component)
	assert.True(t, menus[0].Children[0].Meta.ShowParent, "a directory with one visible page still shows in the sidebar")

	assert.Equal(t, http.StatusForbidden, mgr.do(http.MethodDelete, "/api/v1/users/"+ids["salesRep"], nil).Status)
	admin.ok(http.MethodPut, "/api/v1/roles/"+ids["role"]+"/permissions",
		gin.H{"codes": []string{"iam:user:view", "iam:user:create", "iam:user:delete"}})
	mgr.ok(http.MethodDelete, "/api/v1/users/"+ids["salesRep"], nil)
}

func TestDataScope(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()
	ids := setupOrg(t, admin, []string{"iam:user:view", "iam:user:update", "iam:user:delete", "iam:user:export"})
	mgr := h.client()
	require.Equal(t, http.StatusOK, mgr.login("manager", "manager-pass-1").Status)

	var page struct {
		List []struct {
			Username string `json:"username"`
		} `json:"list"`
		Total int `json:"total"`
	}
	mgr.ok(http.MethodGet, "/api/v1/users", nil).decode(t, &page)
	var names []string
	for _, u := range page.List {
		names = append(names, u.Username)
	}
	assert.ElementsMatch(t, []string{"manager", "rep"}, names, "dept scope hides other departments")
	assert.Equal(t, 2, page.Total)

	// Operations on out-of-scope rows look like missing rows (doc 7.3).
	assert.Equal(t, http.StatusNotFound, mgr.do(http.MethodGet, "/api/v1/users/"+ids["opsUser"], nil).Status)
	assert.Equal(t, http.StatusNotFound, mgr.do(http.MethodDelete, "/api/v1/users/"+ids["opsUser"], nil).Status)
	assert.Equal(t, http.StatusNotFound, mgr.do(http.MethodPut, "/api/v1/users/"+ids["opsUser"],
		gin.H{"nickname": "x", "version": 0}).Status)

	var opts []struct {
		Username string `json:"username"`
	}
	mgr.ok(http.MethodGet, "/api/v1/users/options", nil).decode(t, &opts)
	assert.Len(t, opts, 2, "authenticated helper endpoints are scoped too")

	var tree []struct {
		Name string `json:"name"`
	}
	mgr.ok(http.MethodGet, "/api/v1/departments/tree", nil).decode(t, &tree)
	require.Len(t, tree, 1)
	assert.Equal(t, "销售部", tree[0].Name)

	csv := mgr.ok(http.MethodGet, "/api/v1/users/export", nil)
	assert.Contains(t, csv.Header.Get("Content-Type"), "text/csv")
	assert.Contains(t, csv.Body, "rep")
	assert.NotContains(t, csv.Body, "opsuser", "export honours the data scope")
	assert.Contains(t, csv.Body, `'=HYPERLINK`, "formula injection is neutralized")

	// Moving the ops department under sales widens a dept_and_children scope.
	admin.ok(http.MethodPut, "/api/v1/roles/"+ids["role"], gin.H{"name": "销售经理", "dataScope": "dept_and_children", "version": 0})
	var dept struct {
		Version int `json:"version"`
	}
	admin.ok(http.MethodGet, "/api/v1/departments/tree", nil)
	require.NoError(t, h.db.Raw("SELECT version FROM sys_departments WHERE id = ?", ids["ops"]).Scan(&dept.Version).Error)
	admin.ok(http.MethodPut, "/api/v1/departments/"+ids["ops"], gin.H{"name": "运营部", "parentId": ids["sales"], "version": dept.Version})
	mgr.ok(http.MethodGet, "/api/v1/users/"+ids["opsUser"], nil)

	bad := admin.do(http.MethodPut, "/api/v1/departments/"+ids["sales"], gin.H{"name": "销售部", "parentId": ids["ops"], "version": 0})
	assert.Equal(t, http.StatusUnprocessableEntity, bad.Status, "cannot move under a descendant")
}

func TestPrivilegeEscalationIsBlocked(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()
	ids := setupOrg(t, admin, []string{"iam:user:view", "iam:user:update", "iam:role:view", "iam:role:grant", "iam:role:create"})
	mgr := h.client()
	require.Equal(t, http.StatusOK, mgr.login("manager", "manager-pass-1").Status)

	resp := mgr.do(http.MethodPut, "/api/v1/roles/"+ids["role"]+"/permissions",
		gin.H{"codes": []string{"iam:user:view", "iam:user:update", "iam:role:view", "iam:role:grant", "iam:role:create", "iam:user:delete"}})
	assert.Equal(t, http.StatusForbidden, resp.Status, "cannot grant a code it does not hold: %s", resp.Body)

	resp = mgr.do(http.MethodPost, "/api/v1/roles", gin.H{"code": "god", "name": "全部", "dataScope": "all"})
	assert.Equal(t, http.StatusForbidden, resp.Status, "cannot create an all-scope role")

	var supers struct {
		List []idResp `json:"list"`
	}
	mgr.ok(http.MethodGet, "/api/v1/roles?keyword=super_admin", nil).decode(t, &supers)
	resp = mgr.do(http.MethodPut, "/api/v1/users/"+ids["salesRep"], gin.H{"roleIds": []string{supers.List[0].ID}, "deptId": ids["sales"], "version": 0})
	assert.Equal(t, http.StatusForbidden, resp.Status, "cannot assign the super admin role")
}

func TestUserLifecycleAndProtections(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()
	ids := setupOrg(t, admin, []string{"iam:user:view"})
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	admin.ok(http.MethodGet, "/api/v1/me", nil).decode(t, &me)

	assert.Equal(t, 30002, admin.do(http.MethodDelete, "/api/v1/users/"+me.User.ID, nil).Code, "cannot delete yourself")
	assert.Equal(t, 30002, admin.do(http.MethodPut, "/api/v1/users/"+me.User.ID, gin.H{"roleIds": []string{}, "version": 2}).Code,
		"the last super admin keeps the role")

	dup := admin.do(http.MethodPost, "/api/v1/users", gin.H{"username": "MANAGER", "password": "whatever-1"})
	assert.Equal(t, http.StatusUnprocessableEntity, dup.Status, "usernames are unique case-insensitively")

	rep := h.client()
	require.Equal(t, http.StatusOK, rep.login("rep", "rep-pass-123").Status)

	var user struct {
		Version int `json:"version"`
	}
	admin.ok(http.MethodGet, "/api/v1/users/"+ids["salesRep"], nil).decode(t, &user)
	admin.ok(http.MethodPut, "/api/v1/users/"+ids["salesRep"]+"/status", gin.H{"status": 2, "version": user.Version})
	assert.Equal(t, http.StatusUnauthorized, rep.do(http.MethodGet, "/api/v1/me", nil).Status, "disable takes effect immediately")
	assert.Equal(t, http.StatusUnauthorized, h.client().login("rep", "rep-pass-123").Status)

	stale := admin.do(http.MethodPut, "/api/v1/users/"+ids["salesRep"]+"/status", gin.H{"status": 1, "version": user.Version})
	assert.Equal(t, http.StatusConflict, stale.Status, "optimistic lock")
	admin.ok(http.MethodPut, "/api/v1/users/"+ids["salesRep"]+"/status", gin.H{"status": 1, "version": user.Version + 1})

	admin.ok(http.MethodPut, "/api/v1/users/"+ids["salesRep"]+"/password", gin.H{"password": "reset-pass-1"})
	again := h.client()
	resp := again.login("rep", "reset-pass-1")
	require.Equal(t, http.StatusOK, resp.Status)
	assert.Contains(t, resp.Body, `"mustChangePassword":true`)

	admin.ok(http.MethodDelete, "/api/v1/users/"+ids["salesRep"], nil)
	assert.Equal(t, http.StatusUnauthorized, again.do(http.MethodGet, "/api/v1/me", nil).Status)
	assert.Equal(t, http.StatusNotFound, admin.do(http.MethodGet, "/api/v1/users/"+ids["salesRep"], nil).Status)
	admin.ok(http.MethodPost, "/api/v1/users", gin.H{"username": "rep", "password": "new-rep-pass1"})

	assert.Equal(t, 30003, admin.do(http.MethodDelete, "/api/v1/departments/"+ids["sales"], nil).Code, "department in use")
	assert.Equal(t, 30003, admin.do(http.MethodDelete, "/api/v1/roles/"+ids["role"], nil).Code, "role in use")
}

func TestPositionsAndMenus(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()

	pos := idOf(t, admin.ok(http.MethodPost, "/api/v1/positions", gin.H{"code": "engineer", "name": "工程师"}))
	admin.ok(http.MethodPut, "/api/v1/positions/"+pos, gin.H{"name": "高级工程师", "version": 0})
	u := idOf(t, admin.ok(http.MethodPost, "/api/v1/users", gin.H{"username": "pos_user", "password": "pos-pass-123", "positionIds": []string{pos}}))
	assert.Equal(t, 30003, admin.do(http.MethodDelete, "/api/v1/positions/"+pos, nil).Code)
	admin.ok(http.MethodDelete, "/api/v1/users/"+u, nil)
	admin.ok(http.MethodDelete, "/api/v1/positions/"+pos, nil)

	var tree []struct {
		ID       string `json:"id"`
		Code     string `json:"code"`
		Version  int    `json:"version"`
		Children []struct {
			ID      string `json:"id"`
			Code    string `json:"code"`
			Version int    `json:"version"`
		} `json:"children"`
	}
	admin.ok(http.MethodGet, "/api/v1/menus/tree", nil).decode(t, &tree)
	require.NotEmpty(t, tree)
	userMenu := tree[0].Children[0]
	var withPerms []struct {
		Children []struct {
			Permissions []struct {
				Code string `json:"code"`
			} `json:"permissions"`
		} `json:"children"`
	}
	admin.ok(http.MethodGet, "/api/v1/menus/tree", nil).decode(t, &withPerms)
	require.NotEmpty(t, withPerms[0].Children[0].Permissions)
	assert.Equal(t, "iam:user:view", withPerms[0].Children[0].Permissions[0].Code, "declaration order: view first")
	assert.Equal(t, 30002, admin.do(http.MethodDelete, "/api/v1/menus/"+userMenu.ID, nil).Code, "module menus cannot be deleted")
	admin.ok(http.MethodPut, "/api/v1/menus/"+userMenu.ID, gin.H{"title": "账号管理", "icon": "x", "sort": 9, "hidden": false, "version": userMenu.Version})

	// A restart re-syncs code-owned fields but keeps the admin's title.
	require.NoError(t, module.Sync(context.Background(), h.db, h.app.modules))
	var title string
	require.NoError(t, h.db.Raw("SELECT title FROM sys_menus WHERE id = ?", userMenu.ID).Scan(&title).Error)
	assert.Equal(t, "账号管理", title)

	dir := idOf(t, admin.ok(http.MethodPost, "/api/v1/menus", gin.H{"type": "dir", "title": "外部", "path": "/ext"}))
	admin.ok(http.MethodPost, "/api/v1/menus", gin.H{"type": "link", "title": "文档", "parentId": dir, "externalUrl": "https://example.com"})
	bad := admin.do(http.MethodPost, "/api/v1/menus", gin.H{"type": "link", "title": "坏链接", "externalUrl": "javascript:alert(1)"})
	assert.Equal(t, http.StatusUnprocessableEntity, bad.Status)
	assert.Equal(t, 30003, admin.do(http.MethodDelete, "/api/v1/menus/"+dir, nil).Code)

	menus := admin.ok(http.MethodGet, "/api/v1/me/menus", nil)
	assert.Contains(t, menus.Body, `"path":"https://example.com"`)

	var permTree []struct {
		Title string `json:"title"`
	}
	admin.ok(http.MethodGet, "/api/v1/permissions/tree", nil).decode(t, &permTree)
	require.NotEmpty(t, permTree)
	assert.Equal(t, "系统管理", permTree[0].Title)
}
