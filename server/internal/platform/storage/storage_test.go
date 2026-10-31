package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/config"
)

func TestNewKey(t *testing.T) {
	now := time.Date(2026, 3, 7, 23, 0, 0, 0, time.FixedZone("x", 8*3600))
	key, err := NewKey(now, "png")
	require.NoError(t, err)
	assert.Regexp(t, regexp.MustCompile(`^2026/03/07/[0-9a-f]{32}\.png$`), key)
	require.NoError(t, ValidateKey(key))

	other, err := NewKey(now, "png")
	require.NoError(t, err)
	assert.NotEqual(t, key, other)

	for _, ext := range []string{"", "PNG", "p/ng", "../x", "toolongextension1", ".png"} {
		_, err := NewKey(now, ext)
		assert.ErrorIs(t, err, ErrInvalidKey, ext)
	}
}

func TestValidateKey(t *testing.T) {
	good := []string{"2026/01/02/abc.png", "a", "a/b-c_d.e"}
	for _, k := range good {
		assert.NoError(t, ValidateKey(k), k)
	}
	bad := []string{
		"", "/etc/passwd", "../x", "a/../b", "a/./b", "a//b", "a/", `a\b`, "..", ".",
		"a/..b", ".hidden", "a/.tmp-1", "a b", "a\x00b", "C:/x", "a/b?c", strings.Repeat("a", 201),
	}
	for _, k := range bad {
		assert.ErrorIs(t, ValidateKey(k), ErrInvalidKey, "%q", k)
	}
}

func TestNewSelectsDriver(t *testing.T) {
	s, err := New(config.FileConfig{Driver: config.FileDriverLocal, LocalDir: t.TempDir()})
	require.NoError(t, err)
	assert.Equal(t, "local", s.Name())

	s, err = New(config.FileConfig{Driver: config.FileDriverS3, S3: config.S3Config{Endpoint: "127.0.0.1:9000", Bucket: "b"}})
	require.NoError(t, err)
	assert.Equal(t, "s3", s.Name())

	_, err = New(config.FileConfig{Driver: "ftp"})
	assert.Error(t, err)
	_, err = New(config.FileConfig{Driver: config.FileDriverS3})
	assert.Error(t, err)
	_, err = NewLocal("")
	assert.Error(t, err)
}

// driverContract runs the behaviour every driver must share.
func driverContract(t *testing.T, s Storage) {
	ctx := context.Background()
	key := "2026/01/02/abc.txt"

	_, err := s.Stat(ctx, key)
	assert.ErrorIs(t, err, ErrNotFound)
	_, _, err = s.Open(ctx, key)
	assert.ErrorIs(t, err, ErrNotFound)

	require.NoError(t, s.Put(ctx, key, strings.NewReader("hello world"), -1, "text/plain"))
	obj, err := s.Stat(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, int64(11), obj.Size)

	rc, obj, err := s.Open(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, int64(11), obj.Size)
	seeker, ok := rc.(io.ReadSeeker)
	require.True(t, ok, "drivers must return a seekable reader")
	_, err = seeker.Seek(6, io.SeekStart)
	require.NoError(t, err)
	rest, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "world", string(rest))
	require.NoError(t, rc.Close())

	require.NoError(t, s.Delete(ctx, key))
	require.NoError(t, s.Delete(ctx, key), "delete must be idempotent")
	_, err = s.Stat(ctx, key)
	assert.ErrorIs(t, err, ErrNotFound)

	for _, bad := range []string{"../x", "/abs"} {
		assert.ErrorIs(t, s.Put(ctx, bad, strings.NewReader("x"), 1, ""), ErrInvalidKey)
		_, _, err := s.Open(ctx, bad)
		assert.ErrorIs(t, err, ErrInvalidKey)
		_, err = s.Stat(ctx, bad)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.ErrorIs(t, s.Delete(ctx, bad), ErrInvalidKey)
	}
}

