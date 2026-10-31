//go:build integration

package file

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/storage"
)

func TestListAndFilter(t *testing.T) {
	h := newHarness(t)
	cat := h.mustCategory("合同")
	h.mustUpload("合同_A.pdf", pdfBytes, map[string]string{"categoryId": cat.ID.String()})
	h.mustUpload("photo.png", pngBytes, nil)
	h.mustUpload("data.csv", textBytes, nil)
	h.mustUpload("pack.zip", zipBytes(), nil)
	h.mustUpload("100%_off.txt", textBytes, nil)

	list := func(query string) httpx.Page[fileView] {
		t.Helper()
		resp := h.do(http.MethodGet, "/api/v1/files?"+query, nil)
		require.Equal(t, http.StatusOK, resp.Status, string(resp.Body))
		return decode[httpx.Page[fileView]](t, resp)
	}
	all := list("")
	assert.Equal(t, int64(5), all.Total)
	assert.Equal(t, "100%_off.txt", all.List[0].OriginalName, "newest first")

	assert.Equal(t, int64(1), list("kind=image").Total)
	assert.Equal(t, int64(3), list("kind=document").Total)
	assert.Equal(t, int64(1), list("kind=archive").Total)
	assert.Equal(t, int64(0), list("kind=media").Total)
	assert.Equal(t, int64(1), list("categoryId="+cat.ID.String()).Total)
	assert.Equal(t, int64(4), list("categoryId=none").Total)
	assert.Equal(t, int64(1), list("keyword=合同").Total)
	assert.Equal(t, int64(1), list("keyword=PHOTO").Total, "keyword is case-insensitive")
	assert.Equal(t, int64(1), list("keyword=%25").Total, "LIKE wildcards are escaped")
	assert.Equal(t, int64(0), list("keyword=_x").Total)

	page := list("page=2&pageSize=2")
	assert.Len(t, page.List, 2)
	assert.Equal(t, int64(5), page.Total)

	for _, q := range []string{"kind=exe", "categoryId=abc", "pageSize=1000"} {
		assert.Equal(t, http.StatusUnprocessableEntity, h.do(http.MethodGet, "/api/v1/files?"+q, nil).Status, q)
	}
}

func TestGetRenameMove(t *testing.T) {
	h := newHarness(t)
	cat := h.mustCategory("A")
	v := h.mustUpload("old.png", pngBytes, nil)
	path := "/api/v1/files/" + v.ID.String()

	got := decode[fileView](t, h.do(http.MethodGet, path, nil))
	assert.Equal(t, v.ID, got.ID)
	assert.Equal(t, http.StatusNotFound, h.do(http.MethodGet, "/api/v1/files/999999", nil).Status)

	resp := h.do(http.MethodPut, path, gin.H{"originalName": "新名字.txt", "categoryId": cat.ID.String(), "version": v.Version})
	require.Equal(t, http.StatusOK, resp.Status, string(resp.Body))
	updated := decode[fileView](t, resp)
	assert.Equal(t, "新名字.txt.png", updated.OriginalName, "the stored extension is kept")
	assert.Equal(t, "image/png", updated.MimeType)
	require.NotNil(t, updated.CategoryID)
	assert.Equal(t, cat.ID, *updated.CategoryID)
	assert.Equal(t, v.Version+1, updated.Version)

	stale := h.do(http.MethodPut, path, gin.H{"originalName": "x.png", "version": v.Version})
	assert.Equal(t, http.StatusConflict, stale.Status, "stale version")

	resp = h.do(http.MethodPut, path, gin.H{"originalName": "x.png", "categoryId": nil, "version": updated.Version})
	require.Equal(t, http.StatusOK, resp.Status)
	moved := decode[fileView](t, resp)
	assert.Nil(t, moved.CategoryID, "null moves the file out of its category")
	assert.Equal(t, "x.png", moved.OriginalName)

	bad := []gin.H{
		{"originalName": "x.png"},
		{"originalName": "", "version": moved.Version},
		{"originalName": "..", "version": moved.Version},
		{"originalName": "x.png", "categoryId": "999999", "version": moved.Version},
	}
	for _, body := range bad {
		assert.Equal(t, http.StatusUnprocessableEntity, h.do(http.MethodPut, path, body).Status, body)
	}
}

