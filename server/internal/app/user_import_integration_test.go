//go:build integration

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/simplifiedchinese"

	"base-admin/server/internal/modules/auth"
)

const importPassword = "Import-Pass-2026"

// upload posts a multipart form with one file field.
func (c *apiClient) upload(path string, fields map[string]string, filename string, content []byte) apiResponse {
	c.h.t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		require.NoError(c.h.t, w.WriteField(k, v))
	}
	fw, err := w.CreateFormFile("file", filename)
	require.NoError(c.h.t, err)
	_, _ = fw.Write(content)
	require.NoError(c.h.t, w.Close())
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.h.server.URL+path, &body)
	require.NoError(c.h.t, err)
	req.Header.Set("Content-Type", w.FormDataContentType())
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
	raw, _ := io.ReadAll(resp.Body)
	var env struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(raw, &env)
	return apiResponse{Status: resp.StatusCode, Code: env.Code, Data: env.Data, Body: string(raw), Header: resp.Header}
}

type importRows struct {
	Rows []struct {
		Row     int    `json:"row"`
		Message string `json:"message"`
	} `json:"rows"`
}

func TestUserImport(t *testing.T) {
	h := newAppHarness(t)
	admin := h.admin()
	hq := idOf(t, admin.ok(http.MethodPost, "/api/v1/departments", gin.H{"name": "总部"}))
	branch := idOf(t, admin.ok(http.MethodPost, "/api/v1/departments", gin.H{"name": "分公司"}))
	admin.ok(http.MethodPost, "/api/v1/departments", gin.H{"name": "研发部", "parentId": hq})
	admin.ok(http.MethodPost, "/api/v1/departments", gin.H{"name": "研发部", "parentId": branch})
	admin.ok(http.MethodPost, "/api/v1/roles", gin.H{"code": "dev", "name": "开发", "dataScope": "self"})
	admin.ok(http.MethodPost, "/api/v1/positions", gin.H{"code": "eng", "name": "工程师"})

	tpl := admin.do(http.MethodGet, "/api/v1/users/import-template", nil)
	require.Equal(t, http.StatusOK, tpl.Status)
	assert.Contains(t, tpl.Body, "用户名（必填）")

	header := "用户名（必填）,昵称,手机号,邮箱,部门,角色,岗位\n"
	bad := header +
		"ok_user,张三,,,总部,开发,\n" +
		"x,短名,,,,,\n" +
		"ok_user,重复,,,,,\n" +
		"\n" +
		"mail_user,,,not-an-email,研发部,不存在的角色,\n" +
		"admin,,,,,,\n"
	resp := admin.upload("/api/v1/users/import", map[string]string{"password": importPassword}, "u.csv", []byte(bad))
	require.Equal(t, http.StatusUnprocessableEntity, resp.Status, resp.Body)
	var details importRows
	resp.decode(t, &details)
	byRow := map[int][]string{}
	for _, r := range details.Rows {
		byRow[r.Row] = append(byRow[r.Row], r.Message)
	}
	assert.NotContains(t, byRow, 2, "a valid row has no error")
	assert.Contains(t, byRow[3][0], "3–32")
	assert.Contains(t, byRow[4][0], "第 2 行重复")
	assert.Len(t, byRow[6], 3, "email, ambiguous department and unknown role")
	assert.Contains(t, strings.Join(byRow[6], ";"), "总公司/研发部")
	assert.Contains(t, byRow[7][0], "已存在")
	var n int64
	require.NoError(t, h.db.Table("sys_users").Where("username = ?", "ok_user").Count(&n).Error)
	assert.Zero(t, n, "nothing is imported when a row is wrong")

	resp = admin.upload("/api/v1/users/import", map[string]string{"password": "short"}, "u.csv", []byte(header+"a_user,,,,,,\n"))
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Status, "the initial password follows the policy")

	// GBK, as Excel saves CSV on Chinese Windows, without the hint texts.
	good := "用户名,昵称,手机号,邮箱,部门,角色,岗位\r\n" +
		"imp_one,一号,13800000000,one@example.com,总部/研发部,dev|开发,工程师\r\n" +
		"imp_two,,,,总部,,\r\n"
	gbk, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(good))
	require.NoError(t, err)
	resp = admin.upload("/api/v1/users/import", map[string]string{"password": importPassword}, "u.csv", gbk)
	require.Equal(t, http.StatusOK, resp.Status, resp.Body)
	var result struct {
		Created int `json:"created"`
	}
	resp.decode(t, &result)
	assert.Equal(t, 2, result.Created)

	var user struct {
		Nickname           string
		DeptName           string
		MustChangePassword bool
		Roles              int64
		Positions          int64
	}
	require.NoError(t, h.db.Raw(`SELECT u.nickname, d.name AS dept_name, u.must_change_password,
			(SELECT count(*) FROM sys_user_roles r WHERE r.user_id = u.id) AS roles,
			(SELECT count(*) FROM sys_user_positions p WHERE p.user_id = u.id) AS positions
		FROM sys_users u JOIN sys_departments d ON d.id = u.dept_id WHERE u.username = 'imp_one'`).Scan(&user).Error)
	assert.Equal(t, gin.H{"nick": "一号", "dept": "研发部", "must": true, "roles": int64(1), "positions": int64(1)},
		gin.H{"nick": user.Nickname, "dept": user.DeptName, "must": user.MustChangePassword, "roles": user.Roles, "positions": user.Positions})
	var nick string
	require.NoError(t, h.db.Raw(`SELECT nickname FROM sys_users WHERE username = 'imp_two'`).Scan(&nick).Error)
	assert.Equal(t, "imp_two", nick, "an empty nickname defaults to the username")

	login := h.client().login("imp_one", importPassword)
	assert.Equal(t, http.StatusOK, login.Status, "the initial password works")

	resp = admin.upload("/api/v1/users/import", map[string]string{"password": importPassword}, "u.csv", []byte(good))
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Status, "importing the same users again is rejected")
}
