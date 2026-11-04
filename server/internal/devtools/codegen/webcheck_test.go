package codegen

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestGeneratedWebChecks writes the frontend output for several table
// shapes into the web project, formats it like 生成到项目 does, and runs
// eslint and vue-tsc on it. It needs web/node_modules, so it only runs with
// CODEGEN_WEB_CHECK=1 (make codegen-web-check, and CI).
func TestGeneratedWebChecks(t *testing.T) {
	if os.Getenv("CODEGEN_WEB_CHECK") == "" {
		t.Skip("set CODEGEN_WEB_CHECK=1 to type-check the generated frontend (needs web/node_modules)")
	}
	root, err := filepath.Abs("../../../..")
	require.NoError(t, err)
	web := filepath.Join(root, "web")

	cases := []struct {
		table Table
		spec  Spec
	}{
		{customerTable(), Spec{Module: "zzcgcustomer", Resource: "customer", Name: "客户", NewModule: true}},
		{customerTable(), Spec{Module: "zzcgall", Resource: "customer", Name: "客户", NewModule: true, Fields: allSearchModes(customerTable())}},
		{mixedTable(), Spec{Module: "zzcgmixed", Resource: "sales-order", Name: "订单", NewModule: true, Fields: allSearchModes(mixedTable())}},
		{minimalTable(), Spec{Module: "zzcgnote", Resource: "note", Name: "笔记", NewModule: true}},
		{ticketTable(), Spec{Module: "zzcgticket", Resource: "ticket", Name: "工单", NewModule: true}},
		{productTable(), Spec{Module: "zzcgproduct", Resource: "product", Name: "商品", NewModule: true}},
		{dealTable(), Spec{Module: "zzcgdeal", Resource: "deal", Name: "商机", NewModule: true,
			Fields: []FieldOption{{Column: "customer_id", Search: SearchEq}}}},
	}
	for _, c := range keyShapes("zzcg") {
		cases = append(cases, struct {
			table Table
			spec  Spec
		}{c.table, c.spec})
	}
	var written []File
	t.Cleanup(func() {
		for _, c := range cases {
			_ = os.RemoveAll(filepath.Join(web, "src", "views", c.spec.Module))
			_ = os.RemoveAll(filepath.Join(web, "src", "api", c.spec.Module))
		}
	})
	for _, c := range cases {
		res, err := Generate(c.table, c.spec)
		require.NoError(t, err)
		for _, f := range res.Files {
			if !strings.HasPrefix(f.Path, "web/") {
				continue
			}
			target := filepath.Join(root, filepath.FromSlash(f.Path))
			require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o750))
			require.NoError(t, os.WriteFile(target, []byte(f.Content), 0o600))
			written = append(written, f)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	require.Empty(t, FormatWeb(ctx, root, written))

	var dirs []string
	for _, c := range cases {
		dirs = append(dirs, "src/views/"+c.spec.Module, "src/api/"+c.spec.Module)
	}
	run := func(name string, args ...string) {
		cmd := exec.CommandContext(ctx, filepath.Join(web, "node_modules", ".bin", name), args...)
		cmd.Dir = web
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s on generated code:\n%s", name, out)
	}
	run("eslint", append([]string{"--max-warnings", "0"}, dirs...)...)
	run("vue-tsc", "--noEmit", "--skipLibCheck")
}
