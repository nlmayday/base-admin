// Command gen generates a CRUD module (Go backend and Vue frontend) from a
// PostgreSQL or MySQL table (docs/p1-p3-plan.md, devtools).
//
//	go run ./cmd/gen -table biz_orders -module sales -resource order -name 订单
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"base-admin/server/internal/devtools/codegen"
	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/database"
)

const usage = `usage: go run ./cmd/gen -table <table> -module <module> -resource <resource> -name <中文名> [flags]
       go run ./cmd/gen -spec <specs.json> [flags]

-spec reads one spec or a list of them, each with the fields of the codegen
page ("table", "module", "resource", "name", "moduleTitle", "parentMenu", "menuTitle", "menuSort",
"fields", "keyMode", "readOnly", "softDelete", "children"), and generates them in order:
the way to generate many tables, and to generate them again with -force.

Generates server/internal/modules/<module>/<resource>.go (plus module.go and
common.go for a new module), an integration test skeleton, and the frontend
API wrapper, list page and form dialog. Existing files are never overwritten
without -force.

flags:
`

// options are the parsed command-line flags.
type options struct {
	dsn, root        string
	jobs             []job
	force, dryRun    bool
	noFormat, noWire bool
}

// job is one table to generate.
type job struct {
	table string
	spec  codegen.Spec
}

// specEntry is one spec of a -spec file, with the JSON names of the codegen
// page's request.
type specEntry struct {
	Table       string                `json:"table"`
	Module      string                `json:"module"`
	Resource    string                `json:"resource"`
	Name        string                `json:"name"`
	ModuleTitle string                `json:"moduleTitle,omitempty"`
	ParentMenu  string                `json:"parentMenu,omitempty"`
	MenuTitle   string                `json:"menuTitle,omitempty"`
	MenuSort    *int                  `json:"menuSort,omitempty"`
	Fields      []codegen.FieldOption `json:"fields,omitempty"`
	KeyMode     string                `json:"keyMode,omitempty"`
	ReadOnly    bool                  `json:"readOnly,omitempty"`
	SoftDelete  *bool                 `json:"softDelete,omitempty"`
	Children    []codegen.ChildSpec   `json:"children,omitempty"`
	OrderBy     string                `json:"orderBy,omitempty"`
	NoCreate    bool                  `json:"noCreate,omitempty"`
	NoDelete    bool                  `json:"noDelete,omitempty"`
	// DeleteConfirm is the delete confirmation, {column} for the row's value.
	DeleteConfirm string `json:"deleteConfirm,omitempty"`
	// CountCap bounds the list's count on a large table (0: count every row).
	CountCap int `json:"countCap,omitempty"`
}

func (e specEntry) job() job {
	return job{table: e.Table, spec: codegen.Spec{Module: e.Module, Resource: e.Resource, Name: e.Name,
		ModuleTitle: e.ModuleTitle, ParentMenu: e.ParentMenu, MenuTitle: e.MenuTitle, MenuSort: e.MenuSort,
		Fields: e.Fields, KeyMode: e.KeyMode,
		ReadOnly: e.ReadOnly, SoftDelete: e.SoftDelete, Children: e.Children,
		OrderBy: e.OrderBy, NoCreate: e.NoCreate, NoDelete: e.NoDelete, DeleteConfirm: e.DeleteConfirm, CountCap: e.CountCap}}
}

