package auth

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/password"
)

// errAccountUnavailable means the account cannot log in (disabled, deleted
// or changed concurrently). Clients only ever see errBadCredentials.
var errAccountUnavailable = errors.New("account unavailable")

// Options configures the auth core.
type Options struct {
	DB        *gorm.DB
	Redis     *redis.Client
	Config    config.AuthConfig
	APIPrefix string
	Logger    *zap.Logger
	Password  password.Params
	Now       func() time.Time
	// LoginObserver is optional; nil discards login events.
	LoginObserver LoginObserver
}

// Core holds the shared auth services used by the auth module, the guard
// and other modules (IAM uses Revoker, Versions and HashPassword).
type Core struct {
	cfg       config.AuthConfig
	db        *gorm.DB
	logger    *zap.Logger
	pw        password.Params
	now       func() time.Time
	dummyHash string
	limiter   *loginLimiter
	// hashSlots caps concurrent Argon2 work (19 MiB each) so bursts cannot
	// exhaust memory or CPU.
	hashSlots chan struct{}
	// afterLoginLock lets tests pause login while it holds the row lock.
	afterLoginLock func()
	observer       LoginObserver
	policies       *policyStore
	captchas       *captchas
	lockout        *lockout

	Sessions  *SessionStore
	Versions  *Versions
	Snapshots *Snapshots
	Guard     *Guard
	Revoker   *Revoker
}

// NewCore wires the auth services.
func NewCore(o Options) (*Core, error) {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	dummy, err := password.Hash("timing-equalizer", o.Password)
	if err != nil {
		return nil, err
	}
	observer := o.LoginObserver
	if observer == nil {
		observer = noopObserver{}
	}
	sessions := NewSessionStore(o.Redis, o.Config.IdleTimeout, o.Config.AbsoluteTimeout, o.Config.TouchInterval)
	versions := NewVersions(o.DB, o.Redis, o.Logger)
	snapshots := NewSnapshots(o.DB, o.Redis, sessions, o.Config.SnapshotTTL)
	origins := map[string]bool{}
	for _, origin := range o.Config.AllowedOrigins {
		origins[origin] = true
	}
	return &Core{
		cfg: o.Config, db: o.DB, logger: o.Logger, pw: o.Password, now: now, dummyHash: dummy,
		hashSlots: make(chan struct{}, max(2, runtime.NumCPU())),
		observer:  observer,
		policies:  &policyStore{db: o.DB, ttl: 10 * time.Second},
		captchas:  newCaptchas(o.Redis),
		lockout:   &lockout{rdb: o.Redis},
		limiter: &loginLimiter{rdb: o.Redis, window: o.Config.LoginWindow,
			maxIP: int64(o.Config.LoginMaxPerIP), maxAccount: int64(o.Config.LoginMaxPerAccount)},
		Sessions: sessions, Versions: versions, Snapshots: snapshots,
		Guard: &Guard{
			db: o.DB, cookieName: o.Config.CookieName, sessions: sessions, versions: versions,
			snapshots: snapshots, logger: o.Logger, allowedOrigins: origins, now: now,
			passwordChangeRoutes: map[string]bool{
				o.APIPrefix + "/me": true, o.APIPrefix + "/me/menus": true,
				o.APIPrefix + "/me/password": true, o.APIPrefix + "/auth/logout": true,
			},
		},
		Revoker: &Revoker{db: o.DB, sessions: sessions, versions: versions, lockTimeout: o.Config.LockTimeout, logger: o.Logger},
	}, nil
}

