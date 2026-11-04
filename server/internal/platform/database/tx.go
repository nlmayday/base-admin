package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
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

// MySQL error numbers with the same meaning.
const (
	myDuplicateEntry  = 1062
	myRowIsReferenced = 1451
	myNoReferencedRow = 1452
	myLockWaitTimeout = 1205
	myLockNowait      = 3572
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
	// A nested RunTx shares the outer state, so only the outermost
	// transaction restores the lock wait, after all of its statements.
	lt, nested := ctx.Value(lockTimeoutKey{}).(*lockTimeoutState)
	if !nested {
		lt = &lockTimeoutState{}
		ctx = context.WithValue(ctx, lockTimeoutKey{}, lt)
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) (err error) {
		ran = true
		if !nested {
			// Deferred, so a panic in fn cannot return the connection to
			// the pool with the shorter wait.
			defer func() {
				if rerr := lt.restore(tx); rerr != nil && err == nil {
					err = rerr
				}
			}()
		}
		if fnErr := fn(tx); fnErr != nil {
			return fnError{fnErr}
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
// PostgreSQL scopes the setting to the transaction (SET LOCAL). MySQL has
// no transaction-scoped form, so the session value is set here and RunTx
// restores the server default before the transaction ends; the pooled
// connection never keeps the shorter wait.
func SetLockTimeout(tx *gorm.DB, d time.Duration) error {
	if IsMySQL(tx) {
		lt, ok := tx.Statement.Context.Value(lockTimeoutKey{}).(*lockTimeoutState)
		if !ok {
			return errors.New("database.SetLockTimeout must run inside database.RunTx on MySQL")
		}
		// innodb_lock_wait_timeout counts whole seconds, at least 1.
		secs := max(int64((d+time.Second-1)/time.Second), 1)
		if err := tx.Exec(fmt.Sprintf("SET SESSION innodb_lock_wait_timeout = %d", secs)).Error; err != nil {
			return err
		}
		lt.set = true
		return nil
	}
	// SET cannot take bind parameters; the value is an integer we format.
	return tx.Exec(fmt.Sprintf("SET LOCAL lock_timeout = %d", d.Milliseconds())).Error
}

type lockTimeoutKey struct{}

// lockTimeoutState records that a MySQL transaction changed its lock wait.
type lockTimeoutState struct{ set bool }

func (s *lockTimeoutState) restore(tx *gorm.DB) error {
	if !s.set {
		return nil
	}
	s.set = false
	if err := tx.Exec("SET SESSION innodb_lock_wait_timeout = DEFAULT").Error; err != nil {
		return fmt.Errorf("restore lock wait timeout: %w", err)
	}
	return nil
}

// LockTx takes an exclusive lock named key that is held until the
// transaction ends, serializing transactions that take the same key.
// PostgreSQL uses a transaction-level advisory lock; MySQL, whose named
// locks outlive transactions, locks a row of sys_tx_locks instead.
func LockTx(tx *gorm.DB, key string) error {
	if IsMySQL(tx) {
		return tx.Exec("INSERT INTO sys_tx_locks (name) VALUES (?) ON DUPLICATE KEY UPDATE name = name", key).Error
	}
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", key).Error
}

func pgCode(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code
	}
	return ""
}

func mysqlNumber(err error) uint16 {
	if myErr, ok := errors.AsType[*mysql.MySQLError](err); ok {
		return myErr.Number
	}
	return 0
}

// IsUniqueViolation reports a unique constraint violation.
func IsUniqueViolation(err error) bool {
	return pgCode(err) == pgUniqueViolation || mysqlNumber(err) == myDuplicateEntry ||
		errors.Is(err, gorm.ErrDuplicatedKey)
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
	if myErr, ok := errors.AsType[*mysql.MySQLError](err); ok {
		return mysqlDuplicateKey(myErr.Message), true
	}
	return "", true
}

// mysqlDuplicateKey reads the index name from "Duplicate entry 'x' for key
// 'table.index'". MySQL names a primary key PRIMARY; PostgreSQL would say
// <table>_pkey, so callers match unique indexes by name, never the key.
func mysqlDuplicateKey(msg string) string {
	const marker = " for key '"
	i := strings.LastIndex(msg, marker)
	if i < 0 {
		return ""
	}
	key := strings.TrimSuffix(msg[i+len(marker):], "'")
	if dot := strings.LastIndexByte(key, '.'); dot >= 0 {
		key = key[dot+1:]
	}
	return key
}

// IsForeignKeyViolation reports a foreign key violation.
func IsForeignKeyViolation(err error) bool {
	n := mysqlNumber(err)
	return pgCode(err) == pgForeignKey || n == myRowIsReferenced || n == myNoReferencedRow ||
		errors.Is(err, gorm.ErrForeignKeyViolated)
}

// IsLockTimeout reports that a row lock could not be acquired in time.
func IsLockTimeout(err error) bool {
	n := mysqlNumber(err)
	return pgCode(err) == pgLockNotAvailable || n == myLockWaitTimeout || n == myLockNowait
}
