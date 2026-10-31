//go:build integration

package file

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/storage"
)

func TestUploadHappyPath(t *testing.T) {
	h := newHarness(t)
	cat := h.mustCategory("图片")
	v := h.mustUpload("../../logo.png", pngBytes, map[string]string{"categoryId": cat.ID.String()})

	sum := sha256.Sum256(pngBytes)
	assert.Equal(t, "logo.png", v.OriginalName, "path components are stripped")
	assert.Equal(t, "png", v.Ext)
	assert.Equal(t, KindImage, v.Kind)
	assert.Equal(t, "image/png", v.MimeType)
	assert.Equal(t, int64(len(pngBytes)), v.Size)
	assert.Equal(t, hex.EncodeToString(sum[:]), v.Sha256)
	assert.Equal(t, "local", v.Storage)
	require.NotNil(t, v.CategoryID)
	assert.Equal(t, cat.ID, *v.CategoryID)
	assert.Equal(t, "图片", v.CategoryName)
	assert.Equal(t, "上传者", v.UploaderName)

	var key string
	require.NoError(t, h.db.Raw("SELECT object_key FROM sys_files WHERE id = ?", int64(v.ID)).Scan(&key).Error)
	require.NoError(t, storage.ValidateKey(key))
	assert.True(t, strings.HasPrefix(key, h.clock.Now().UTC().Format("2006/01/02/")))
	stored, err := os.ReadFile(filepath.Join(h.dir, filepath.FromSlash(key)))
	require.NoError(t, err)
	assert.Equal(t, pngBytes, stored)

	// The category may also be passed as a query parameter, after the file.
	resp := h.serve(func() *http.Request {
		r := uploadReq(t, "a.pdf", pdfBytes, nil)
		r.URL.RawQuery = "categoryId=" + cat.ID.String()
		return r
	}())
	require.Equal(t, http.StatusOK, resp.Status, string(resp.Body))
	assert.Equal(t, KindDocument, decode[fileView](t, resp).Kind)
}

func TestUploadRejections(t *testing.T) {
	h := newHarness(t, harnessOpts{policy: stubPolicy{exts: []string{"png", "pdf", "txt", "svg", "html"}, max: 1024}})
	big := append(append([]byte{}, pngBytes...), bytes.Repeat([]byte{0}, 2048)...)
	cases := []struct {
		name    string
		file    string
		content []byte
		status  int
		code    int
	}{
		{"too large", "big.png", big, http.StatusRequestEntityTooLarge, CodeTooLarge},
		{"extension not in policy", "a.docx", zipBytes(), http.StatusUnprocessableEntity, CodeTypeNotAllowed},
		{"svg always refused", "x.svg", svgBytes, http.StatusUnprocessableEntity, CodeTypeNotAllowed},
		{"html always refused", "x.html", htmlBytes, http.StatusUnprocessableEntity, CodeTypeNotAllowed},
		{"exe refused", "x.exe", exeBytes, http.StatusUnprocessableEntity, CodeTypeNotAllowed},
		{"no extension", "README", textBytes, http.StatusUnprocessableEntity, CodeTypeNotAllowed},
		{"exe renamed png", "photo.png", exeBytes, http.StatusUnprocessableEntity, CodeContentMismatch},
		{"html renamed txt", "notes.txt", htmlBytes, http.StatusUnprocessableEntity, CodeContentMismatch},
		{"svg renamed png", "icon.png", svgBytes, http.StatusUnprocessableEntity, CodeContentMismatch},
		{"jpeg renamed pdf", "doc.pdf", jpegBytes, http.StatusUnprocessableEntity, CodeContentMismatch},
		{"empty file", "empty.txt", nil, http.StatusUnprocessableEntity, CodeEmptyFile},
		{"bad file name", "..", pngBytes, http.StatusUnprocessableEntity, httpx.CodeValidation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := h.upload(tc.file, tc.content, nil)
			assert.Equal(t, tc.status, resp.Status, string(resp.Body))
			assert.Equal(t, tc.code, resp.Code)
		})
	}

	resp := h.upload("", nil, map[string]string{"categoryId": "1"})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Status, "missing file part")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	assert.Equal(t, http.StatusBadRequest, h.serve(req).Status, "non-multipart body")

	assert.Empty(t, h.storedFiles(), "rejected uploads must not leave objects")
	assert.Zero(t, h.liveRows())
}