func TestDeleteRemovesObject(t *testing.T) {
	h := newHarness(t)
	v := h.mustUpload("a.png", pngBytes, nil)
	require.Len(t, h.storedFiles(), 1)
	path := "/api/v1/files/" + v.ID.String()

	require.Equal(t, http.StatusOK, h.do(http.MethodDelete, path, nil).Status)
	assert.Empty(t, h.storedFiles())
	assert.Zero(t, h.liveRows())
	var rows int64
	require.NoError(t, h.db.Raw("SELECT count(*) FROM sys_files").Scan(&rows).Error)
	assert.Zero(t, rows, "once the object is gone the row is removed too")

	assert.Equal(t, http.StatusNotFound, h.do(http.MethodDelete, path, nil).Status)
	assert.Equal(t, http.StatusNotFound, h.do(http.MethodGet, path, nil).Status)
	assert.Equal(t, http.StatusNotFound, h.do(http.MethodGet, path+"/download", nil).Status)
}

// A storage failure must not leave a live row without its object: the row
// is soft-deleted at once and the purge job removes the object later.
func TestDeleteIsFinishedByPurgeWhenStorageFails(t *testing.T) {
	var fs *failingStorage
	h := newHarness(t, harnessOpts{storage: func(s storage.Storage) storage.Storage {
		fs = &failingStorage{Storage: s}
		return fs
	}})
	v := h.mustUpload("a.png", pngBytes, nil)
	fs.failDelete = true
	assert.Equal(t, http.StatusOK, h.do(http.MethodDelete, "/api/v1/files/"+v.ID.String(), nil).Status)
	assert.Zero(t, h.liveRows(), "the file is gone for users at once")
	require.Len(t, h.storedFiles(), 1, "the object is still there")

	ctx := context.Background()
	h.clock.Advance(purgeGrace + time.Minute)
	assert.Error(t, h.mod.purgeDeleted(ctx, nil), "a failing purge is reported and retried next run")

	fs.failDelete = false
	require.NoError(t, h.mod.purgeDeleted(ctx, nil))
	assert.Empty(t, h.storedFiles())
	var rows int64
	require.NoError(t, h.db.Raw("SELECT count(*) FROM sys_files").Scan(&rows).Error)
	assert.Zero(t, rows)
}

func TestPurgeLeavesRecentDeletions(t *testing.T) {
	var fs *failingStorage
	h := newHarness(t, harnessOpts{storage: func(s storage.Storage) storage.Storage {
		fs = &failingStorage{Storage: s}
		return fs
	}})
	v := h.mustUpload("a.png", pngBytes, nil)
	fs.failDelete = true
	require.Equal(t, http.StatusOK, h.do(http.MethodDelete, "/api/v1/files/"+v.ID.String(), nil).Status)
	fs.failDelete = false
	require.NoError(t, h.mod.purgeDeleted(context.Background(), nil))
	assert.Len(t, h.storedFiles(), 1, "deletions younger than the grace period are left alone")
}

