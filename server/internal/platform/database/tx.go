package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"base-admin/server/internal/platform/httpx"
)

// PostgreSQL error codes the application reacts to.
const (
	pgUniqueViolation  = "23505"
	pgForeignKey       = "23503"
	pgLockNotAvailable = "55P03"
)

type fnError struct{ err error }

func (e fnError) Error() string { return e.err.Error() }
func (e fnError) Unwrap() error { return e.err }

// RunTx runs fn in a transaction. Errors returned by fn are passed through
// unchanged. When fn succeeded but the transaction still failed, COMMIT
// itself failed and the outcome is unknown, so httpx.OutcomeUnknown is
// returned (design doc 6.5.5 B4).
func RunTx(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	ran := false
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ran = true
		if err := fn(tx); err != nil {
			return fnError{err}
		}
		return nil
	})
	if err == nil {
		return nil
	}
	if fe, ok := errors.AsType[fnError](err); ok {
		return fe.err
	}
	if !ran {
		return fmt.Errorf("begin transaction: %w", err)
	}
	return httpx.OutcomeUnknown(err)
}

// SetLockTimeout bounds row-lock waits for the rest of the transaction.
func SetLockTimeout(tx *gorm.DB, d time.Duration) error {
	// SET cannot take bind parameters; the value is an integer we format.
	return tx.Exec(fmt.Sprintf("SET LOCAL lock_timeout = %d", d.Milliseconds())).Error
}

func pgCode(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code
	}
	return ""
}

// IsUniqueViolation reports a unique constraint violation.
func IsUniqueViolation(err error) bool {
	return pgCode(err) == pgUniqueViolation || errors.Is(err, gorm.ErrDuplicatedKey)
}

// UniqueConstraint returns the name of the violated unique constraint or
// index when err is a unique violation.
func UniqueConstraint(err error) (string, bool) {
	if !IsUniqueViolation(err) {
		return "", false
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.ConstraintName, true
	}
	return "", true
}

// IsForeignKeyViolation reports a foreign key violation.
func IsForeignKeyViolation(err error) bool {
	return pgCode(err) == pgForeignKey || errors.Is(err, gorm.ErrForeignKeyViolated)
}

// IsLockTimeout reports that a row lock could not be acquired in time.
func IsLockTimeout(err error) bool { return pgCode(err) == pgLockNotAvailable }
