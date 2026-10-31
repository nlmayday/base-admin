//go:build integration

package auth

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/httpx"
)

// Scenario tests for design doc 6.5.5. Each test name carries its ID.

func waitBlocked(t *testing.T, done <-chan error, d time.Duration) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("operation should still be blocked on the row lock, finished with %v", err)
	case <-time.After(d):
	}
}

func TestA1_LoginHoldsLockThenDisable(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("alice")
	c := h.newClient()

	locked, release := make(chan struct{}), make(chan struct{})
	h.core.afterLoginLock = func() { close(locked); <-release }
	loginDone := make(chan response, 1)
	go func() { loginDone <- c.login("alice") }()
	<-locked
	h.core.afterLoginLock = nil

	disableDone := make(chan error, 1)
	go func() { disableDone <- h.disable(uid) }()
	waitBlocked(t, disableDone, 300*time.Millisecond)

	close(release)
	require.Equal(t, http.StatusOK, (<-loginDone).Status)
	require.NoError(t, <-disableDone)

	assert.Less(t, c.session().Gen, h.mirror(uid), "session was stamped before the raise")
	assert.Equal(t, http.StatusUnauthorized, c.ping(), "session created just before the disable must be dead")
}

func TestA2_DisableHoldsLockThenLogin(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("bob")
	c := h.newClient()

	locked, release := make(chan struct{}), make(chan struct{})
	h.core.Revoker.hooks.afterLock = func() { close(locked); <-release }
	disableDone := make(chan error, 1)
	go func() { disableDone <- h.disable(uid) }()
	<-locked
	h.core.Revoker.hooks.afterLock = nil

	loginDone := make(chan response, 1)
	go func() { loginDone <- c.login("bob") }()
	time.Sleep(300 * time.Millisecond)
	select {
	case r := <-loginDone:
		t.Fatalf("login must wait for the disable, got %d", r.Status)
	default:
	}

	close(release)
	require.NoError(t, <-disableDone)
	resp := <-loginDone
	assert.Equal(t, http.StatusUnauthorized, resp.Status)
	assert.Equal(t, CodeBadCredentials, resp.Code)
}

func TestA3_DisableFailsAfterRaisingGenWhileLoginWaits(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("carol")
	old := h.newClient()
	old.mustLogin("carol")
	g := h.pgGen(uid)

	locked, release := make(chan struct{}), make(chan struct{})
	h.core.Revoker.hooks.afterLock = func() { close(locked); <-release }
	disableDone := make(chan error, 1)
	go func() {
		_, err := h.core.Revoker.Revoke(context.Background(), uid, RevokeSpec{
			Apply: func(*gorm.DB, int64) error { return errors.New("simulated failure after the gen raise") },
		})
		disableDone <- err
	}()
	<-locked
	h.core.Revoker.hooks.afterLock = nil

	fresh := h.newClient()
	loginDone := make(chan response, 1)
	go func() { loginDone <- fresh.login("carol") }()
	time.Sleep(200 * time.Millisecond)
	close(release)

	require.Error(t, <-disableDone)
	require.Equal(t, http.StatusOK, (<-loginDone).Status)
	assert.Equal(t, g, h.pgGen(uid), "the database change rolled back")
	assert.Equal(t, g+1, h.mirror(uid), "the mirror raise is irreversible")
	assert.Equal(t, g+1, fresh.session().Gen, "login stamps max(pg, mirror)")
	assert.Equal(t, http.StatusOK, fresh.ping())
	assert.Equal(t, http.StatusUnauthorized, old.ping())
}