func TestDownloadHeaders(t *testing.T) {
	h := newHarness(t)
	img := h.mustUpload("图 1.png", pngBytes, nil)
	doc := h.mustUpload("报告.docx", zipBytes(), nil)
	txt := h.mustUpload("notes.txt", textBytes, nil)

	resp := h.do(http.MethodGet, "/api/v1/files/"+img.ID.String()+"/download", nil)
	require.Equal(t, http.StatusOK, resp.Status)
	assert.Equal(t, pngBytes, resp.Body)
	assert.Equal(t, "image/png", resp.Header.Get("Content-Type"))
	assert.Equal(t, `inline; filename="_ 1.png"; filename*=UTF-8''%E5%9B%BE%201.png`, resp.Header.Get("Content-Disposition"))
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	assert.Equal(t, "private, no-cache", resp.Header.Get("Cache-Control"))
	assert.Equal(t, "SAMEORIGIN", resp.Header.Get("X-Frame-Options"))
	assert.Equal(t, `"`+img.Sha256+`"`, resp.Header.Get("ETag"))

	resp = h.do(http.MethodGet, "/api/v1/files/"+doc.ID.String()+"/download", nil)
	require.Equal(t, http.StatusOK, resp.Status)
	assert.True(t, strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment;"))
	assert.Contains(t, resp.Header.Get("Content-Disposition"), "filename*=UTF-8''%E6%8A%A5%E5%91%8A.docx")
	assert.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"), "attachments keep the global frame policy")

	resp = h.do(http.MethodGet, "/api/v1/files/"+txt.ID.String()+"/download", nil)
	assert.True(t, strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment;"), "text is never inline")

	resp = h.do(http.MethodGet, "/api/v1/files/"+img.ID.String()+"/download?download=1", nil)
	assert.True(t, strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment;"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+img.ID.String()+"/download", nil)
	req.Header.Set("Range", "bytes=0-3")
	ranged := h.serve(req)
	assert.Equal(t, http.StatusPartialContent, ranged.Status)
	assert.Equal(t, pngBytes[:4], ranged.Body)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/files/"+img.ID.String()+"/download", nil)
	req.Header.Set("If-None-Match", `"`+img.Sha256+`"`)
	assert.Equal(t, http.StatusNotModified, h.serve(req).Status)
}

func TestDownloadMissingObject(t *testing.T) {
	h := newHarness(t)
	v := h.mustUpload("a.png", pngBytes, nil)
	for _, f := range h.storedFiles() {
		require.NoError(t, h.store.Delete(t.Context(), f))
	}
	resp := h.do(http.MethodGet, "/api/v1/files/"+v.ID.String()+"/download", nil)
	assert.Equal(t, http.StatusNotFound, resp.Status)
}

func TestSignedURL(t *testing.T) {
	h := newHarness(t)
	img := h.mustUpload("a.png", pngBytes, nil)
	doc := h.mustUpload("b.docx", zipBytes(), nil)

	sign := func(id string) signedURLView {
		t.Helper()
		resp := h.do(http.MethodGet, "/api/v1/files/"+id+"/signed-url", nil)
		require.Equal(t, http.StatusOK, resp.Status, string(resp.Body))
		return decode[signedURLView](t, resp)
	}
	link := sign(img.ID.String())
	assert.True(t, strings.HasPrefix(link.URL, "/api/v1/files/signed/"), link.URL)
	assert.WithinDuration(t, h.clock.Now().Add(10*time.Minute), link.ExpiresAt, 2*time.Second)
	assert.Equal(t, http.StatusNotFound, h.do(http.MethodGet, "/api/v1/files/999999/signed-url", nil).Status)

	// Public: works without a session.
	h.setAnonymous()
	resp := h.do(http.MethodGet, link.URL, nil)
	require.Equal(t, http.StatusOK, resp.Status, string(resp.Body))
	assert.Equal(t, pngBytes, resp.Body)
	assert.True(t, strings.HasPrefix(resp.Header.Get("Content-Disposition"), "inline;"))
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	assert.Regexp(t, `^private, max-age=(599|600)$`, resp.Header.Get("Cache-Control"))
	assert.Contains(t, resp.Header.Get("Content-Security-Policy"), "frame-ancestors 'self'")

	resp = h.do(http.MethodGet, link.URL+"?download=1", nil)
	assert.True(t, strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment;"))

	// Tampering: another file id with the same MAC.
	token := strings.TrimPrefix(link.URL, "/api/v1/files/signed/")
	forged := doc.ID.String() + token[strings.Index(token, "."):]
	resp = h.do(http.MethodGet, "/api/v1/files/signed/"+forged, nil)
	assert.Equal(t, http.StatusNotFound, resp.Status)
	assert.Equal(t, CodeLinkInvalid, resp.Code)
	for _, bad := range []string{"garbage", token + "x", strings.Repeat("a", 300)} {
		assert.Equal(t, http.StatusNotFound, h.do(http.MethodGet, "/api/v1/files/signed/"+bad, nil).Status, bad)
	}

	// Expiry.
	h.clock.Advance(10*time.Minute + time.Second)
	resp = h.do(http.MethodGet, link.URL, nil)
	assert.Equal(t, http.StatusGone, resp.Status)
	assert.Equal(t, CodeLinkExpired, resp.Code)

	// A valid link to a deleted file stops working.
	h.setPerms(allPerms...)
	docLink := sign(doc.ID.String())
	require.Equal(t, http.StatusOK, h.do(http.MethodDelete, "/api/v1/files/"+doc.ID.String(), nil).Status)
	assert.Equal(t, http.StatusNotFound, h.do(http.MethodGet, docLink.URL, nil).Status)
}

func TestCategories(t *testing.T) {
	h := newHarness(t)
	a := h.mustCategory("设计稿")
	b := h.mustCategory("空分类")
	h.mustUpload("a.png", pngBytes, map[string]string{"categoryId": a.ID.String()})
	h.mustUpload("b.png", pngBytes, nil)

	resp := h.do(http.MethodPost, "/api/v1/files/categories", gin.H{"name": " 设计稿 "})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Status, "duplicate name")
	assert.Equal(t, http.StatusUnprocessableEntity, h.do(http.MethodPost, "/api/v1/files/categories", gin.H{"name": "  "}).Status)
	assert.Equal(t, http.StatusUnprocessableEntity, h.do(http.MethodPost, "/api/v1/files/categories", gin.H{"name": strings.Repeat("x", 65)}).Status)

	list := decode[categoryList](t, h.do(http.MethodGet, "/api/v1/files/categories", nil))
	assert.Equal(t, int64(2), list.Total)
	assert.Equal(t, int64(1), list.Uncategorized)
	require.Len(t, list.List, 2)
	counts := map[string]int64{}
	for _, c := range list.List {
		counts[c.Name] = c.FileCount
	}
	assert.Equal(t, map[string]int64{"设计稿": 1, "空分类": 0}, counts)

	path := "/api/v1/files/categories/" + b.ID.String()
	resp = h.do(http.MethodPut, path, gin.H{"name": "重命名", "sort": 5, "version": b.Version})
	require.Equal(t, http.StatusOK, resp.Status, string(resp.Body))
	renamed := decode[categoryView](t, resp)
	assert.Equal(t, "重命名", renamed.Name)
	assert.Equal(t, 5, renamed.Sort)
	assert.Equal(t, http.StatusConflict, h.do(http.MethodPut, path, gin.H{"name": "x", "version": b.Version}).Status)
	assert.Equal(t, http.StatusUnprocessableEntity, h.do(http.MethodPut, path, gin.H{"name": "设计稿", "version": renamed.Version}).Status)
	assert.Equal(t, http.StatusUnprocessableEntity, h.do(http.MethodPut, path, gin.H{"name": "x"}).Status, "version required")
	assert.Equal(t, http.StatusNotFound, h.do(http.MethodPut, "/api/v1/files/categories/999999", gin.H{"name": "x", "version": 0}).Status)

	resp = h.do(http.MethodDelete, "/api/v1/files/categories/"+a.ID.String(), nil)
	assert.Equal(t, http.StatusConflict, resp.Status)
	assert.Equal(t, CodeCategoryInUse, resp.Code)

	require.Equal(t, http.StatusOK, h.do(http.MethodDelete, path, nil).Status)
	assert.Equal(t, http.StatusNotFound, h.do(http.MethodDelete, path, nil).Status)

	// Soft-deleted files do not block deletion.
	files := decode[httpx.Page[fileView]](t, h.do(http.MethodGet, "/api/v1/files?categoryId="+a.ID.String(), nil))
	require.Len(t, files.List, 1)
	require.Equal(t, http.StatusOK, h.do(http.MethodDelete, "/api/v1/files/"+files.List[0].ID.String(), nil).Status)
	assert.Equal(t, http.StatusOK, h.do(http.MethodDelete, "/api/v1/files/categories/"+a.ID.String(), nil).Status)
}

func TestStorageStats(t *testing.T) {
	h := newHarness(t, harnessOpts{policy: stubPolicy{exts: []string{"png", "pdf", "zip"}, max: 2 << 20}})
	h.mustCategory("c")
	h.mustUpload("a.png", pngBytes, nil)
	h.mustUpload("b.png", pngBytes, nil)
	h.mustUpload("c.pdf", pdfBytes, nil)
	z := h.mustUpload("d.zip", zipBytes(), nil)
	require.Equal(t, http.StatusOK, h.do(http.MethodDelete, "/api/v1/files/"+z.ID.String(), nil).Status)

	resp := h.do(http.MethodGet, "/api/v1/files/storage", nil)
	require.Equal(t, http.StatusOK, resp.Status, string(resp.Body))
	assert.NotContains(t, string(resp.Body), testSigningKey)
	s := decode[storageView](t, resp)
	assert.Equal(t, "local", s.Driver)
	assert.Equal(t, h.dir, s.Location.Dir)
	assert.Equal(t, int64(testCeiling), s.MaxUploadBytes)
	assert.Equal(t, int64(600), s.SignedURLTTLSecs)
	require.NotNil(t, s.Policy)
	assert.Equal(t, int64(2<<20), s.Policy.MaxBytes)
	assert.Equal(t, int64(3), s.FileCount, "deleted files are not counted")
	assert.Equal(t, int64(2*len(pngBytes)+len(pdfBytes)), s.TotalSize)
	assert.Equal(t, int64(1), s.CategoryCount)
	require.Len(t, s.ByKind, len(allKinds))
	byKind := map[string]usageStat{}
	for _, k := range s.ByKind {
		byKind[k.Key] = k
	}
	assert.Equal(t, int64(2), byKind[KindImage].Count)
	assert.Equal(t, int64(2*len(pngBytes)), byKind[KindImage].Size)
	assert.Equal(t, int64(1), byKind[KindDocument].Count)
	assert.Equal(t, int64(0), byKind[KindArchive].Count)
	require.NotEmpty(t, s.TopExtensions)
	assert.Equal(t, "png", s.TopExtensions[0].Key)
	require.Len(t, s.Daily, trendDays)
	assert.Equal(t, int64(3), s.Daily[trendDays-1].Count, "today's uploads")
}
