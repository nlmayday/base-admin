//go:build integration

package lookup

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/testinfra/dbtest"
	"base-admin/server/internal/platform/testinfra/migrated"
)

func TestTarget(t *testing.T) {
	ctx := context.Background()
	cfg := migrated.Database(t)
	sqlDB, err := database.OpenSQL(ctx, cfg, "lookup-test")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := database.NewGorm(sqlDB, cfg, zap.NewNop())
	require.NoError(t, err)

	var alice, bob, gone int64
	insert := func(name, nickname string, deleted bool, id *int64) {
		*id = dbtest.InsertID(t, db, `INSERT INTO sys_users (username, password_hash, nickname, deleted_at)
			VALUES (?, 'x', ?, CASE WHEN ? THEN CURRENT_TIMESTAMP(6) END)`, name, nickname, deleted)
	}
	insert("alice", "爱丽丝", false, &alice)
	insert("bob", "鲍勃_%", false, &bob)
	insert("gone", "爱丽丝二号", true, &gone)

	users := Target{Table: "sys_users", Label: "nickname", SoftDelete: true}
	all, err := users.Options(ctx, db, "")
	require.NoError(t, err)
	labels := map[string]bool{}
	for _, o := range all {
		labels[o.Label] = true
	}
	assert.True(t, labels["爱丽丝"] && labels["鲍勃_%"])
	assert.False(t, labels["爱丽丝二号"], "soft-deleted rows are hidden")

	found, err := users.Options(ctx, db, "爱丽")
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, strconv.FormatInt(alice, 10), found[0].ID)
	found, err = users.Options(ctx, db, "_%")
	require.NoError(t, err)
	require.Len(t, found, 1, "LIKE wildcards are literal")
	assert.Equal(t, strconv.FormatInt(bob, 10), found[0].ID)

	// A search form also lists soft-deleted rows, marked.
	found, err = users.Find(ctx, db, Query{Keyword: "爱丽", Deleted: true})
	require.NoError(t, err)
	require.Len(t, found, 2)
	assert.Equal(t, Option{ID: strconv.FormatInt(gone, 10), Label: "爱丽丝二号（已删除）", Deleted: true}, found[1])

	// An integer label is matched exactly (its index), never with LIKE.
	byID := Target{Table: "sys_users", Label: "id", NumberLabel: true, SoftDelete: true}
	found, err = byID.Find(ctx, db, Query{Keyword: strconv.FormatInt(bob, 10)})
	require.NoError(t, err)
	assert.Equal(t, []Option{{ID: strconv.FormatInt(bob, 10), Label: strconv.FormatInt(bob, 10)}}, found)
	found, err = byID.Find(ctx, db, Query{Keyword: "bob"})
	require.NoError(t, err)
	assert.Empty(t, found)

	// A text key (a code).
	byName := Target{Table: "sys_users", Key: "username", Label: "nickname", SoftDelete: true}
	found, err = byName.Options(ctx, db, "鲍勃")
	require.NoError(t, err)
	assert.Equal(t, []Option{{ID: "bob", Label: "鲍勃_%"}}, found)
	ok, err := byName.Exists(ctx, db, "bob")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = users.Exists(ctx, db, alice)
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = users.Exists(ctx, db, gone)
	require.NoError(t, err)
	assert.False(t, ok)

	_, err = Target{Table: "sys_users; drop", Label: "nickname"}.Options(ctx, db, "")
	assert.Error(t, err, "identifiers are checked")
}
