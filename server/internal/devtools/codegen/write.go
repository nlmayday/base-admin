package codegen

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrExists is returned when generated files already exist and force is off.
var ErrExists = errors.New("files already exist")

// FindRepoRoot walks up from start to the directory holding server/go.mod
// and web/package.json.
func FindRepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if isFile(filepath.Join(dir, "server", "go.mod")) && isFile(filepath.Join(dir, "web", "package.json")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repository root not found above %s (expected server/go.mod and web/package.json)", start)
		}
		dir = parent
	}
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// ModuleExists reports whether server/internal/modules/<name> exists.
func ModuleExists(root, name string) bool {
	st, err := os.Stat(filepath.Join(root, "server", "internal", "modules", name))
	return err == nil && st.IsDir()
}

// Existing returns the generated paths that already exist under root.
func Existing(root string, files []File) ([]string, error) {
	var out []string
	for _, f := range files {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(f.Path)))
		switch {
		case err == nil:
			out = append(out, f.Path)
		case !errors.Is(err, fs.ErrNotExist):
			return nil, err
		}
	}
	return out, nil
}

// Write writes files under root. Without force it refuses, before writing
// anything, when any target exists. When a write fails, the files already
// written are restored (or removed when they were new).
func Write(root string, files []File, force bool) (err error) {
	if !force {
		existing, err := Existing(root, files)
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			return fmt.Errorf("%w (use -force to overwrite): %s", ErrExists, strings.Join(existing, ", "))
		}
	}
	var undo []func()
	defer func() {
		if err != nil {
			for i := len(undo) - 1; i >= 0; i-- {
				undo[i]()
			}
		}
	}()
	for _, f := range files {
		target := filepath.Join(root, filepath.FromSlash(f.Path))
		if !strings.HasPrefix(target, filepath.Clean(root)+string(filepath.Separator)) {
			return fmt.Errorf("refusing to write outside the repository: %s", f.Path)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		old, readErr := os.ReadFile(target) //nolint:gosec // G304: target is checked to be inside root
		switch {
		case readErr == nil:
			undo = append(undo, func() { _ = os.WriteFile(target, old, 0o644) }) //nolint:gosec // G306: source files
		case errors.Is(readErr, fs.ErrNotExist):
			undo = append(undo, func() { _ = os.Remove(target) })
		default:
			return readErr
		}
		// Generated sources are ordinary, world-readable repository files.
		if err := os.WriteFile(target, []byte(f.Content), 0o644); err != nil { //nolint:gosec // G306: source files
			return err
		}
	}
	return nil
}
