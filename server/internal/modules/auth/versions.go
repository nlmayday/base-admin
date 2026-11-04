package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/database"
)

// Versions mirrors the global auth version from PostgreSQL (the source of
// truth) into Redis. Changes are bounded-delay eventually consistent; see
// design doc 5.6.
type Versions struct {
	db     *gorm.DB
	rdb    *redis.Client
	logger *zap.Logger
}

// NewVersions creates a version mirror.
func NewVersions(db *gorm.DB, rdb *redis.Client, logger *zap.Logger) *Versions {
	return &Versions{db: db, rdb: rdb, logger: logger}
}

// Current returns the mirrored version. A missing mirror is a cache miss,
// never zero: it is reloaded from PostgreSQL.
func (v *Versions) Current(ctx context.Context) (int64, error) {
	ver, err := v.rdb.Get(ctx, versionKey).Int64()
	if err == nil {
		return ver, nil
	}
	if !errors.Is(err, redis.Nil) {
		return 0, fmt.Errorf("read auth version: %w", err)
	}
	return v.Reconcile(ctx)
}

// Reconcile copies the PostgreSQL version into Redis if Redis is behind.
func (v *Versions) Reconcile(ctx context.Context) (int64, error) {
	var ver int64
	if err := v.db.WithContext(ctx).Raw("SELECT version FROM sys_auth_version WHERE id = 1").Scan(&ver).Error; err != nil {
		return 0, fmt.Errorf("read auth version from database: %w", err)
	}
	return v.publish(ctx, ver)
}

func (v *Versions) publish(ctx context.Context, ver int64) (int64, error) {
	cur, err := maxSetScript.Run(ctx, v.rdb, []string{versionKey}, ver).Int64()
	if err != nil {
		return 0, fmt.Errorf("publish auth version: %w", err)
	}
	return cur, nil
}

// publishAfterCommit retries a few times; the reconciler covers failures.
func (v *Versions) publishAfterCommit(ctx context.Context, ver int64) {
	var err error
	for range 3 {
		if _, err = v.publish(ctx, ver); err == nil {
			return
		}
	}
	v.logger.Warn("auth version not published; reconciler will converge", zap.Int64("version", ver), zap.Error(err))
}

// Bump increments the database version inside tx and returns it.
func Bump(tx *gorm.DB) (int64, error) {
	// The UPDATE keeps the row locked until tx ends, so the SELECT reads
	// this transaction's own increment.
	if err := tx.Exec("UPDATE sys_auth_version SET version = version + 1 WHERE id = 1").Error; err != nil {
		return 0, fmt.Errorf("bump auth version: %w", err)
	}
	var ver int64
	if err := tx.Raw("SELECT version FROM sys_auth_version WHERE id = 1").Scan(&ver).Error; err != nil {
		return 0, fmt.Errorf("read auth version: %w", err)
	}
	return ver, nil
}

// Mutate runs fn in a transaction that also bumps the auth version, then
// publishes the version. Every change that affects authorization (grants,
// role status, user roles, departments, data scopes) must go through it.
func (v *Versions) Mutate(ctx context.Context, fn func(tx *gorm.DB) error) error {
	var ver int64
	err := database.RunTx(ctx, v.db, func(tx *gorm.DB) error {
		if err := fn(tx); err != nil {
			return err
		}
		var err error
		ver, err = Bump(tx)
		return err
	})
	if err != nil {
		return err
	}
	v.publishAfterCommit(ctx, ver)
	return nil
}

// RunReconciler compares PostgreSQL and Redis every interval until ctx ends.
func (v *Versions) RunReconciler(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := v.Reconcile(ctx); err != nil && ctx.Err() == nil {
				v.logger.Warn("auth version reconcile failed", zap.Error(err))
			}
		}
	}
}
