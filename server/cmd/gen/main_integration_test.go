//go:build integration

package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/testinfra"
	"base-admin/server/internal/platform/testinfra/migrated"
)

func TestGenerateFromRealTable(t *testing.T) {
	cfg := migrated.Database(t)
	sqlDB, err := database.OpenSQL(context.Background(), cfg, "gen-test")
	require.NoError(t, err)
	defer func() { _ = sqlDB.Close() }()
	ddl := `CREATE TABLE biz_widgets (
		id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		name varchar(64) NOT NULL,
		status smallint NOT NULL DEFAULT 1,
		version integer NOT NULL DEFAULT 0,
		created_at timestamptz NOT NULL DEFAULT now())`
	if testinfra.IsMySQL(t) {
		ddl = `CREATE TABLE biz_widgets (
		id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
		name varchar(64) NOT NULL,
		status smallint NOT NULL DEFAULT 1,
		version integer NOT NULL DEFAULT 0,
		created_at datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6))`
	}
	_, err = sqlDB.Exec(ddl)
	require.NoError(t, err)

	root := fakeRepo(t)
	args := []string{"-table", "biz_widgets", "-module", "demo", "-resource", "widget", "-name", "部件",
		"-root", root, "-dsn", cfg.URL, "-no-format"}
	code, stdout, stderr := runGen(t, args, loadTable)
	require.Equal(t, 0, code, stderr)
	assert.FileExists(t, filepath.Join(root, "server", "internal", "modules", "demo", "widget.go"))
	assert.Contains(t, stdout, "demo:widget:export")

	args[1] = "no_such_table"
	code, _, stderr = runGen(t, args, loadTable)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "table not found")
}
