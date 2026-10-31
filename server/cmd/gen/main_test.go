package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/devtools/codegen"
)

func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "server"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "web"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, "server", "go.mod"), []byte("module x\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "web", "package.json"), []byte("{}"), 0o600))
	return root
}

func fakeLoader(t codegen.Table, err error) loader {
	return func(context.Context, string, string) (codegen.Table, error) { return t, err }
}

func widgetTable() codegen.Table {
	return codegen.Table{Name: "biz_widgets", Columns: []codegen.Column{
		{Name: "id", UDT: "int8", Identity: true, Primary: true},
		{Name: "name", UDT: "varchar", MaxLength: 64},
		{Name: "version", UDT: "int4", Default: "0"},
	}}
}

func baseArgs(root string) []string {
	return []string{"-table", "biz_widgets", "-module", "demo", "-resource", "widget", "-name", "部件",
		"-root", root, "-dsn", "postgres://unused", "-no-format"}
}

func runGen(t *testing.T, args []string, load loader) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, &out, &errOut, load)
	return code, out.String(), errOut.String()
}

func TestFlagValidation(t *testing.T) {
	code, _, stderr := runGen(t, []string{"-table", "x", "-dsn", "postgres://x"}, fakeLoader(widgetTable(), nil))
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "missing required flags: -module, -name, -resource")

	t.Setenv("ADMIN_DATABASE_URL", "")
	code, _, stderr = runGen(t, []string{"-table", "x", "-module", "m", "-resource", "r", "-name", "n"}, fakeLoader(widgetTable(), nil))
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "ADMIN_DATABASE_URL")

	code, _, _ = runGen(t, []string{"-h"}, nil)
	assert.Equal(t, 0, code)
	code, _, stderr = runGen(t, append(baseArgs(t.TempDir()), "extra"), nil)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "unexpected arguments")
}

func TestDryRunWritesNothing(t *testing.T) {
	root := fakeRepo(t)
	code, stdout, stderr := runGen(t, append(baseArgs(root), "-dry-run"), fakeLoader(widgetTable(), nil))
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, "==> server/internal/modules/demo/widget.go")
	assert.Contains(t, stdout, "==> server/internal/modules/demo/module.go")
	assert.Contains(t, stdout, "demo:widget:view")
	assert.Contains(t, stdout, "demo.New(demo.Deps{DB: d.db, Dict: systemModule}),")
	_, err := os.Stat(filepath.Join(root, "server", "internal"))
	assert.True(t, os.IsNotExist(err))
}

func TestWriteAndOverwriteProtection(t *testing.T) {
	root := fakeRepo(t)
	code, stdout, stderr := runGen(t, baseArgs(root), fakeLoader(widgetTable(), nil))
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, "wrote web/src/views/demo/widget/components/WidgetFormDialog.vue")
	assert.FileExists(t, filepath.Join(root, "server", "internal", "modules", "demo", "module.go"))
	assert.FileExists(t, filepath.Join(root, "web", "src", "api", "demo", "widget.ts"))

	// The module now exists, so a second run only targets resource files,
	// which exist: refused without -force.
	code, _, stderr = runGen(t, baseArgs(root), fakeLoader(widgetTable(), nil))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "already exist")
	assert.Contains(t, stderr, "widget.go")

	marker := filepath.Join(root, "server", "internal", "modules", "demo", "module.go")
	require.NoError(t, os.WriteFile(marker, []byte("// hand edited\n"), 0o600))
	code, stdout, stderr = runGen(t, append(baseArgs(root), "-force"), fakeLoader(widgetTable(), nil))
	require.Equal(t, 0, code, stderr)
	got, err := os.ReadFile(marker)
	require.NoError(t, err)
	assert.Equal(t, "// hand edited\n", string(got), "module.go of an existing module is never regenerated")
	assert.Contains(t, stdout, "模块 demo 已存在")
}

func TestLoaderAndGeneratorErrors(t *testing.T) {
	root := fakeRepo(t)
	code, _, stderr := runGen(t, baseArgs(root), fakeLoader(codegen.Table{}, errors.New("boom")))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "boom")

	bad := widgetTable()
	bad.Columns[0].Identity = false
	code, _, stderr = runGen(t, baseArgs(root), fakeLoader(bad, nil))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "IDENTITY")
}