// loader reads a table definition with the tables the spec refers to.
type loader func(ctx context.Context, dsn, table string, spec codegen.Spec) (codegen.Table, error)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, loadTable))
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var o options
	var one specEntry
	fs := flag.NewFlagSet("gen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}
	fs.StringVar(&one.Table, "table", "", "source table in the current schema (required without -spec)")
	fs.StringVar(&one.Module, "module", "", "module / Go package name, e.g. example (required without -spec)")
	fs.StringVar(&one.Resource, "resource", "", "kebab-case resource name, e.g. customer (required without -spec)")
	fs.StringVar(&one.Name, "name", "", "Chinese display name, e.g. 客户 (required without -spec)")
	fs.StringVar(&one.ModuleTitle, "module-title", "", "directory menu title when the module is new (default: module name)")
	fs.StringVar(&one.ParentMenu, "parent-menu", "", "parent menu code (default: <module>.root)")
	fs.StringVar(&one.MenuTitle, "menu-title", "", "the page's menu title (default: <name>管理)")
	fs.Func("menu-sort", "the page's menu order among its siblings, smaller first (default 1)", func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("menu-sort: %w", err)
		}
		one.MenuSort = &n
		return nil
	})
	fs.StringVar(&one.KeyMode, "key-mode", "", "how rows get their primary key: auto, generated (snowflake, Deps.NewID) or input (default: from the column)")
	fs.BoolVar(&one.ReadOnly, "read-only", false, "generate the list, detail and export only")
	fs.StringVar(&one.OrderBy, "order-by", "", `list and export order, e.g. "created_at DESC, id DESC" or "status = 0 DESC, id" (default: sort, then the key descending)`)
	fs.StringVar(&one.DeleteConfirm, "delete-confirm", "", `delete confirmation, {column} for the row's value, e.g. '删除分类「{code}」？'`)
	fs.IntVar(&one.CountCap, "count-cap", 0, "count a large table's list only up to this many rows, e.g. 10000 (default 0: count every row)")
	fs.BoolVar(&one.NoCreate, "no-create", false, "no create (route, permission, button)")
	fs.BoolVar(&one.NoDelete, "no-delete", false, "no delete (route, permission, button), e.g. rows are disabled instead")
	fs.BoolFunc("no-soft-delete", "treat deleted_at as a plain time column: list every row, delete removes the row", func(string) error {
		off := false
		one.SoftDelete = &off
		return nil
	})
	specFile := fs.String("spec", "", "JSON file with one spec or a list of specs (see above)")
	children := fs.String("children", "", `child tables as JSON, e.g. '[{"table":"lg_item_i18n","foreignKey":"item_id","keyColumn":"lang","keyDict":"lang"}]'`)
	fs.StringVar(&o.root, "root", "", "repository root (default: found from the working directory)")
	fs.StringVar(&o.dsn, "dsn", os.Getenv("ADMIN_DATABASE_URL"), "database URL, postgres:// or mysql:// (default: $ADMIN_DATABASE_URL)")
	fs.BoolVar(&o.force, "force", false, "overwrite existing files")
	fs.BoolVar(&o.dryRun, "dry-run", false, "print the files instead of writing them")
	fs.BoolVar(&o.noFormat, "no-format", false, "do not run prettier (frontend) and gofmt -s (Go) on the generated files")
	fs.BoolVar(&o.noWire, "no-wire", false, "do not register a new module in server/internal/app/modules.go")
	fields := fs.String("fields", "", `field options as JSON, e.g. '[{"column":"order_no","label":"订单号","search":"like"}]' (copy it from 开发工具 → 代码生成)`)
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	for flagName, raw := range map[string]struct {
		text string
		dst  any
	}{"-fields": {*fields, &one.Fields}, "-children": {*children, &one.Children}} {
		if raw.text == "" {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(raw.text))
		dec.DisallowUnknownFields()
		if err := dec.Decode(raw.dst); err != nil {
			return o, fmt.Errorf("invalid %s JSON: %w", flagName, err)
		}
	}
	if *specFile != "" {
		if one.Table != "" || one.Module != "" {
			return o, errors.New("-spec replaces -table, -module, -resource and -name")
		}
		entries, err := readSpecs(*specFile)
		if err != nil {
			return o, err
		}
		for _, e := range entries {
			o.jobs = append(o.jobs, e.job())
		}
	} else {
		var missing []string
		for name, v := range map[string]string{"-table": one.Table, "-module": one.Module, "-resource": one.Resource, "-name": one.Name} {
			if v == "" {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			return o, fmt.Errorf("missing required flags: %s", strings.Join(slices.Sorted(slices.Values(missing)), ", "))
		}
		o.jobs = []job{one.job()}
	}
	if o.dsn == "" {
		return o, errors.New("no database: set ADMIN_DATABASE_URL or pass -dsn")
	}
	return o, nil
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, load loader) int {
	o, err := parseFlags(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "gen:", err)
		return 2
	}
	if err := generate(ctx, o, stdout, load); err != nil {
		_, _ = fmt.Fprintln(stderr, "gen:", err)
		return 1
	}
	return 0
}

// readSpecs reads a -spec file: one spec or a list of them.
func readSpecs(path string) ([]specEntry, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // G304: a file the developer names
	if err != nil {
		return nil, fmt.Errorf("read -spec: %w", err)
	}
	decode := func(dst any) error {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		return dec.Decode(dst)
	}
	var list []specEntry
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		err = decode(&list)
	} else {
		var one specEntry
		err = decode(&one)
		list = []specEntry{one}
	}
	if err != nil {
		return nil, fmt.Errorf("invalid -spec JSON: %w", err)
	}
	if len(list) == 0 {
		return nil, errors.New("-spec lists no table")
	}
	for i, e := range list {
		if e.Table == "" || e.Module == "" || e.Resource == "" || e.Name == "" {
			return nil, fmt.Errorf("-spec entry %d: table, module, resource and name are required", i+1)
		}
	}
	return list, nil
}

