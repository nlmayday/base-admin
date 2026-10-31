//go:build integration

package notice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/datascope"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/testinfra/migrated"
)

// userGuard admits the user named by the X-Test-User header.
type userGuard struct{}

func (userGuard) Require(access.Level) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := strconv.ParseInt(c.GetHeader("X-Test-User"), 10, 64)
		a := &actor.Actor{Snapshot: &actor.Snapshot{UserID: uid, IsSuper: true, DataScope: datascope.Resolved{All: true}}}
		c.Request = c.Request.WithContext(actor.With(c.Request.Context(), a))
	}
}

type inboxHarness struct {
	t      *testing.T
	h      http.Handler
	db     *gorm.DB
	admin  int64
	reader int64
}

func newInboxHarness(t *testing.T) *inboxHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := migrated.Database(t)
	sqlDB, err := database.OpenSQL(context.Background(), cfg, "notice-inbox-test")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := database.NewGorm(sqlDB, cfg, zap.NewNop())
	require.NoError(t, err)
	engine := gin.New()
	m := New(Deps{DB: db})
	r := module.NewRouter(engine.Group("/api/v1"), Name, userGuard{}, module.NewRegistry())
	m.noticeRoutes(r)
	m.inboxRoutes(r)
	x := &inboxHarness{t: t, h: engine, db: db}
	x.admin = x.user("notice_admin", "发布人")
	x.reader = x.user("notice_reader", "读者")
	return x
}

func (x *inboxHarness) user(name, nickname string) int64 {
	var id int64
	require.NoError(x.t, x.db.Raw(`INSERT INTO sys_users (username, password_hash, nickname)
		VALUES (?, 'x', ?) RETURNING id`, name, nickname).Scan(&id).Error)
	return id
}

func (x *inboxHarness) call(uid int64, method, path string, body any) (int, json.RawMessage) {
	x.t.Helper()
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User", strconv.FormatInt(uid, 10))
	rec := httptest.NewRecorder()
	x.h.ServeHTTP(rec, req)
	var env noticeEnvelope
	require.NoError(x.t, json.Unmarshal(rec.Body.Bytes(), &env), rec.Body.String())
	return rec.Code, env.Data
}

type noticeState struct {
	ID          string  `json:"id"`
	Version     int     `json:"version"`
	PublishedAt *string `json:"publishedAt"`
}

func (x *inboxHarness) create(title string) noticeState {
	status, data := x.call(x.admin, http.MethodPost, "/api/v1/notices", map[string]any{"title": title, "type": 1, "content": title + "的内容"})
	require.Equal(x.t, http.StatusOK, status, string(data))
	var n noticeState
	require.NoError(x.t, json.Unmarshal(data, &n))
	return n
}

func (x *inboxHarness) publish(n noticeState, action string) (int, noticeState) {
	status, data := x.call(x.admin, http.MethodPost, fmt.Sprintf("/api/v1/notices/%s/%s", n.ID, action), map[string]any{"version": n.Version})
	var out noticeState
	if status == http.StatusOK {
		require.NoError(x.t, json.Unmarshal(data, &out))
	}
	return status, out
}

func (x *inboxHarness) unread(uid int64) int64 {
	status, data := x.call(uid, http.MethodGet, "/api/v1/me/notices/unread-count", nil)
	require.Equal(x.t, http.StatusOK, status)
	var v unreadView
	require.NoError(x.t, json.Unmarshal(data, &v))
	return v.Count
}