func (c *Core) acquireHashSlot(ctx context.Context) (release func(), err error) {
	select {
	case c.hashSlots <- struct{}{}:
		return func() { <-c.hashSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ValidatePassword checks pw against the security policy and returns a
// 422 error on field when it does not comply.
func (c *Core) ValidatePassword(ctx context.Context, field, pw string) error {
	p, err := c.policies.get(ctx)
	if err != nil {
		return err
	}
	if msg := p.CheckPassword(pw); msg != "" {
		return httpx.Validation([]httpx.FieldError{{Field: field, Message: msg}})
	}
	return nil
}

// HashPassword hashes a password with the configured parameters.
func (c *Core) HashPassword(ctx context.Context, pw string) (string, error) {
	release, err := c.acquireHashSlot(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	return password.Hash(pw, c.pw)
}

// verify checks a password against a stored hash within the hashing cap.
func (c *Core) verify(ctx context.Context, pw, encoded string) (bool, error) {
	release, err := c.acquireHashSlot(ctx)
	if err != nil {
		return false, err
	}
	defer release()
	return password.Verify(pw, encoded)
}

// ResetPassword sets a new password chosen by an administrator, ends all of
// the user's sessions and forces a change at next login.
func (c *Core) ResetPassword(ctx context.Context, uid int64, newPassword string, prepare func(tx *gorm.DB) error) error {
	if err := c.ValidatePassword(ctx, "password", newPassword); err != nil {
		return err
	}
	h, err := c.HashPassword(ctx, newPassword)
	if err != nil {
		return err
	}
	_, err = c.Revoker.Revoke(ctx, uid, RevokeSpec{
		Prepare: prepare,
		Apply: func(tx *gorm.DB, _ int64) error {
			return tx.Exec(`UPDATE sys_users SET password_hash = ?, password_changed_at = now(),
				must_change_password = true, version = version + 1 WHERE id = ?`, h, uid).Error
		},
	})
	return err
}

// checkCredentials verifies a password. Unknown accounts still cost one
// hash verification so response timing does not reveal them.
func (c *Core) checkCredentials(ctx context.Context, account, pw string) (int64, string, bool, error) {
	var row struct {
		ID           int64
		PasswordHash string
		Status       int16
	}
	res := c.db.WithContext(ctx).Raw(`SELECT id, password_hash, status FROM sys_users
		WHERE lower(username) = ? AND deleted_at IS NULL`, account).Scan(&row)
	if res.Error != nil {
		return 0, "", false, fmt.Errorf("load credentials: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		_, _ = c.verify(ctx, pw, c.dummyHash)
		return 0, "", false, nil
	}
	match, err := c.verify(ctx, pw, row.PasswordHash)
	if err != nil {
		c.logger.Error("stored password hash is malformed", zap.Int64("user_id", row.ID), zap.Error(err))
		return 0, "", false, nil
	}
	if !match || row.Status != actor.StatusEnabled {
		return 0, "", false, nil
	}
	return row.ID, row.PasswordHash, true, nil
}

// establishSession creates a session under a shared lock on the user row,
// so it cannot interleave with a revocation (design doc 6.5.4). The session
// is stamped with max(pg generation, mirror).
func (c *Core) establishSession(ctx context.Context, uid int64, expectedHash, ip, ua string) (string, Session, error) {
	var token string
	var created *Session
	err := database.RunTx(ctx, c.db, func(tx *gorm.DB) error {
		if err := database.SetLockTimeout(tx, c.cfg.LockTimeout); err != nil {
			return err
		}
		var row struct {
			SessionGen   int64
			Status       int16
			PasswordHash string
		}
		res := tx.Raw(`SELECT session_gen, status, password_hash FROM sys_users
			WHERE id = ? AND deleted_at IS NULL FOR SHARE`, uid).Scan(&row)
		if res.Error != nil {
			if database.IsLockTimeout(res.Error) {
				return errBusy.WithCause(res.Error)
			}
			return res.Error
		}
		if c.afterLoginLock != nil {
			c.afterLoginLock()
		}
		if res.RowsAffected == 0 || row.Status != actor.StatusEnabled || row.PasswordHash != expectedHash {
			return errAccountUnavailable
		}
		mirror, found, err := c.Sessions.GenMirror(ctx, uid)
		if err != nil {
			return err
		}
		if !found {
			if mirror, err = c.Sessions.RaiseGen(ctx, uid, row.SessionGen); err != nil {
				return err
			}
		}
		tok, sess, err := c.Sessions.Create(ctx, uid, max(row.SessionGen, mirror), ip, ua, c.now())
		if err != nil {
			return err
		}
		token, created = tok, &sess
		return nil
	})
	if err != nil {
		if created != nil {
			_ = c.Sessions.Delete(ctx, *created)
		}
		return "", Session{}, err
	}
	return token, *created, nil
}

// actorFor builds the actor of a freshly created session.
func (c *Core) actorFor(ctx context.Context, sess Session, ip, ua string) (*actor.Actor, error) {
	ver, err := c.Versions.Current(ctx)
	if err != nil {
		return nil, unavailable(err)
	}
	snap, err := c.Snapshots.Load(ctx, ver, sess.UserID)
	if err != nil {
		return nil, unavailable(err)
	}
	return &actor.Actor{Snapshot: snap, SessionHash: sess.Hash, IP: ip, UserAgent: ua}, nil
}
