// Command gen generates a CRUD module (Go backend and Vue frontend) from a
// PostgreSQL table (docs/p1-p3-plan.md, devtools).
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
	"strings"
	"time"

	"go.uber.org/zap"

	"base-admin/server/internal/devtools/codegen"
	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/database"
)

const usage = `usage: go run ./cmd/gen -table <table> -module <module> -resource <resource> -name <中文名> [flags]

Generates server/internal/modules/<module>/<resource>.go (plus module.go and
common.go for a new module), an integration test skeleton, and the frontend
API wrapper, list page and form dialog. Existing files are never overwritten
without -force.

flags:
`

// options are the parsed command-line flags.
type options struct {
	table, dsn, root string
	spec             codegen.Spec
	force, dryRun    bool
	noFormat, noWire bool
}

// loader reads a table definition.
type loader func(ctx context.Context, dsn, table string) (codegen.Table, error)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, loadTable))
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("gen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}
	fs.StringVar(&o.table, "table", "", "source table in the current schema (required)")
	fs.StringVar(&o.spec.Module, "module", "", "module / Go package name, e.g. example (required)")
	fs.StringVar(&o.spec.Resource, "resource", "", "kebab-case resource name, e.g. customer (required)")
	fs.StringVar(&o.spec.Name, "name", "", "Chinese display name, e.g. 客户 (required)")
	fs.StringVar(&o.spec.ModuleTitle, "module-title", "", "directory menu title when the module is new (default: module name)")
	fs.StringVar(&o.spec.ParentMenu, "parent-menu", "", "parent menu code (default: <module>.root)")
	fs.StringVar(&o.root, "root", "", "repository root (default: found from the working directory)")
	fs.StringVar(&o.dsn, "dsn", os.Getenv("ADMIN_DATABASE_URL"), "PostgreSQL URL (default: $ADMIN_DATABASE_URL)")
	fs.BoolVar(&o.force, "force", false, "overwrite existing files")
	fs.BoolVar(&o.dryRun, "dry-run", false, "print the files instead of writing them")
	fs.BoolVar(&o.noFormat, "no-format", false, "do not run prettier on the generated frontend files")
	fs.BoolVar(&o.noWire, "no-wire", false, "do not register a new module in server/internal/app/modules.go")
	fields := fs.String("fields", "", `field options as JSON, e.g. '[{"column":"order_no","label":"订单号","search":"like"}]' (copy it from 开发工具 → 代码生成)`)
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if *fields != "" {
		dec := json.NewDecoder(strings.NewReader(*fields))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&o.spec.Fields); err != nil {
			return o, fmt.Errorf("invalid -fields JSON: %w", err)
		}
	}
	var missing []string
	for name, v := range map[string]string{"-table": o.table, "-module": o.spec.Module, "-resource": o.spec.Resource, "-name": o.spec.Name} {
		if v == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return o, fmt.Errorf("missing required flags: %s", strings.Join(slices.Sorted(slices.Values(missing)), ", "))
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
	table, err := load(ctx, o.dsn, o.table)
	if err != nil {
		return err
	}
	o.spec.NewModule = !codegen.ModuleExists(root, o.spec.Module)
	res, err := codegen.Generate(table, o.spec)
	if err != nil {
		return err
	}
	if o.dryRun {
		printFiles(stdout, res)
		printSummary(stdout, res, o.spec, false)
		return nil
	}
	if err := codegen.Write(root, res.Files, o.force); err != nil {
		return err
	}
	for _, f := range res.Files {
		_, _ = fmt.Fprintln(stdout, "wrote", f.Path)
	}
	if !o.noFormat {
		if note := codegen.FormatWeb(ctx, root, res.Files); note != "" {
			_, _ = fmt.Fprintln(stdout, "note:", note)
		}
	}
	wired := false
	switch {
	case o.noWire:
	case o.spec.NewModule:
		if wired, err = codegen.Wire(root, o.spec.Module); err != nil {
			_, _ = fmt.Fprintln(stdout, "note: 模块未能自动接入：", err)
		} else if wired {
			_, _ = fmt.Fprintln(stdout, "registered the module in server/internal/app/modules.go")
		}
	default:
		if wired, err = codegen.WireResource(root, o.spec); err != nil {
			_, _ = fmt.Fprintln(stdout, "note: 新资源未能自动注册：", err, "；", res.ManualWiring)
		} else if wired {
			_, _ = fmt.Fprintf(stdout, "registered the resource in server/internal/modules/%s/module.go\n", o.spec.Module)
		}
	}
	printSummary(stdout, res, o.spec, wired)
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

// loadTable reads the table definition from PostgreSQL.
func loadTable(ctx context.Context, dsn, table string) (codegen.Table, error) {
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
	return codegen.Introspect(ctx, db, table)
}