func TestB1_RedisFailureBeforeRaiseHasNoSideEffects(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("dave")
	c := h.newClient()
	c.mustLogin("dave")
	g := h.pgGen(uid)

	broken := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 100 * time.Millisecond, MaxRetries: -1})
	defer func() { _ = broken.Close() }()
	revoker := *h.core.Revoker
	revoker.sessions = NewSessionStore(broken, time.Hour, 2*time.Hour, time.Minute)

	_, err := revoker.Revoke(context.Background(), uid, RevokeSpec{
		Apply: func(tx *gorm.DB, _ int64) error {
			return tx.Exec("UPDATE sys_users SET status = 2 WHERE id = ?", uid).Error
		},
	})
	require.Error(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, httpx.AsError(err).Status)
	assert.Equal(t, g, h.pgGen(uid))
	assert.Equal(t, int16(1), h.status(uid))
	assert.Equal(t, http.StatusOK, c.ping(), "nothing changed, the session survives")
}

func TestB1b_RedisErrorAfterScriptRanIsAnIrreversibleRaise(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("erin")
	c := h.newClient()
	c.mustLogin("erin")
	g := h.pgGen(uid)

	h.core.Revoker.hooks.afterRevokeGen = func() error { return errors.New("i/o timeout") }
	err := h.disable(uid)
	h.core.Revoker.hooks.afterRevokeGen = nil

	require.Error(t, err)
	assert.Contains(t, httpx.AsError(err).Message, "会话可能已失效")
	assert.Equal(t, g, h.pgGen(uid))
	assert.Equal(t, int16(1), h.status(uid), "user not disabled")
	assert.Equal(t, http.StatusUnauthorized, c.ping(), "but old sessions already ended")
}

func TestB2B3_FailedCommitThenRetry(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("frank")
	old := h.newClient()
	old.mustLogin("frank")

	// B2: the change fails after the raise.
	_, err := h.core.Revoker.Revoke(context.Background(), uid, RevokeSpec{
		Apply: func(*gorm.DB, int64) error { return errors.New("constraint violated") },
	})
	require.Error(t, err)
	assert.Contains(t, httpx.AsError(err).Message, "会话可能已失效")
	n := h.mirror(uid)
	assert.Greater(t, n, h.pgGen(uid), "mirror ahead of PG and never rolled back")
	assert.Equal(t, http.StatusUnauthorized, old.ping())

	afterB2 := h.newClient()
	afterB2.mustLogin("frank")
	assert.Equal(t, n, afterB2.session().Gen)

	// B3: the retry must kill every session, including those stamped n.
	require.NoError(t, h.disable(uid))
	assert.Equal(t, n+1, h.pgGen(uid))
	assert.Equal(t, http.StatusUnauthorized, afterB2.ping())
}

func TestB4_CommitFailureIsReportedAsOutcomeUnknown(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("grace")
	c := h.newClient()
	c.mustLogin("grace")
	// A deferred constraint fails only at COMMIT, after fn succeeded.
	require.NoError(t, h.db.Exec(`CREATE TABLE commit_bomb (
		ref bigint REFERENCES sys_users (id) DEFERRABLE INITIALLY DEFERRED)`).Error)

	_, err := h.core.Revoker.Revoke(context.Background(), uid, RevokeSpec{
		Apply: func(tx *gorm.DB, _ int64) error {
			if err := tx.Exec("UPDATE sys_users SET status = 2 WHERE id = ?", uid).Error; err != nil {
				return err
			}
			return tx.Exec("INSERT INTO commit_bomb (ref) VALUES (-1)").Error
		},
	})
	require.Error(t, err)
	assert.Equal(t, httpx.CodeOutcomeUnknown, httpx.AsError(err).Code, "a COMMIT failure must not be reported as a plain failure")
	assert.Equal(t, http.StatusUnauthorized, c.ping(), "old sessions are gone whatever the outcome")

	// The client re-reads the state, then retries; the generation only grows.
	assert.Equal(t, int16(1), h.status(uid))
	before := h.mirror(uid)
	require.NoError(t, h.disable(uid))
	assert.Equal(t, int16(2), h.status(uid))
	assert.Equal(t, before+1, h.pgGen(uid))
}

