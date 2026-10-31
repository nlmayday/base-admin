package codegen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// modulesFile is where business modules are registered.
const modulesFile = "server/internal/app/modules.go"

// moduleMarker marks the line in modulesFile above which Wire inserts a
// new module; the import goes after the last module import.
const moduleMarker = "// codegen:modules"

// goModulePath is the Go module path; scripts/new-project.sh rewrites it.
const goModulePath = "base-admin/server"

// ErrNoMarkers is returned when modulesFile lacks the codegen markers.
var ErrNoMarkers = errors.New("server/internal/app/modules.go 中缺少 codegen 标记，请手动接入模块")

// Wire registers module pkg in modulesFile under root: an import and a
// pkg.New(pkg.Deps{DB: d.db, Dict: systemModule}) entry (the system module
// checks dictionary-backed fields), inserted above the codegen marker.
// It reports false when the module is already registered.
func Wire(root, pkg string) (bool, error) {
	path := filepath.Join(root, filepath.FromSlash(modulesFile))
	src, err := os.ReadFile(path) //nolint:gosec // G304: fixed path under the repository root
	if err != nil {
		return false, err
	}
	importLine := fmt.Sprintf("%q", goModulePath+"/internal/modules/"+pkg)
	if bytes.Contains(src, []byte(importLine)) {
		return false, nil
	}
	out, err := insertImport(string(src), importLine)
	if err != nil {
		return false, err
	}
	if out, err = insertAbove(out, moduleMarker, fmt.Sprintf("%s.New(%s.Deps{DB: d.db, Dict: systemModule}),", pkg, pkg)); err != nil {
		return false, err
	}
	formatted, err := format.Source([]byte(out))
	if err != nil {
		return false, fmt.Errorf("format %s: %w", modulesFile, err)
	}
	// Repository source file, world-readable like the rest.
	return true, os.WriteFile(path, formatted, 0o644) //nolint:gosec // G306: source file
}

// WireResource registers a resource generated into an existing module: its
// permissions, menus and routes are added above the codegen markers of the
// module's module.go. It reports false when the resource is already there,
// and ErrNoMarkers for a module.go without markers (written by hand).
func WireResource(root string, spec Spec) (bool, error) {
	path := filepath.Join(root, "server", "internal", "modules", spec.Module, "module.go")
	src, err := os.ReadFile(path) //nolint:gosec // G304: generated path under the repository root
	if err != nil {
		return false, err
	}
	lower := goUnexported(spec.Resource)
	if bytes.Contains(src, []byte("m."+lower+"Routes(r)")) {
		return false, nil
	}
	out := string(src)
	for _, ins := range []struct{ marker, line string }{
		{"// codegen:permissions", lower + "Permissions,"},
		{"// codegen:menus", lower + "Menus,"},
		{"// codegen:routes", "m." + lower + "Routes(r)"},
	} {
		if out, err = insertAbove(out, ins.marker, ins.line); err != nil {
			return false, err
		}
	}
	formatted, err := format.Source([]byte(out))
	if err != nil {
		return false, fmt.Errorf("format %s: %w", path, err)
	}
	return true, os.WriteFile(path, formatted, 0o644) //nolint:gosec // G306: source file
}

// insertImport adds an import line after the last import of a module
// package; gofmt then sorts the group.
func insertImport(src, importLine string) (string, error) {
	prefix := "\t\"" + goModulePath + "/internal/modules/"
	i := strings.LastIndex(src, prefix)
	if i < 0 {
		return "", ErrNoMarkers
	}
	end := i + strings.IndexByte(src[i:], '\n') + 1
	return src[:end] + "\t" + importLine + "\n" + src[end:], nil
}

// insertAbove inserts line above the line holding marker, with the same
// indentation.
func insertAbove(src, marker, line string) (string, error) {
	i := strings.Index(src, marker)
	if i < 0 {
		return "", ErrNoMarkers
	}
	start := strings.LastIndexByte(src[:i], '\n') + 1
	indent := src[start:i]
	return src[:start] + indent + line + "\n" + src[start:], nil
}

// FormatWeb runs the repository's prettier on the generated frontend files
// so they match the project style. It is best effort: a returned note says
// what to run by hand.
func FormatWeb(ctx context.Context, root string, files []File) (note string) {
	webDir := filepath.Join(root, "web")
	bin := filepath.Join(webDir, "node_modules", ".bin", "prettier")
	var targets []string
	for _, f := range files {
		if rel, ok := strings.CutPrefix(f.Path, "web/"); ok {
			targets = append(targets, rel)
		}
	}
	if len(targets) == 0 {
		return ""
	}
	if _, err := os.Stat(bin); err != nil {
		return "未找到 prettier，请在 web 目录运行 pnpm exec prettier --write 格式化生成的前端文件"
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, append([]string{"--write", "--log-level", "warn"}, targets...)...) //nolint:gosec // G204: fixed binary, generated paths
	cmd.Dir = webDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Sprintf("prettier 格式化失败（%v）：%s", err, out)
	}
	return ""
}
