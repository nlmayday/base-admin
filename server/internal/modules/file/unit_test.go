package file

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/httpx"
)

func TestCheckContentMatrix(t *testing.T) {
	cases := []struct {
		ext  string
		data []byte
		ok   bool
		mime string
	}{
		{"png", pngBytes, true, "image/png"},
		{"jpg", jpegBytes, true, "image/jpeg"},
		{"jpeg", jpegBytes, true, "image/jpeg"},
		{"gif", gifBytes, true, "image/gif"},
		{"webp", webpBytes, true, "image/webp"},
		{"pdf", pdfBytes, true, "application/pdf"},
		{"docx", zipBytes(), true, "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"xlsx", zipBytes(), true, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{"zip", zipBytes(), true, "application/zip"},
		{"gz", gzipBytes(), true, "application/gzip"},
		{"doc", oleBytes, true, "application/msword"},
		{"csv", textBytes, true, "text/csv; charset=utf-8"},
		{"txt", []byte("\xef\xbb\xbfhello 世界"), true, "text/plain; charset=utf-8"},
		{"json", []byte(`{"a":1}`), true, "application/json"},
		{"mp3", mp3Bytes, true, "audio/mpeg"},
		{"dat", exeBytes, true, "application/octet-stream"},

		{"png", exeBytes, false, ""},
		{"png", jpegBytes, false, ""},
		{"jpg", pngBytes, false, ""},
		{"pdf", pngBytes, false, ""},
		{"docx", pdfBytes, false, ""},
		{"doc", exeBytes, false, ""},
		{"txt", htmlBytes, false, ""},
		{"csv", svgBytes, false, ""},
		{"md", pngBytes, false, ""},
		{"png", htmlBytes, false, ""},
		{"dat", htmlBytes, false, ""},
		{"dat", pdfBytes, false, ""},
		{"zip", exeBytes, false, ""},
	}
	for _, tc := range cases {
		mime, ok := checkContent(tc.ext, tc.data)
		assert.Equal(t, tc.ok, ok, "%s with %q", tc.ext, tc.data[:min(8, len(tc.data))])
		assert.Equal(t, tc.mime, mime, tc.ext)
	}
}

func TestKnownTypesAreConsistent(t *testing.T) {
	for ext, ft := range knownTypes {
		assert.False(t, blockedExts[ext], "%s is both known and blocked", ext)
		assert.Contains(t, allKinds, ft.kind, ext)
		assert.NotEmpty(t, ft.mime, ext)
		if ft.sniffed == nil {
			assert.NotEmpty(t, familySniffed[ft.family], "%s has no accepted sniff result", ext)
		}
	}
	for _, ext := range []string{"html", "htm", "svg", "js", "exe", "sh", "bat", "php", "xml"} {
		assert.True(t, blockedExts[ext], ext)
	}
	assert.Equal(t, KindOther, typeOf("dwg").kind)
	assert.True(t, isInline("image/png"))
	assert.True(t, isInline("application/pdf"))
	assert.False(t, isInline("text/plain; charset=utf-8"))
	assert.False(t, isInline("application/zip"))
}

type stubPolicy struct {
	exts []string
	max  int64
	err  error
}

func (s stubPolicy) UploadPolicy(context.Context) ([]string, int64, error) {
	return s.exts, s.max, s.err
}

func TestEffectivePolicy(t *testing.T) {
	ctx := context.Background()
	m := New(Deps{Config: config.FileConfig{MaxUploadBytes: 1000}})
	p, err := m.effectivePolicy(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1000), p.maxBytes)
	assert.Contains(t, p.exts, "png")
	assert.NotContains(t, p.exts, "svg")

	m = New(Deps{Config: config.FileConfig{MaxUploadBytes: 1000},
		Policy: stubPolicy{exts: []string{"PNG", ".pdf", "svg", "html", "exe", "bad ext", "png", "dwg"}, max: 500}})
	p, err = m.effectivePolicy(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"dwg", "pdf", "png"}, p.exts, "blocked and malformed extensions are dropped")
	assert.Equal(t, int64(500), p.maxBytes, "the lower limit wins")

	m = New(Deps{Config: config.FileConfig{MaxUploadBytes: 1000}, Policy: stubPolicy{exts: []string{"png"}, max: 5000}})
	p, err = m.effectivePolicy(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1000), p.maxBytes, "policy cannot exceed the config ceiling")

	m = New(Deps{Config: config.FileConfig{MaxUploadBytes: 1000}, Policy: stubPolicy{err: errors.New("db down")}})
	_, err = m.effectivePolicy(ctx)
	require.Error(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, httpx.AsError(err).Status)
}