func TestUploadBodyLimits(t *testing.T) {
	h := newHarness(t)
	// Over the route's MaxBody (ceiling + 1 MiB): refused by the middleware
	// before the handler runs.
	huge := append(append([]byte{}, pngBytes...), bytes.Repeat([]byte{0}, testCeiling+multipartOverhead)...)
	resp := h.upload("huge.png", huge, nil)
	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.Status)

	// Between the config ceiling and MaxBody: stopped while streaming, and
	// the declared Content-Length is not trusted.
	over := append(append([]byte{}, pngBytes...), bytes.Repeat([]byte{0}, testCeiling)...)
	req := uploadReq(t, "over.png", over, nil)
	req.ContentLength = -1
	resp = h.serve(req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.Status)
	assert.Equal(t, CodeTooLarge, resp.Code)

	// A body above the global 2 MiB default is fine on the upload route.
	ok := append(append([]byte{}, pngBytes...), bytes.Repeat([]byte{0}, 3<<20)...)
	h.mustUpload("ok.png", ok, nil)
	assert.Len(t, h.storedFiles(), 1)
}

func TestUploadDoesNotLeaveOrphans(t *testing.T) {
	h := newHarness(t)
	resp := h.upload("a.png", pngBytes, map[string]string{"categoryId": "999999"})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Status, "unknown category")
	resp = h.upload("a.png", pngBytes, map[string]string{"categoryId": "abc"})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Status)

	// Two file parts in one request: the first object is removed again.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, name := range []string{"a.png", "b.png"} {
		fw, err := w.CreateFormFile("file", name)
		require.NoError(t, err)
		_, err = fw.Write(pngBytes)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	assert.Equal(t, http.StatusBadRequest, h.serve(req).Status)

	assert.Empty(t, h.storedFiles())
	assert.Zero(t, h.liveRows())
}

func TestUploadStorageFailure(t *testing.T) {
	h := newHarness(t, harnessOpts{storage: func(s storage.Storage) storage.Storage {
		return &failingStorage{Storage: s, failPut: true}
	}})
	resp := h.upload("a.png", pngBytes, nil)
	assert.Equal(t, http.StatusServiceUnavailable, resp.Status)
	assert.Equal(t, CodeStorageFailed, resp.Code)
	assert.Zero(t, h.liveRows())
}

func TestPermissions(t *testing.T) {
	h := newHarness(t)
	v := h.mustUpload("a.png", pngBytes, nil)

	h.setPerms(permFileView)
	assert.Equal(t, http.StatusForbidden, h.upload("b.png", pngBytes, nil).Status)
	assert.Equal(t, http.StatusForbidden, h.do(http.MethodDelete, "/api/v1/files/"+v.ID.String(), nil).Status)
	assert.Equal(t, http.StatusForbidden, h.do(http.MethodGet, "/api/v1/files/storage", nil).Status)
	assert.Equal(t, http.StatusForbidden, h.do(http.MethodPost, "/api/v1/files/categories", gin.H{"name": "x"}).Status)
	assert.Equal(t, http.StatusOK, h.do(http.MethodGet, "/api/v1/files", nil).Status)

	h.setAnonymous()
	assert.Equal(t, http.StatusUnauthorized, h.do(http.MethodGet, "/api/v1/files", nil).Status)
	assert.Equal(t, http.StatusUnauthorized, h.do(http.MethodGet, "/api/v1/files/"+v.ID.String()+"/download", nil).Status)
}

func TestUploadPolicyEndpoint(t *testing.T) {
	h := newHarness(t, harnessOpts{policy: stubPolicy{exts: []string{"png", "svg"}, max: 1 << 20}})
	p := decode[policyView](t, h.do(http.MethodGet, "/api/v1/files/upload-policy", nil))
	assert.Equal(t, []string{"png"}, p.Extensions)
	assert.Equal(t, int64(1<<20), p.MaxBytes)
}