func TestNoticePublishAndInbox(t *testing.T) {
	x := newInboxHarness(t)
	a := x.create("系统升级")
	b := x.create("放假安排")

	assert.Equal(t, int64(0), x.unread(x.reader), "drafts are not delivered")
	status, _ := x.call(x.reader, http.MethodGet, "/api/v1/me/notices/"+a.ID, nil)
	assert.Equal(t, http.StatusNotFound, status, "a draft cannot be read")

	status, a = x.publish(a, "publish")
	require.Equal(t, http.StatusOK, status)
	require.NotNil(t, a.PublishedAt)
	status, _ = x.publish(a, "publish")
	assert.Equal(t, http.StatusConflict, status, "publishing twice is rejected")
	status, b = x.publish(b, "publish")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, int64(2), x.unread(x.reader))

	status, data := x.call(x.reader, http.MethodGet, "/api/v1/me/notices/"+a.ID, nil)
	require.Equal(t, http.StatusOK, status)
	var detail inboxView
	require.NoError(t, json.Unmarshal(data, &detail))
	assert.Equal(t, "系统升级的内容", detail.Content)
	assert.True(t, detail.Read)
	assert.Equal(t, "发布人", detail.Publisher)
	assert.Equal(t, int64(1), x.unread(x.reader))
	assert.Equal(t, int64(2), x.unread(x.admin), "read marks are per user")

	status, data = x.call(x.reader, http.MethodGet, "/api/v1/me/notices?unread=true", nil)
	require.Equal(t, http.StatusOK, status)
	var page struct {
		Total int64       `json:"total"`
		List  []inboxView `json:"list"`
	}
	require.NoError(t, json.Unmarshal(data, &page))
	require.Equal(t, int64(1), page.Total)
	assert.Equal(t, "放假安排", page.List[0].Title)
	assert.Empty(t, page.List[0].Content, "the list omits the content")

	status, b = x.publish(b, "revoke")
	require.Equal(t, http.StatusOK, status)
	assert.Nil(t, b.PublishedAt)
	assert.Equal(t, int64(0), x.unread(x.reader), "a revoked notice disappears")
	status, _ = x.call(x.admin, http.MethodGet, "/api/v1/notices?state=draft", nil)
	assert.Equal(t, http.StatusOK, status)

	// Republishing counts as new again.
	_, a = x.publish(a, "revoke")
	_, a = x.publish(a, "publish")
	_, _ = x.publish(b, "publish")
	assert.Equal(t, int64(2), x.unread(x.reader))

	status, data = x.call(x.reader, http.MethodPost, "/api/v1/me/notices/read-all", nil)
	require.Equal(t, http.StatusOK, status)
	var marked readAllView
	require.NoError(t, json.Unmarshal(data, &marked))
	assert.Equal(t, int64(2), marked.Marked)
	assert.Equal(t, int64(0), x.unread(x.reader))
}

func TestNoticeStateFilter(t *testing.T) {
	x := newInboxHarness(t)
	n := x.create("草稿")
	x.create("另一篇草稿")
	_, _ = x.publish(n, "publish")
	for state, want := range map[string]int64{"draft": 1, "published": 1, "": 2} {
		status, data := x.call(x.admin, http.MethodGet, "/api/v1/notices?state="+state, nil)
		require.Equal(t, http.StatusOK, status)
		var page struct {
			Total int64 `json:"total"`
		}
		require.NoError(t, json.Unmarshal(data, &page))
		assert.Equal(t, want, page.Total, "state=%q", state)
	}
	status, _ := x.call(x.admin, http.MethodGet, "/api/v1/notices?state=bogus", nil)
	assert.Equal(t, http.StatusBadRequest, status)
}

func TestNoticeAttachments(t *testing.T) {
	x := newInboxHarness(t)
	const url = "/api/v1/files/a/2026/10/30/0123456789abcdef0123456789abcdef.pdf"
	body := map[string]any{"title": "制度", "type": 1, "content": "见附件",
		"attachments": []map[string]any{{"name": "员工手册.pdf", "url": url, "size": 2048}}}
	status, data := x.call(x.admin, http.MethodPost, "/api/v1/notices", body)
	require.Equal(t, http.StatusOK, status, string(data))
	var n struct {
		noticeState
		Attachments []map[string]any `json:"attachments"`
	}
	require.NoError(t, json.Unmarshal(data, &n))
	require.Len(t, n.Attachments, 1)
	assert.Equal(t, "员工手册.pdf", n.Attachments[0]["name"])

	_, _ = x.publish(n.noticeState, "publish")
	_, data = x.call(x.reader, http.MethodGet, "/api/v1/me/notices/"+n.ID, nil)
	var detail inboxView
	require.NoError(t, json.Unmarshal(data, &detail))
	require.Len(t, detail.Attachments, 1)
	assert.Equal(t, url, detail.Attachments[0].URL)

	// Without attachments the list is empty, not null.
	status, data = x.call(x.admin, http.MethodPost, "/api/v1/notices", map[string]any{"title": "无附件", "type": 1, "content": "x"})
	require.Equal(t, http.StatusOK, status)
	assert.Contains(t, string(data), `"attachments":[]`)

	body["attachments"] = []map[string]any{{"name": "x", "url": "javascript:alert(1)", "size": 1}}
	status, data = x.call(x.admin, http.MethodPost, "/api/v1/notices", body)
	assert.Equal(t, http.StatusUnprocessableEntity, status, "only attachment URLs are stored")
	assert.Contains(t, string(data), "附件地址无效")
}
