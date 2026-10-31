//go:build integration

package file

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uploadAttachment posts a file to the attachment endpoint.
func (h *harness) uploadAttachment(query, name string, content []byte) response {
	h.t.Helper()
	req := uploadReq(h.t, name, content, nil)
	req.URL.Path, req.URL.RawQuery = "/api/v1/files/attachments", query
	req.RequestURI = ""
	return h.serve(req)
}

func TestAttachments(t *testing.T) {
	h := newHarness(t)
	h.setPerms() // a signed-in user without any file permission

	resp := h.uploadAttachment("", "合同.pdf", pdfBytes)
	require.Equal(t, http.StatusOK, resp.Status, string(resp.Body))
	var doc attachmentView
	require.NoError(t, json.Unmarshal(resp.Data, &doc))
	assert.Equal(t, "合同.pdf", doc.Name)
	assert.Equal(t, "document", doc.Kind)
	require.True(t, strings.HasPrefix(doc.URL, "/api/v1/files/a/"), doc.URL)
	assert.Regexp(t, `/[0-9a-f]{32}\.pdf$`, doc.URL, "the URL carries the random object key")

	got := h.serve(httptest.NewRequest(http.MethodGet, doc.URL, nil))
	require.Equal(t, http.StatusOK, got.Status)
	assert.Equal(t, pdfBytes, got.Body)
	assert.Equal(t, "nosniff", got.Header.Get("X-Content-Type-Options"))

	// The media library still needs its own permission.
	assert.Equal(t, http.StatusForbidden, h.do(http.MethodGet, "/api/v1/files/"+doc.ID.String(), nil).Status)

	// ?kind=image refuses other types and leaves nothing behind.
	before := len(h.storedFiles())
	resp = h.uploadAttachment("kind=image", "a.pdf", pdfBytes)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Status, string(resp.Body))
	assert.Equal(t, CodeTypeNotAllowed, resp.Code)
	assert.Len(t, h.storedFiles(), before, "the refused object is deleted")
	resp = h.uploadAttachment("kind=image", "avatar.png", pngBytes)
	require.Equal(t, http.StatusOK, resp.Status, string(resp.Body))

	for _, path := range []string{
		"/api/v1/files/a/2026/01/01/00000000000000000000000000000000.pdf", // unknown key
		"/api/v1/files/a/../../etc/passwd",                                // invalid key
	} {
		assert.Equal(t, http.StatusNotFound, h.serve(httptest.NewRequest(http.MethodGet, path, nil)).Status, path)
	}

	// Deleted in the media library: the link stops working.
	h.setPerms(permFileDelete, permFileView)
	require.Equal(t, http.StatusOK, h.do(http.MethodDelete, "/api/v1/files/"+doc.ID.String(), nil).Status)
	assert.Equal(t, http.StatusNotFound, h.serve(httptest.NewRequest(http.MethodGet, doc.URL, nil)).Status)

	h.setAnonymous()
	assert.Equal(t, http.StatusUnauthorized, h.uploadAttachment("", "a.pdf", pdfBytes).Status)
}
