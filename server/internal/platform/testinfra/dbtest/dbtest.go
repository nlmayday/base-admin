//go:build integration

// Package dbtest holds SQL helpers for integration tests that must run on
// both dialects. It is separate from testinfra, which package database's
// own tests import.
package dbtest

import (
	"testing"

	"gorm.io/gorm"

	"base-admin/server/internal/platform/database"
)

// InsertID runs an INSERT (without RETURNING) and returns the new row's id.
func InsertID(t testing.TB, db *gorm.DB, query string, args ...any) int64 {
	t.Helper()
	id, err := database.InsertID(db, query, args...)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	return id
}
