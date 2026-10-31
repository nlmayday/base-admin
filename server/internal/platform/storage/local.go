package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
)

const (
	dirPerm  = 0o750
	filePerm = 0o640
	// tmpPrefix marks partial writes; such files are never valid keys
	// because ValidateKey rejects a leading dot.
	tmpPrefix = ".tmp-"
)

// Local stores objects in a directory. All access goes through os.Root, so
// neither ".." nor symlinks can escape the base directory.
type Local struct {
	root *os.Root
	dir  string
}

var _ Storage = (*Local)(nil)

// NewLocal opens (creating if needed) the base directory dir.
func NewLocal(dir string) (*Local, error) {
	if dir == "" {
		return nil, errors.New("storage: local directory is empty")
	}
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return nil, fmt.Errorf("storage: create %s: %w", dir, err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("storage: open %s: %w", dir, err)
	}
	return &Local{root: root, dir: dir}, nil
}

// Name implements Storage.
func (l *Local) Name() string { return "local" }

// Dir returns the base directory.
func (l *Local) Dir() string { return l.dir }

// Close releases the directory handle.
func (l *Local) Close() error { return l.root.Close() }

// Put writes to a temporary file in the target directory, fsyncs it, then
// renames it into place and fsyncs the directory, so a crash never leaves
// a partially written object under key.
func (l *Local) Put(ctx context.Context, key string, r io.Reader, size int64, _ string) (err error) {
	if err := ValidateKey(key); err != nil {
		return err
	}
	if _, statErr := l.root.Lstat(key); statErr == nil {
		return fmt.Errorf("%w: %s", ErrExists, key)
	}
	dir := path.Dir(key)
	if err := l.root.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("storage: mkdir: %w", err)
	}
	tmp := path.Join(dir, tmpPrefix+randomSuffix())
	f, err := l.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm)
	if err != nil {
		return fmt.Errorf("storage: create temp: %w", err)
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = l.root.Remove(tmp)
		}
	}()
	n, err := io.Copy(f, ctxReader{ctx: ctx, r: r})
	if err != nil {
		return fmt.Errorf("storage: write: %w", err)
	}
	if size >= 0 && n != size {
		return fmt.Errorf("storage: wrote %d bytes, expected %d", n, size)
	}
	if err = f.Sync(); err != nil {
		return fmt.Errorf("storage: sync: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("storage: close: %w", err)
	}
	if err = l.root.Rename(tmp, key); err != nil {
		return fmt.Errorf("storage: rename: %w", err)
	}
	return l.syncDir(dir)
}

func (l *Local) syncDir(dir string) error {
	d, err := l.root.Open(dir)
	if err != nil {
		return fmt.Errorf("storage: open dir: %w", err)
	}
	defer func() { _ = d.Close() }()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("storage: sync dir: %w", err)
	}
	return nil
}

// Open implements Storage. The returned *os.File is an io.ReadSeeker.
func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, Object, error) {
	if err := ValidateKey(key); err != nil {
		return nil, Object{}, err
	}
	f, err := l.root.Open(key)
	if err != nil {
		return nil, Object{}, mapFSError(err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, Object{}, fmt.Errorf("storage: stat: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, Object{}, ErrNotFound
	}
	return f, Object{Key: key, Size: info.Size(), ModTime: info.ModTime()}, nil
}

// Stat implements Storage.
func (l *Local) Stat(_ context.Context, key string) (Object, error) {
	if err := ValidateKey(key); err != nil {
		return Object{}, err
	}
	info, err := l.root.Stat(key)
	if err != nil {
		return Object{}, mapFSError(err)
	}
	if !info.Mode().IsRegular() {
		return Object{}, ErrNotFound
	}
	return Object{Key: key, Size: info.Size(), ModTime: info.ModTime()}, nil
}

// Delete implements Storage.
func (l *Local) Delete(_ context.Context, key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	if err := l.root.Remove(key); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("storage: delete: %w", err)
	}
	return nil
}

func mapFSError(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotFound
	}
	return fmt.Errorf("storage: %w", err)
}

func randomSuffix() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ctxReader stops a long copy when the request context ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
