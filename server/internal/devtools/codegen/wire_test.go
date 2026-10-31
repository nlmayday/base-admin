package codegen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const modulesSample = `package app

import (
	"base-admin/server/internal/modules/system"
)

func build() {
	mods := []module.Module{
		system.New(),
		// codegen:modules — 代码生成器在此行之上加入新模块
	}
	_ = mods
}
`

func TestWire(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(modulesFile))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(modulesSample), 0o600))

	changed, err := Wire(root, "sales")
	require.NoError(t, err)
	assert.True(t, changed)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(got), "\t\"base-admin/server/internal/modules/sales\"\n", "imports stay sorted by gofmt")
	assert.Contains(t, string(got), "\t\tsales.New(sales.Deps{DB: d.db, Dict: systemModule}),\n\t\t// codegen:modules")

	changed, err = Wire(root, "sales")
	require.NoError(t, err)
	assert.False(t, changed, "already registered")

	require.NoError(t, os.WriteFile(path, []byte("package app\n"), 0o600))
	_, err = Wire(root, "crm")
	assert.ErrorIs(t, err, ErrNoMarkers)
}

func TestWireResource(t *testing.T) {
	root := t.TempDir()
	spec := Spec{Module: "crm", Resource: "customer", Name: "客户", NewModule: true}
	res, err := Generate(customerTable(), spec)
	require.NoError(t, err)
	module := fileByPath(t, res, "/crm/module.go")
	path := filepath.Join(root, filepath.FromSlash(module.Path))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(module.Content), 0o600))

	changed, err := WireResource(root, Spec{Module: "crm", Resource: "sales-order"})
	require.NoError(t, err)
	assert.True(t, changed)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, want := range []string{"customerPermissions,\n\t\tsalesOrderPermissions,", "customerMenus,\n\t\tsalesOrderMenus,", "m.customerRoutes(r)\n\tm.salesOrderRoutes(r)"} {
		assert.Contains(t, string(got), want)
	}

	changed, err = WireResource(root, Spec{Module: "crm", Resource: "sales-order"})
	require.NoError(t, err)
	assert.False(t, changed, "already registered")

	require.NoError(t, os.WriteFile(path, []byte("package crm\n"), 0o600))
	_, err = WireResource(root, Spec{Module: "crm", Resource: "invoice"})
	assert.ErrorIs(t, err, ErrNoMarkers)
	assert.Contains(t, res.ManualWiring, "m.customerRoutes(r)")
}