func TestLocalContract(t *testing.T) {
	l, err := NewLocal(filepath.Join(t.TempDir(), "nested", "uploads"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	driverContract(t, l)
}

func TestS3Contract(t *testing.T) {
	s, _ := newFakeS3(t)
	assert.Equal(t, "bkt", s.Bucket())
	driverContract(t, s)
}

func TestLocalWritesFilesUnderBase(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLocal(dir)
	require.NoError(t, err)
	require.NoError(t, l.Put(context.Background(), "2026/01/02/a.bin", bytes.NewReader([]byte{1, 2, 3}), 3, ""))
	data, err := os.ReadFile(filepath.Join(dir, "2026", "01", "02", "a.bin"))
	require.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3}, data)
	assert.Equal(t, dir, l.Dir())

	err = l.Put(context.Background(), "2026/01/02/a.bin", strings.NewReader("x"), 1, "")
	assert.ErrorIs(t, err, ErrExists, "existing objects are never overwritten")
}

func TestLocalRefusesSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(base, "link")))
	l, err := NewLocal(base)
	require.NoError(t, err)
	ctx := context.Background()

	_, _, err = l.Open(ctx, "link/secret.txt")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrInvalidKey, "the key is syntactically valid; os.Root must stop it")
	_, err = l.Stat(ctx, "link/secret.txt")
	require.Error(t, err)
	require.Error(t, l.Put(ctx, "link/new.txt", strings.NewReader("x"), 1, ""))
	_, err = os.Stat(filepath.Join(outside, "new.txt"))
	assert.True(t, os.IsNotExist(err), "nothing may be written outside the base directory")
	require.Error(t, l.Delete(ctx, "link/secret.txt"))
	_, err = os.Stat(filepath.Join(outside, "secret.txt"))
	assert.NoError(t, err)
}

type failingReader struct{ n int }

func (f *failingReader) Read(p []byte) (int, error) {
	if f.n > 0 {
		f.n--
		p[0] = 'x'
		return 1, nil
	}
	return 0, errors.New("boom")
}

func TestLocalFailedPutLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLocal(dir)
	require.NoError(t, err)
	ctx := context.Background()
	key := "2026/01/02/f.txt"

	require.Error(t, l.Put(ctx, key, &failingReader{n: 3}, -1, ""))
	require.Error(t, l.Put(ctx, key, strings.NewReader("short"), 10, ""), "size mismatch must fail")
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, l.Put(canceled, key, strings.NewReader("x"), -1, ""), context.Canceled)

	entries, err := os.ReadDir(filepath.Join(dir, "2026", "01", "02"))
	require.NoError(t, err)
	assert.Empty(t, entries, "no temp files or partial objects may remain")
}

func TestLocalStatDirectoryIsNotFound(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "a", "b"), 0o750))
	l, err := NewLocal(dir)
	require.NoError(t, err)
	_, err = l.Stat(context.Background(), "a/b")
	assert.ErrorIs(t, err, ErrNotFound)
	_, _, err = l.Open(context.Background(), "a/b")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestS3LargeUploadUsesMultipart(t *testing.T) {
	s, fake := newFakeS3(t)
	data := bytes.Repeat([]byte("0123456789abcdef"), (s3SinglePutLimit+1024)/16)
	key := "2026/01/02/big.bin"
	require.NoError(t, s.Put(context.Background(), key, bytes.NewReader(data), -1, "application/octet-stream"))
	assert.Equal(t, data, fake.objects[key])
	assert.Empty(t, fake.uploads)
}

func TestS3FailedMultipartIsAborted(t *testing.T) {
	s, fake := newFakeS3(t)
	fake.failPart = true
	data := bytes.Repeat([]byte("x"), s3SinglePutLimit+1)
	err := s.Put(context.Background(), "2026/01/02/big.bin", bytes.NewReader(data), -1, "")
	require.Error(t, err)
	assert.NotContains(t, fake.objects, "2026/01/02/big.bin")
	assert.Equal(t, 1, fake.aborted)
}

func TestS3ReaderErrorFailsPut(t *testing.T) {
	s, fake := newFakeS3(t)
	require.Error(t, s.Put(context.Background(), "2026/01/02/x.txt", &failingReader{n: 2}, -1, ""))
	assert.Empty(t, fake.objects)
}

func TestS3OtherErrorsAreNotNotFound(t *testing.T) {
	s, _ := newFakeS3(t)
	s.bucket = "missing"
	err := s.Delete(context.Background(), "a.txt")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotFound, "a missing bucket is a configuration error")
}
