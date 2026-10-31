//go:build integration

package notice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"base-admin/server/internal/core/message"
	"base-admin/server/internal/platform/httpx"
)

func (x *inboxHarness) messages(uid int64, query string) httpx.Page[messageView] {
	x.t.Helper()
	status, data := x.call(uid, http.MethodGet, "/api/v1/me/messages"+query, nil)
	require.Equal(x.t, http.StatusOK, status, string(data))
	var page httpx.Page[messageView]
	require.NoError(x.t, json.Unmarshal(data, &page))
	return page
}

func (x *inboxHarness) counts(uid int64) unreadView {
	x.t.Helper()
	status, data := x.call(uid, http.MethodGet, "/api/v1/me/notices/unread-count", nil)
	require.Equal(x.t, http.StatusOK, status)
	var v unreadView
	require.NoError(x.t, json.Unmarshal(data, &v))
	return v
}

func TestSendAndReadMessages(t *testing.T) {
	x := newInboxHarness(t)
	ctx := context.Background()
	reminder := message.Message{UserID: x.reader, Category: "crm.reminder", Title: "客户「华强」今天需要跟进",
		Content: "上次跟进：10 月 24 日", Link: "/crm/customer?id=42"}

	require.NoError(t, x.m.Send(ctx, nil, reminder))
	gone := x.user("gone_user", "已删除")
	require.NoError(t, x.db.Exec(`UPDATE sys_users SET deleted_at = now() WHERE id = ?`, gone).Error)
	require.NoError(t, x.m.Send(ctx, nil,
		message.Message{UserID: x.reader, Category: "crm.assign", Title: "资源已分配给你"},
		message.Message{UserID: gone, Category: "crm.assign", Title: "发给已删除用户"},
		message.Message{UserID: 999999, Category: "crm.assign", Title: "发给不存在的用户"},
	), "missing and deleted recipients are skipped")

	// In the caller's transaction: rolled back with it.
	rollback := errors.New("rollback")
	err := x.db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, x.m.Send(ctx, tx, message.Message{UserID: x.reader, Category: "crm.order", Title: "回滚的消息"}))
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	// An invalid message fails the whole call before anything is written.
	err = x.m.Send(ctx, nil,
		message.Message{UserID: x.reader, Category: "crm.order", Title: "合法"},
		message.Message{UserID: x.reader, Category: "crm.order", Title: "外链", Link: "https://evil.example"})
	assert.Error(t, err)

	var stored int64
	require.NoError(t, x.db.Raw(`SELECT count(*) FROM sys_user_messages`).Scan(&stored).Error)
	assert.Equal(t, int64(2), stored)

	page := x.messages(x.reader, "")
	require.Equal(t, int64(2), page.Total)
	assert.Equal(t, "资源已分配给你", page.List[0].Title, "newest first")
	assert.Equal(t, int64(0), x.messages(x.admin, "").Total, "others do not see them")

	x.create("公告")
	_, _ = x.publish(x.create("已发布公告"), "publish")
	assert.Equal(t, unreadView{Count: 3, Notices: 1, Messages: 2}, x.counts(x.reader), "the bell counts both")
	assert.Equal(t, unreadView{Count: 1, Notices: 1}, x.counts(x.admin))

	first := page.List[1]
	status, _ := x.call(x.admin, http.MethodGet, "/api/v1/me/messages/"+first.ID.String(), nil)
	assert.Equal(t, http.StatusNotFound, status, "another user's message")
	status, data := x.call(x.reader, http.MethodGet, "/api/v1/me/messages/"+first.ID.String(), nil)
	require.Equal(t, http.StatusOK, status)
	var read messageView
	require.NoError(t, json.Unmarshal(data, &read))
	assert.True(t, read.Read)
	assert.Equal(t, "/crm/customer?id=42", read.Link)
	assert.Equal(t, "上次跟进：10 月 24 日", read.Content)
	assert.Equal(t, int64(1), x.messages(x.reader, "?unread=true").Total)
	assert.Equal(t, int64(1), x.messages(x.reader, "?title=华强").Total)

	status, data = x.call(x.reader, http.MethodPost, "/api/v1/me/messages/read-all", nil)
	require.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{"marked":1}`, string(data))
	assert.Equal(t, unreadView{Count: 1, Notices: 1}, x.counts(x.reader), "notices are not marked by it")
}

func TestMessageCleanup(t *testing.T) {
	x := newInboxHarness(t)
	ctx := context.Background()
	gone := x.user("leaver", "离职")
	for _, uid := range []int64{x.reader, x.reader, x.reader, gone} {
		require.NoError(t, x.m.Send(ctx, nil, message.Message{UserID: uid, Category: "crm.reminder", Title: "提醒"}))
	}
	var ids []int64
	require.NoError(t, x.db.Raw(`SELECT id FROM sys_user_messages WHERE user_id = ? ORDER BY id`, x.reader).Scan(&ids).Error)
	require.Len(t, ids, 3)
	require.NoError(t, x.db.Exec(`UPDATE sys_user_messages SET read_at = now() - interval '100 days' WHERE id = ?`, ids[0]).Error)
	require.NoError(t, x.db.Exec(`UPDATE sys_user_messages SET read_at = now() - interval '10 days' WHERE id = ?`, ids[1]).Error)
	require.NoError(t, x.db.Exec(`UPDATE sys_users SET deleted_at = now() WHERE id = ?`, gone).Error)

	require.NoError(t, x.m.cleanupMessages(ctx, nil))
	var left []int64
	require.NoError(t, x.db.Raw(`SELECT id FROM sys_user_messages ORDER BY id`).Scan(&left).Error)
	assert.Equal(t, ids[1:], left, "old read messages and a deleted user's messages go; recent and unread stay")

	require.NoError(t, x.m.cleanupMessages(ctx, json.RawMessage(`{"days": 7}`)))
	require.NoError(t, x.db.Raw(`SELECT id FROM sys_user_messages ORDER BY id`).Scan(&left).Error)
	assert.Equal(t, ids[2:], left)
	for _, bad := range []string{`{"days": 1}`, `{"days": "x"}`} {
		assert.Error(t, x.m.cleanupMessages(ctx, json.RawMessage(bad)), bad)
	}

	// A hard-deleted user takes the messages along (ON DELETE CASCADE).
	require.NoError(t, x.db.Exec(`DELETE FROM sys_users WHERE id = ?`, x.reader).Error)
	var n int64
	require.NoError(t, x.db.Raw(`SELECT count(*) FROM sys_user_messages`).Scan(&n).Error)
	assert.Zero(t, n)
	assert.Equal(t, cleanupJobKey, x.m.Jobs()[0].Key)
}
