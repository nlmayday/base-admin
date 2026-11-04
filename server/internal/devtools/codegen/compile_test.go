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

// TestGeneratedGoCompiles writes the Go output for several table shapes into
// a temporary package inside this module (under testdata, which ./...
// ignores) and runs go vet on it, including the integration test file.
func TestGeneratedGoCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the go tool")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	require.NoError(t, os.MkdirAll("testdata", 0o750))
	dir, err := os.MkdirTemp("testdata", "compile-")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
		_ = os.Remove("testdata") // only succeeds when empty
	})

	cases := []struct {
		table Table
		spec  Spec
	}{
		{customerTable(), customerSpec()},
		{mixedTable(), Spec{Module: "shop", Resource: "sales-order", Name: "订单", NewModule: true}},
		{minimalTable(), Spec{Module: "notes", Resource: "note", Name: "笔记", NewModule: true}},
		// Every search mode a column supports, the last one listed (range
		// where possible), plus label, list and form overrides.
		{mixedTable(), Spec{Module: "shopall", Resource: "sales-order", Name: "订单", NewModule: true, Fields: allSearchModes(mixedTable())}},
		{customerTable(), Spec{Module: "crmall", Resource: "customer", Name: "客户", NewModule: true, Fields: allSearchModes(customerTable())}},
		// Options written in column comments (text and integer, nullable).
		{ticketTable(), Spec{Module: "desk", Resource: "ticket", Name: "工单", NewModule: true}},
		// Image and attachment uploads.
		{productTable(), Spec{Module: "shopup", Resource: "product", Name: "商品", NewModule: true}},
		// A database-assigned key narrower than bigint.
		{Table{Name: "biz_tags", Columns: []Column{{Name: "id", UDT: "int4", Identity: true, Primary: true}, {Name: "name", UDT: "varchar", MaxLength: 32}}},
			Spec{Module: "tags", Resource: "tag", Name: "标签", NewModule: true}},
		// Relation fields, searched and not.
		{dealTable(), Spec{Module: "crmrel", Resource: "deal", Name: "商机", NewModule: true,
			Fields: []FieldOption{{Column: "customer_id", Search: SearchEq}}}},
	}
	for _, c := range keyShapes("") {
		cases = append(cases, struct {
			table Table
			spec  Spec
		}{c.table, c.spec})
	}
	var pkgs []string
	for _, c := range cases {
		res, err := Generate(c.table, c.spec)
		require.NoError(t, err)
		for _, f := range res.Files {
			if !strings.HasSuffix(f.Path, ".go") {
				continue
			}
			target := filepath.Join(dir, c.spec.Module, filepath.Base(f.Path))
			require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o750))
			require.NoError(t, os.WriteFile(target, []byte(f.Content), 0o600))
		}
		pkgs = append(pkgs, "./"+filepath.ToSlash(filepath.Join(dir, c.spec.Module)))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	args := append([]string{"vet", "-tags", "integration"}, pkgs...)
	cmd := exec.CommandContext(ctx, goBin, args...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "go vet on generated code:\n%s", out)
}

// allSearchModes picks, for every column, its last search mode and hides
// optional columns from the list and form.
func allSearchModes(t Table) []FieldOption {
	infos, err := Describe(t)
	if err != nil {
		panic(err)
	}
	hide := false
	out := make([]FieldOption, 0, len(infos))
	for _, f := range infos {
		o := FieldOption{Column: f.Column, Label: f.Label + "名", Search: f.SearchModes[len(f.SearchModes)-1], List: &hide}
		if f.DictAllowed {
			o.Dict, o.Search = ptr("sample_dict"), SearchEq
		}
		if !f.FormLocked {
			o.Form = &hide
		}
		out = append(out, o)
	}
	return out
}
