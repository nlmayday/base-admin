package file

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/storage"
)

// inlineCSP lets same-origin pages frame an image or PDF preview while
// still forbidding scripts. object-src 'self' keeps the browser's built-in
// PDF viewer (an internal same-origin embed) working.
const inlineCSP = "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; object-src 'self'; frame-ancestors 'self'"

// download streams a file to an authenticated caller.
func (m *Module) download(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	row, err := m.loadFileRow(m.db.WithContext(c.Request.Context()), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	m.serve(c, row, c.Query("download") == "1", "private, no-cache")
}

type signedURLView struct {
	// URL is site-relative, e.g. "/api/v1/files/signed/<token>".
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (m *Module) signedURL(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	if _, err := m.loadFileRow(m.db.WithContext(c.Request.Context()), id); err != nil {
		return nil, err
	}
	expires := m.now().Add(m.cfg.SignedURLTTL).Truncate(time.Second)
	// The route template is "<prefix>/files/:id/signed-url"; reuse its
	// prefix so the module does not need to know where it is mounted.
	base := strings.TrimSuffix(c.FullPath(), "/:id/signed-url")
	return signedURLView{URL: base + "/signed/" + m.signer.sign(id, expires), ExpiresAt: expires}, nil
}

// signedDownload serves a file to anyone holding a valid, unexpired link.
func (m *Module) signedDownload(c *gin.Context) {
	id, expires, err := m.signer.verify(c.Param("token"))
	switch {
	case errors.Is(err, errTokenExpired):
		httpx.Fail(c, httpx.NewError(http.StatusGone, CodeLinkExpired, "链接已过期"))
		return
	case err != nil:
		httpx.Fail(c, httpx.NewError(http.StatusNotFound, CodeLinkInvalid, "链接无效"))
		return
	}
	row, err := m.loadFileRow(m.db.WithContext(c.Request.Context()), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	// Public route: lift the connection deadlines only now that the link is
	// known to be valid (the module router does not do it for public routes).
	httpx.ExtendDeadlines(c.Writer, transferTimeout)
	maxAge := int64(expires.Sub(m.now()) / time.Second)
	m.serve(c, row, c.Query("download") == "1", "private, max-age="+strconv.FormatInt(maxAge, 10))
}

// serve writes the object with safe headers: only images and PDF render
// inline, everything else downloads; the browser must not sniff.
func (m *Module) serve(c *gin.Context, row fileRow, forceAttachment bool, cacheControl string) {
	rc, obj, err := m.storage.Open(c.Request.Context(), row.ObjectKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			m.log.Warn("file object missing", zap.Int64("id", row.ID), zap.String("key", row.ObjectKey))
			httpx.Fail(c, httpx.NotFound("文件内容不存在"))
			return
		}
		httpx.Fail(c, storageFailed(err))
		return
	}
	defer func() { _ = rc.Close() }()

	inline := !forceAttachment && isInline(row.MimeType)
	h := c.Writer.Header()
	h.Set("Content-Type", row.MimeType)
	h.Set("Content-Disposition", contentDisposition(inline, row.OriginalName))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", cacheControl)
	h.Set("ETag", `"`+row.Sha256+`"`)
	if inline {
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Content-Security-Policy", inlineCSP)
	}
	if rs, ok := rc.(io.ReadSeeker); ok {
		http.ServeContent(c.Writer, c.Request, "", row.CreatedAt, rs)
		return
	}
	h.Set("Content-Length", strconv.FormatInt(obj.Size, 10))
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, rc); err != nil {
		m.log.Debug("file download interrupted", zap.Int64("id", row.ID), zap.Error(err))
	}
}
