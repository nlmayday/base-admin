//go:build integration

package testinfra

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"base-admin/server/internal/platform/config"
)

// Keys that RedisDB keeps in a leased database. The marker stays while the
// database holds test data, so a run that died before releasing its lease
// does not leave the database unusable.
const (
	leaseKey  = "testinfra:lease"
	markerKey = "testinfra:owned"
)

const (
	leaseTTL     = 30 * time.Second
	leaseWait    = 2 * time.Minute
	leaseRetryIn = 200 * time.Millisecond
)

// acquireScript takes the lease when nobody holds it and the database is
// either empty or holds only test data, and empties the database.
var acquireScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then return 0 end
if redis.call('DBSIZE') > 0 and redis.call('EXISTS', KEYS[2]) == 0 then return 0 end
redis.call('FLUSHDB')
redis.call('SET', KEYS[2], '1')
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
return 1`)

// refreshScript extends the lease, taking it back if it went missing.
var refreshScript = redis.NewScript(`
local owner = redis.call('GET', KEYS[1])
if owner and owner ~= ARGV[1] then return 0 end
redis.call('SET', KEYS[2], '1')
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
return 1`)

// releaseScript empties the database unless another run holds it. A
// missing lease means it expired or the test flushed it; either way the
// data is this test's.
var releaseScript = redis.NewScript(`
local owner = redis.call('GET', KEYS[1])
if owner and owner ~= ARGV[1] then return 0 end
redis.call('FLUSHDB')
return 1`)

// flushScript empties the database but keeps the lease and the marker.
var flushScript = redis.NewScript(`
local owner = redis.call('GET', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
redis.call('FLUSHDB')
redis.call('SET', KEYS[2], '1')
if owner and ttl > 0 then redis.call('SET', KEYS[1], owner, 'PX', ttl) end
return 1`)

// RedisDB leases a Redis logical database for the test, flushes it and
// returns a config pointing at it.
//
// Test databases reuse the same user and job IDs, so tests sharing a
// logical database would overwrite each other's sessions, rate limits and
// locks. A lease gives each running test its own database, also across
// `go test` processes and checkouts that share one Redis.
//
// Database 0 and databases holding other data (a development server's, for
// example) are never used: only empty databases and those left by earlier
// test runs qualify. The database is flushed again when the test ends; if
// the process dies the lease expires after 30 seconds. Tests that need an
// empty database mid-test call FlushRedis rather than FLUSHDB.
func RedisDB(t *testing.T) config.RedisConfig {
	t.Helper()
	cfg := Redis(t)
	ctx := context.Background()
	token := randomToken()

	admin := redis.NewClient(&redis.Options{Addr: cfg.Addr})
	defer func() { _ = admin.Close() }()
	count := databaseCount(ctx, admin)

	deadline := time.Now().Add(leaseWait)
	for {
		for index := 1; index < count; index++ {
			client := redis.NewClient(&redis.Options{Addr: cfg.Addr, DB: index, PoolSize: 1})
			ok, err := acquire(ctx, client, token)
			if err != nil {
				_ = client.Close()
				t.Fatalf("lease redis db %d: %v", index, err)
			}
			if ok {
				keepLease(t, client, token)
				cfg.DB = index
				return cfg
			}
			_ = client.Close()
		}
		if time.Now().After(deadline) {
			t.Fatalf("no free Redis logical database on %s after %s: databases 1-%d are leased by other test runs or hold other data",
				cfg.Addr, leaseWait, count-1)
		}
		time.Sleep(leaseRetryIn)
	}
}

// FlushRedis empties a database leased by RedisDB, as a Redis data loss
// would, without giving up the lease.
func FlushRedis(t *testing.T, client *redis.Client) {
	t.Helper()
	if err := flushScript.Run(context.Background(), client, []string{leaseKey, markerKey}).Err(); err != nil {
		t.Fatalf("flush redis db %d: %v", client.Options().DB, err)
	}
}

func acquire(ctx context.Context, client *redis.Client, token string) (bool, error) {
	n, err := acquireScript.Run(ctx, client, []string{leaseKey, markerKey}, token, leaseTTL.Milliseconds()).Int()
	return n == 1, err
}

func release(ctx context.Context, client *redis.Client, token string) error {
	return releaseScript.Run(ctx, client, []string{leaseKey}, token).Err()
}

// keepLease refreshes the lease while the test runs, then empties the
// database and lets it go.
func keepLease(t *testing.T, client *redis.Client, token string) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() {
		ticker := time.NewTicker(leaseTTL / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := refreshScript.Run(ctx, client, []string{leaseKey, markerKey}, token, leaseTTL.Milliseconds()).Int()
				if err == nil && n == 0 {
					t.Errorf("lost the lease on redis db %d to another test run", client.Options().DB)
					return
				}
			}
		}
	})
	t.Cleanup(func() {
		cancel()
		wg.Wait()
		defer func() { _ = client.Close() }()
		if err := release(context.Background(), client, token); err != nil {
			t.Errorf("release redis db %d: %v", client.Options().DB, err)
		}
	})
}

// databaseCount returns the server's number of logical databases, or the
// Redis default when CONFIG is disabled.
func databaseCount(ctx context.Context, client *redis.Client) int {
	values, err := client.ConfigGet(ctx, "databases").Result()
	if err == nil {
		if n, err := strconv.Atoi(values["databases"]); err == nil && n > 1 {
			return n
		}
	}
	return 16
}

func randomToken() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