func generate(ctx context.Context, o options, stdout io.Writer, load loader) error {
	root := o.root
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		if root, err = codegen.FindRepoRoot(wd); err != nil {
			return err
		}
	}
	for i, j := range o.jobs {
		if len(o.jobs) > 1 {
			_, _ = fmt.Fprintf(stdout, "\n== %d/%d %s → %s/%s\n", i+1, len(o.jobs), j.table, j.spec.Module, j.spec.Resource)
		}
		if err := generateOne(ctx, o, j, root, stdout, load); err != nil {
			return fmt.Errorf("%s: %w", j.table, err)
		}
	}
	return nil
}

func generateOne(ctx context.Context, o options, j job, root string, stdout io.Writer, load loader) error {
	table, err := load(ctx, o.dsn, j.table, j.spec)
	if err != nil {
		return err
	}
	j.spec.NewModule = !codegen.ModuleExists(root, j.spec.Module)
	res, err := codegen.Generate(table, j.spec)
	if err != nil {
		return err
	}
	if o.dryRun {
		printFiles(stdout, res)
		printSummary(stdout, res, j.spec, false)
		return nil
	}
	kept, err := codegen.Kept(root, res.Files)
	if err != nil {
		return err
	}
	if err := codegen.Write(root, res.Files, o.force); err != nil {
		return err
	}
	written := make([]codegen.File, 0, len(res.Files))
	for _, f := range res.Files {
		if slices.Contains(kept, f.Path) {
			_, _ = fmt.Fprintln(stdout, "kept", f.Path, "(extension points, never overwritten)")
			continue
		}
		written = append(written, f)
		_, _ = fmt.Fprintln(stdout, "wrote", f.Path)
	}
	if !o.noFormat {
		if note := codegen.FormatWeb(ctx, root, written); note != "" {
			_, _ = fmt.Fprintln(stdout, "note:", note)
		}
		if note := codegen.FormatGo(ctx, root, written); note != "" {
			_, _ = fmt.Fprintln(stdout, "note:", note)
		}
	}
	wired := false
	switch {
	case o.noWire:
	case j.spec.NewModule:
		if wired, err = codegen.Wire(root, j.spec.Module); err != nil {
			_, _ = fmt.Fprintln(stdout, "note: 模块未能自动接入：", err)
		} else if wired {
			_, _ = fmt.Fprintln(stdout, "registered the module in server/internal/app/modules.go")
		}
	default:
		if wired, err = codegen.WireResource(root, j.spec); err != nil {
			_, _ = fmt.Fprintln(stdout, "note: 新资源未能自动注册：", err, "；", res.ManualWiring)
		} else if wired {
			_, _ = fmt.Fprintf(stdout, "registered the resource in server/internal/modules/%s/module.go\n", j.spec.Module)
		}
	}
	printSummary(stdout, res, j.spec, wired)
	return nil
}

func printFiles(w io.Writer, res codegen.Result) {
	for _, f := range res.Files {
		_, _ = fmt.Fprintf(w, "==> %s\n%s\n", f.Path, f.Content)
	}
}

func printSummary(w io.Writer, res codegen.Result, spec codegen.Spec, wired bool) {
	_, _ = fmt.Fprintln(w, "\nPermission codes:")
	for _, p := range res.Permissions {
		_, _ = fmt.Fprintf(w, "  %-36s %s\n", p.Code, p.Name)
	}
	_, _ = fmt.Fprintln(w, "\nMenus:")
	for _, m := range res.Menus {
		_, _ = fmt.Fprintf(w, "  %-24s %-4s %-10s %s %s\n", m.Code, m.Type, m.Title, m.Path, m.Component)
	}
	if spec.NewModule && !wired {
		_, _ = fmt.Fprintf(w, "\nWire the new module in server/internal/app/modules.go:\n%s\n", indent(res.Wiring))
	}
	if len(res.Notes) > 0 {
		_, _ = fmt.Fprintln(w, "\nNotes:")
		for _, n := range res.Notes {
			_, _ = fmt.Fprintln(w, "  -", n)
		}
	}
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

// loadTable reads the table definition, and the tables the spec refers to,
// from the database.
func loadTable(ctx context.Context, dsn, table string, spec codegen.Spec) (codegen.Table, error) {
	cfg := config.DatabaseConfig{
		URL: dsn, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute, ConnMaxIdleTime: time.Minute,
		ConnectTimeout: 5 * time.Second, StatementTimeout: 30 * time.Second, SlowQueryThreshold: time.Second,
	}
	sqlDB, err := database.OpenSQL(ctx, cfg, "base-admin-gen")
	if err != nil {
		return codegen.Table{}, err
	}
	defer func() { _ = sqlDB.Close() }()
	db, err := database.NewGorm(sqlDB, cfg, zap.NewNop())
	if err != nil {
		return codegen.Table{}, err
	}
	table2, err := codegen.Load(ctx, db, table, spec)
	if errors.Is(err, codegen.ErrSpec) {
		return table2, errors.New(codegen.SpecMessage(err))
	}
	return table2, err
}