func TestC1_MissingMirrorReloadsFromPostgres(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("heidi")
	old := h.newClient()
	old.mustLogin("heidi")
	_, err := h.core.Revoker.Revoke(context.Background(), uid, RevokeSpec{})
	require.NoError(t, err)
	current := h.newClient()
	current.mustLogin("heidi")

	require.NoError(t, h.rdb.Del(context.Background(), genKey(uid)).Err())
	assert.Equal(t, http.StatusUnauthorized, old.ping(), "missing mirror is never treated as 0")
	assert.Equal(t, http.StatusOK, current.ping())
	assert.Equal(t, h.pgGen(uid), h.mirror(uid), "mirror restored from PG")
}

func TestC2_RegressedMirrorHealsAfterReconcile(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	uid := h.createUser("ivan")
	old := h.newClient()
	old.mustLogin("ivan")
	staleVer, err := h.core.Versions.Current(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, old.ping(), "caches a snapshot for the stale version")
	oldGen := h.pgGen(uid)

	_, err = h.core.Revoker.Revoke(ctx, uid, RevokeSpec{})
	require.NoError(t, err)
	// (Not probing here: a rejected session is deleted by the guard.)

	// Simulate AOF losing the last writes: mirror and version go back.
	require.NoError(t, h.rdb.Set(ctx, genKey(uid), oldGen, 0).Err())
	require.NoError(t, h.rdb.Set(ctx, versionKey, staleVer, 0).Err())
	assert.Equal(t, http.StatusOK, old.ping(), "documented bounded window: stale data accepts the old session")

	_, err = h.core.Versions.Reconcile(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, old.ping(), "after reconcile the snapshot comes from PG and wins")
	assert.Equal(t, h.pgGen(uid), h.mirror(uid), "mirror healed by the snapshot rebuild")
}

func TestC3_RedisDataLossLogsEveryoneOut(t *testing.T) {
	h := newHarness(t)
	h.createUser("judy")
	c := h.newClient()
	c.mustLogin("judy")
	require.NoError(t, h.rdb.FlushDB(context.Background()).Err())
	assert.Equal(t, http.StatusUnauthorized, c.ping())
	c.mustLogin("judy")
	assert.Equal(t, http.StatusOK, c.ping())
}

func TestC4_RedisUnavailableFailsClosed(t *testing.T) {
	h := newHarness(t)
	h.createUser("ken")
	c := h.newClient()
	c.mustLogin("ken")

	broken := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 100 * time.Millisecond, MaxRetries: -1})
	defer func() { _ = broken.Close() }()
	g := *h.core.Guard
	g.sessions = NewSessionStore(broken, time.Hour, 2*time.Hour, time.Minute)
	h.core.Guard = &g
	h.restart()

	resp := c.do(http.MethodGet, "/api/v1/ping", nil)
	assert.Equal(t, http.StatusServiceUnavailable, resp.Status)
	assert.Equal(t, httpx.CodeUnavailable, resp.Code)
}

func TestC5_PostgresUnavailableWithMissingMirrorFailsClosed(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("leo")
	c := h.newClient()
	c.mustLogin("leo")
	require.NoError(t, h.rdb.Del(context.Background(), genKey(uid)).Err())
	sqlDB, err := h.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	resp := c.do(http.MethodGet, "/api/v1/ping", nil)
	assert.Equal(t, http.StatusServiceUnavailable, resp.Status, "must not fall back to treating the generation as 0")
}

func TestRevocationCleanupDeletesSessionKeys(t *testing.T) {
	h := newHarness(t)
	uid := h.createUser("mia")
	c := h.newClient()
	c.mustLogin("mia")
	hash := c.session().Hash
	h.core.Revoker.hooks.skipCleanup = false

	_, err := h.core.Revoker.Revoke(context.Background(), uid, RevokeSpec{})
	require.NoError(t, err)
	n, err := h.rdb.Exists(context.Background(), sessionKey(hash), userSessionsKey(uid)).Result()
	require.NoError(t, err)
	assert.Zero(t, n)
}