func TestCheckExtension(t *testing.T) {
	pol := effectivePolicy{exts: []string{"png", "pdf"}}
	assert.NoError(t, checkExtension("png", pol))
	for _, ext := range []string{"", "svg", "exe", "docx"} {
		err := checkExtension(ext, pol)
		require.Error(t, err, ext)
		assert.Equal(t, CodeTypeNotAllowed, httpx.AsError(err).Code)
	}
	// Even a policy listing a blocked extension cannot allow it.
	assert.Error(t, checkExtension("html", effectivePolicy{exts: []string{"html"}}))
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"photo.png":                       "photo.png",
		"  report 2026.pdf  ":             "report 2026.pdf",
		"../../etc/passwd":                "passwd",
		`C:\Users\x\evil.exe`:             "evil.exe",
		"a\x00b\nc.txt":                   "abc.txt",
		"invoice\u202Egnp.exe":            "invoicegnp.exe",
		"中文文件名.docx":                      "中文文件名.docx",
		"trailing.dots...":                "trailing.dots",
		"..":                              "",
		"/":                               "",
		"   ":                             "",
		"bad\xffutf8.txt":                 "badutf8.txt",
		strings.Repeat("x", 300) + ".pdf": strings.Repeat("x", 251) + ".pdf",
	}
	for in, want := range cases {
		assert.Equal(t, want, sanitizeName(in), "%q", in)
	}
	assert.Equal(t, "a.png", withExt("a.png", "png"))
	assert.Equal(t, "a.txt.png", withExt("a.txt", "png"))
	assert.Equal(t, "a.PNG", withExt("a.PNG", "png"))
	assert.Equal(t, "png", extOf("A.PNG"))
	assert.Equal(t, "", extOf("noext"))
}

func TestContentDisposition(t *testing.T) {
	assert.Equal(t, `inline; filename="a.png"; filename*=UTF-8''a.png`, contentDisposition(true, "a.png"))
	assert.Equal(t, `attachment; filename="____.docx"; filename*=UTF-8''%E6%8A%A5%E5%91%8A%E6%B1%87%E6%80%BB.docx`,
		contentDisposition(false, "报告汇总.docx"))
	got := contentDisposition(false, `a "b"\c; d.txt`)
	assert.Equal(t, `attachment; filename="a _b__c; d.txt"; filename*=UTF-8''a%20%22b%22%5Cc%3B%20d.txt`, got)
	assert.NotContains(t, contentDisposition(false, "x\r\ny.txt"), "\n", "header injection must be impossible")
}

func TestHumanSize(t *testing.T) {
	assert.Equal(t, "512 B", humanSize(512))
	assert.Equal(t, "1 KB", humanSize(1024))
	assert.Equal(t, "1.5 MB", humanSize(3<<19))
	assert.Equal(t, "20 MB", humanSize(20<<20))
	assert.Equal(t, "2 GB", humanSize(2<<30))
}

func TestSigner(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	s := signer{key: []byte(strings.Repeat("k", 32)), now: func() time.Time { return now }}
	exp := now.Add(10 * time.Minute)
	token := s.sign(42, exp)

	id, gotExp, err := s.verify(token)
	require.NoError(t, err)
	assert.Equal(t, int64(42), id)
	assert.True(t, exp.Equal(gotExp))

	parts := strings.Split(token, ".")
	tampered := []string{
		"", "abc", "1.2", "1.2.3.4", strings.Repeat("a", 200),
		"43." + parts[1] + "." + parts[2],                                  // other file
		parts[0] + ".9999999999." + parts[2],                               // longer expiry
		parts[0] + "." + parts[1] + "." + parts[2][:len(parts[2])-1] + "A", // flipped mac
		parts[0] + "." + parts[1] + ".!!!",                                 // bad base64
		"-1." + parts[1] + "." + parts[2],
	}
	for _, tok := range tampered {
		_, _, err := s.verify(tok)
		assert.ErrorIs(t, err, errTokenInvalid, tok)
	}

	other := signer{key: []byte(strings.Repeat("o", 32)), now: s.now}
	_, _, err = other.verify(token)
	assert.ErrorIs(t, err, errTokenInvalid, "a different key must not verify")

	later := signer{key: s.key, now: func() time.Time { return exp }}
	_, _, err = later.verify(token)
	assert.ErrorIs(t, err, errTokenExpired, "expiry is exclusive")

	// A token with a bad numeric field but a valid MAC is still rejected.
	forged := "x." + parts[1]
	forged += "." + b64(s.mac(forged))
	_, _, err = s.verify(forged)
	assert.ErrorIs(t, err, errTokenInvalid)
}

func TestNewGeneratesKeyWhenEmpty(t *testing.T) {
	a := New(Deps{})
	b := New(Deps{})
	assert.Len(t, a.signer.key, config.MinSigningKeyBytes)
	assert.NotEqual(t, a.signer.key, b.signer.key)
	assert.Equal(t, Name, a.Name())
	assert.Len(t, a.Permissions(), 5)
	assert.Len(t, a.Menus(), 3)
	assert.NoError(t, a.Seed(context.Background(), nil))
}

func TestMeteredReader(t *testing.T) {
	m := &meteredReader{r: strings.NewReader("hello world"), limit: 5, hash: nopHash{}}
	buf := make([]byte, 4)
	n, err := m.Read(buf)
	assert.Equal(t, 4, n)
	require.NoError(t, err)
	_, err = m.Read(buf)
	assert.ErrorIs(t, err, errTooLarge)
	assert.True(t, m.exceeded)
	assert.Equal(t, http.StatusRequestEntityTooLarge, httpx.AsError(readError(err, 5)).Status)
	assert.Equal(t, http.StatusRequestEntityTooLarge, httpx.AsError(readError(&http.MaxBytesError{Limit: 1}, 5)).Status)
	assert.Equal(t, http.StatusBadRequest, httpx.AsError(readError(errors.New("eof"), 5)).Status)
}
