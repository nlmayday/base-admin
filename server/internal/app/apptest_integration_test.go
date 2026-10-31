//go:build integration

package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/modules/auth"
	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/observability"
	"base-admin/server/internal/platform/password"
	"base-admin/server/internal/platform/testinfra/migrated"
)

const (
	bootstrapPassword = "bootstrap-pass-123"
	adminPassword     = "admin-pass-12345"
)

var fastHash = password.Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

// appHarness runs the fully assembled application against fresh storage.
type appHarness struct {
	t      *testing.T
	app    *application
	db     *gorm.DB
	server *httptest.Server
}

func newAppHarness(t *testing.T) *appHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := integrationConfig(t)
	cfg.Database = migrated.Database(t)
	cfg.Bootstrap = config.BootstrapConfig{AdminUsername: "admin", AdminPassword: bootstrapPassword}

	ctx := context.Background()
	d, err := openDeps(ctx, cfg, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.close() })

	health := observability.NewHealth(zap.NewNop(), observability.HealthOptions{Timeout: 1e9})
	application, err := buildApplication(ctx, cfg, zap.NewNop(), d, observability.NewMetrics(), health, fastHash)
	require.NoError(t, err)
	t.Cleanup(func() { _ = application.close() })
	srv := httptest.NewServer(application.engine)
	t.Cleanup(srv.Close)
	return &appHarness{t: t, app: application, db: d.db, server: srv}
}

type apiClient struct {
	h    *appHarness
	http *http.Client
}

func (h *appHarness) client() *apiClient {
	jar, err := cookiejar.New(nil)
	require.NoError(h.t, err)
	return &apiClient{h: h, http: &http.Client{Jar: jar}}
}

type apiResponse struct {
	Status int
	Code   int
	Data   json.RawMessage
	Body   string
	Header http.Header
}

func (r apiResponse) decode(t *testing.T, v any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(r.Data, v), r.Body)
}

func (c *apiClient) do(method, path string, body any) apiResponse {
	c.h.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(c.h.t, err)
		reader = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(context.Background(), method, c.h.server.URL+path, reader)
	require.NoError(c.h.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)
	u, _ := url.Parse(c.h.server.URL)
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == auth.CSRFCookie {
			req.Header.Set(auth.CSRFHeader, ck.Value)
		}
	}
	resp, err := c.http.Do(req)
	require.NoError(c.h.t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(c.h.t, err)
	var env struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(raw, &env)
	return apiResponse{Status: resp.StatusCode, Code: env.Code, Data: env.Data, Body: string(raw), Header: resp.Header}
}

// ok performs a request and requires HTTP 200.
func (c *apiClient) ok(method, path string, body any) apiResponse {
	c.h.t.Helper()
	resp := c.do(method, path, body)
	require.Equal(c.h.t, http.StatusOK, resp.Status, "%s %s: %s", method, path, resp.Body)
	return resp
}

func (c *apiClient) login(username, pw string) apiResponse {
	return c.do(http.MethodPost, "/api/v1/auth/login", gin.H{"username": username, "password": pw})
}

// admin logs in as the bootstrap admin and completes the forced password
// change.
func (h *appHarness) admin() *apiClient {
	h.t.Helper()
	c := h.client()
	if resp := c.login("admin", adminPassword); resp.Status == http.StatusOK {
		return c
	}
	resp := c.login("admin", bootstrapPassword)
	require.Equal(h.t, http.StatusOK, resp.Status, resp.Body)
	c.ok(http.MethodPut, "/api/v1/me/password", gin.H{"oldPassword": bootstrapPassword, "newPassword": adminPassword})
	return c
}

// idOf extracts "id" from a response body.
func idOf(t *testing.T, r apiResponse) string {
	t.Helper()
	var v struct {
		ID string `json:"id"`
	}
	r.decode(t, &v)
	require.NotEmpty(t, v.ID)
	return v.ID
}

// activate clears the forced password change of users created through the
// API, for tests that exercise other flows.
func (h *appHarness) activate(usernames ...string) {
	h.t.Helper()
	require.NoError(h.t, h.db.Exec("UPDATE sys_users SET must_change_password = false WHERE username IN ?", usernames).Error)
}

// actor creates a role with codes and scope, a user holding it in deptID,
// and returns a logged-in client plus the role id.
func (h *appHarness) actor(admin *apiClient, username, deptID, scope string, customDepts, codes []string) (*apiClient, string) {
	h.t.Helper()
	role := idOf(h.t, admin.ok(http.MethodPost, "/api/v1/roles", gin.H{"code": "r_" + username, "name": username,
		"dataScope": scope, "deptIds": customDepts}))
	admin.ok(http.MethodPut, "/api/v1/roles/"+role+"/permissions", gin.H{"codes": codes})
	admin.ok(http.MethodPost, "/api/v1/users", gin.H{"username": username, "password": username + "-pass-1",
		"deptId": deptID, "roleIds": []string{role}})
	h.activate(username)
	c := h.client()
	require.Equal(h.t, http.StatusOK, c.login(username, username+"-pass-1").Status)
	return c, role
}
