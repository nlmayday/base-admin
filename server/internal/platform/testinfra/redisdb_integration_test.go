//go:build integration

package testinfra

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dbClient(t *testing.T, addr string, index int) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: addr, DB: index})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// releasedDB leases a database in a subtest, writes key there and returns
// the index once the lease is released. setup runs before the release.
func releasedDB(t *testing.T, key string, setup func(*testing.T, *redis.Client)) int {
	t.Helper()
	var index int
	t.Run("lease", func(t *testing.T) {
		cfg := RedisDB(t)
		index = cfg.DB
		client := dbClient(t, cfg.Addr, index)
		require.NoError(t, client.Set(context.Background(), key, "v", 0).Err())
		if setup != nil {
			setup(t, client)
		}
	})
	return index
}

func TestRedisDBLeasesSeparateDatabases(t *testing.T) {
	ctx := context.Background()
	a, b := RedisDB(t), RedisDB(t)
	assert.NotZero(t, a.DB)
	assert.NotZero(t, b.DB)
	assert.NotEqual(t, a.DB, b.DB)

	client := dbClient(t, a.Addr, a.DB)
	assert.Equal(t, int64(1), client.Exists(ctx, leaseKey).Val())
	assert.Positive(t, client.PTTL(ctx, leaseKey).Val())
}

func TestRedisDBFlushesOnRelease(t *testing.T) {
	ctx := context.Background()
	addr := Redis(t).Addr
	key := "k:" + randomToken()
	client := dbClient(t, addr, releasedDB(t, key, nil))
	assert.Zero(t, client.Exists(ctx, key).Val(), "test data is gone")
}

func TestFlushRedisKeepsTheLease(t *testing.T) {
	ctx := context.Background()
	addr := Redis(t).Addr
	key, after := "k:"+randomToken(), "k:"+randomToken()
	index := releasedDB(t, key, func(t *testing.T, client *redis.Client) {
		lease := client.Get(ctx, leaseKey).Val()
		FlushRedis(t, client)
		assert.Zero(t, client.Exists(ctx, key).Val())
		assert.Equal(t, lease, client.Get(ctx, leaseKey).Val())
		assert.Equal(t, int64(1), client.Exists(ctx, markerKey).Val())
		require.NoError(t, client.Set(ctx, after, "v", 0).Err())
	})
	assert.Zero(t, dbClient(t, addr, index).Exists(ctx, after).Val(), "data written after the flush is released too")
}

func TestRedisDBCleansUpAfterAPlainFlush(t *testing.T) {
	ctx := context.Background()
	addr := Redis(t).Addr
	key := "k:" + randomToken()
	index := releasedDB(t, "first", func(t *testing.T, client *redis.Client) {
		require.NoError(t, client.FlushDB(ctx).Err())
		require.NoError(t, client.Set(ctx, key, "v", 0).Err())
	})
	assert.Zero(t, dbClient(t, addr, index).Exists(ctx, key).Val(), "the database does not look like someone else's data")
}

func TestRedisDBSkipsDatabasesWithOtherData(t *testing.T) {
	ctx := context.Background()
	addr := Redis(t).Addr
	client := dbClient(t, addr, releasedDB(t, "k", nil))
	require.NoError(t, client.Set(ctx, "dev:session", "keep", 0).Err())
	t.Cleanup(func() { _ = client.Del(context.Background(), "dev:session").Err() })

	ok, err := acquire(ctx, client, randomToken())
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "keep", client.Get(ctx, "dev:session").Val())
}

func TestRedisDBReusesDatabasesLeftByDeadRuns(t *testing.T) {
	ctx := context.Background()
	addr := Redis(t).Addr
	client := dbClient(t, addr, releasedDB(t, "k", nil))
	// A run that died: test data and the marker, the lease expired.
	require.NoError(t, client.MSet(ctx, markerKey, "1", "k", "v").Err())

	token := randomToken()
	ok, err := acquire(ctx, client, token)
	require.NoError(t, err)
	if !ok {
		t.Skip("another test run took the database first")
	}
	t.Cleanup(func() { _ = release(context.Background(), client, token) })
	assert.Zero(t, client.Exists(ctx, "k").Val(), "leftover data is flushed")
	assert.Equal(t, token, client.Get(ctx, leaseKey).Val())
}
