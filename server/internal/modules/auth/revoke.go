package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
)

// errRevocationSideEffect is returned when the Redis generation was already
// raised (old sessions are gone) but the database change did not commit.
// The raise is an intentional, irreversible safety side effect (6.5.3).
var errRevocationSideEffect = httpx.NewError(http.StatusServiceUnavailable, httpx.CodeUnavailable,
	"操作未完成，该用户现有会话可能已失效，请重试")

// Revoker ends all sessions of a user together with a database change, per
// design doc 6.5.3.
type Revoker struct {
	db          *gorm.DB
	sessions    *SessionStore
	versions    *Versions
	lockTimeout time.Duration
	logger      *zap.Logger
	hooks       revokeHooks
}

// revokeHooks let tests pause or fail the protocol at precise points. They
// are nil in production.
type revokeHooks struct {
	afterLock      func()
	afterRevokeGen func() error
	// skipCleanup proves correctness does not depend on deleting keys.
	skipCleanup bool
}

// RevokeSpec describes the change made together with the revocation.
type RevokeSpec struct {
	// Prepare runs after the user row is locked and before any Redis write;
	// validation errors here have no side effects.
	Prepare func(tx *gorm.DB) error
	// Apply makes the change after the generation was raised.
	Apply func(tx *gorm.DB, newGen int64) error
}

// Revoke locks the user, raises the generation mirror to max(mirror, pg)+1,
// stores it in PostgreSQL, applies the change and bumps the auth version, all
// in one transaction. It returns the new generation.
func (r *Revoker) Revoke(ctx context.Context, uid int64, spec RevokeSpec) (int64, error) {
	var newGen, ver int64
	err := database.RunTx(ctx, r.db, func(tx *gorm.DB) error {
		pgGen, err := lockUser(tx, uid, r.lockTimeout, "FOR UPDATE")
		if err != nil {
			return err
		}
		if r.hooks.afterLock != nil {
			r.hooks.afterLock()
		}
		if spec.Prepare != nil {
			if err := spec.Prepare(tx); err != nil {
				return err
			}
		}
		n, err := r.sessions.RevokeGen(ctx, uid, pgGen)
		if err == nil && r.hooks.afterRevokeGen != nil {
			err = r.hooks.afterRevokeGen()
		}
		if err != nil {
			// The script may have run before the error surfaced (B1').
			return errRevocationSideEffect.WithCause(err)
		}
		newGen = n
		// GREATEST keeps invariant I3 (never decreases) in the statement
		// itself; MySQL has no trigger for it (migrations/mysql/000003).
		if err := tx.Exec("UPDATE sys_users SET session_gen = GREATEST(session_gen, ?), updated_at = CURRENT_TIMESTAMP(6) WHERE id = ?", n, uid).Error; err != nil {
			return err
		}
		if spec.Apply != nil {
			if err := spec.Apply(tx, n); err != nil {
				return err
			}
		}
		ver, err = Bump(tx)
		return err
	})
	if err != nil {
		return 0, r.describeFailure(err, newGen)
	}
	r.versions.publishAfterCommit(ctx, ver)
	if r.hooks.skipCleanup {
		return newGen, nil
	}
	if err := r.sessions.DeleteAllForUser(ctx, uid); err != nil {
		r.logger.Warn("session cleanup failed; generation already revokes them", zap.Int64("user_id", uid), zap.Error(err))
	}
	return newGen, nil
}

// describeFailure maps errors after the generation was raised to the B2
// message unless the outcome is unknown (B4), which keeps its own code.
func (r *Revoker) describeFailure(err error, newGen int64) error {
	if apiErr, ok := errors.AsType[*httpx.Error](err); ok && apiErr.Code == httpx.CodeOutcomeUnknown {
		return err
	}
	if newGen > 0 {
		return errRevocationSideEffect.WithCause(err)
	}
	return err
}

// lockUser locks the user row and returns its session generation.
func lockUser(tx *gorm.DB, uid int64, lockTimeout time.Duration, mode string) (int64, error) {
	if err := database.SetLockTimeout(tx, lockTimeout); err != nil {
		return 0, err
	}
	var gen int64
	res := tx.Raw("SELECT session_gen FROM sys_users WHERE id = ? AND deleted_at IS NULL "+mode, uid).Scan(&gen)
	if res.Error != nil {
		if database.IsLockTimeout(res.Error) {
			return 0, errBusy.WithCause(res.Error)
		}
		return 0, res.Error
	}
	if res.RowsAffected == 0 {
		return 0, httpx.NotFound("用户不存在")
	}
	return gen, nil
}
