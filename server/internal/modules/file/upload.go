package file

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
	"base-admin/server/internal/platform/storage"
)

const (
	// sniffLen is what http.DetectContentType looks at.
	sniffLen = 512
	// maxFieldBytes bounds non-file form fields.
	maxFieldBytes = 64
	// cleanupTimeout bounds deleting an orphaned object after a failure.
	cleanupTimeout = 30 * time.Second
)

var (
	errTooLarge  = errors.New("file: upload exceeds the size limit")
	validExtName = regexp.MustCompile(`^[a-z0-9]{1,16}$`)
)

// effectivePolicy is the upload policy capped by the configured ceiling.
type effectivePolicy struct {
	exts     []string
	maxBytes int64
}

type policyView struct {
	Extensions []string `json:"extensions"`
	MaxBytes   int64    `json:"maxBytes"`
}

func (m *Module) effectivePolicy(ctx context.Context) (effectivePolicy, error) {
	exts, maxBytes := defaultExts(), m.cfg.MaxUploadBytes
	if m.policy != nil {
		policyExts, policyMax, err := m.policy.UploadPolicy(ctx)
		if err != nil {
			return effectivePolicy{}, httpx.Unavailable("上传配置暂不可用，请稍后重试").WithCause(err)
		}
		exts = policyExts
		if policyMax > 0 && policyMax < maxBytes {
			maxBytes = policyMax
		}
	}
	allowed := make([]string, 0, len(exts))
	for _, e := range exts {
		e = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(e), "."))
		if validExtName.MatchString(e) && !blockedExts[e] && !slices.Contains(allowed, e) {
			allowed = append(allowed, e)
		}
	}
	slices.Sort(allowed)
	return effectivePolicy{exts: allowed, maxBytes: maxBytes}, nil
}

func (m *Module) uploadPolicyView(c *gin.Context) (any, error) {
	p, err := m.effectivePolicy(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return policyView{Extensions: p.exts, MaxBytes: p.maxBytes}, nil
}

// storedObject is an object written to storage but not yet recorded.
type storedObject struct {
	key, name, ext, kind, mime, sha256 string
	size                               int64
}

// upload streams the multipart "file" part into storage, then records it.
// Any failure after the object was written deletes the object again.
func (m *Module) upload(c *gin.Context) (any, error) {
	ctx := c.Request.Context()
	obj, categoryRaw, err := m.receive(c)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			m.discard(obj.key)
		}
	}()
	categoryID, err := parseOptionalID(categoryRaw)
	if err != nil {
		return nil, err
	}
	id, err := m.insertFile(ctx, obj, categoryID)
	if err != nil {
		return nil, err
	}
	// From here the row references the object: never discard it, even if
	// reading the row back fails.
	committed = true
	return m.loadFile(m.db.WithContext(ctx), id)
}

// receive stores the multipart "file" part and returns it with the raw
// categoryId (query or form field). On error nothing is left in storage;
// otherwise the caller must record or discard the object.
func (m *Module) receive(c *gin.Context) (*storedObject, string, error) {
	ctx := c.Request.Context()
	pol, err := m.effectivePolicy(ctx)
	if err != nil {
		return nil, "", err
	}
	if c.Request.ContentLength > pol.maxBytes+multipartOverhead {
		return nil, "", tooLarge(pol.maxBytes)
	}
	mr, err := c.Request.MultipartReader()
	if err != nil {
		return nil, "", httpx.BadRequest("请使用 multipart/form-data 上传").WithCause(err)
	}
	var obj *storedObject
	fail := func(err error) (*storedObject, string, error) {
		if obj != nil {
			m.discard(obj.key)
		}
		return nil, "", err
	}
	categoryRaw := c.Query("categoryId")
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fail(readError(err, pol.maxBytes))
		}
		switch part.FormName() {
		case "file":
			if obj != nil {
				return fail(httpx.BadRequest("一次只能上传一个文件"))
			}
			if obj, err = m.storePart(ctx, part, pol); err != nil {
				return fail(err)
			}
		case "categoryId":
			if categoryRaw, err = readField(part); err != nil {
				return fail(readError(err, pol.maxBytes))
			}
		}
		_ = part.Close()
	}
	if obj == nil {
		return nil, "", fieldError("file", "请选择要上传的文件")
	}
	return obj, categoryRaw, nil
}

func (m *Module) storePart(ctx context.Context, part *multipart.Part, pol effectivePolicy) (*storedObject, error) {
	name := sanitizeName(part.FileName())
	if name == "" {
		return nil, fieldError("file", "文件名无效")
	}
	ext := extOf(name)
	if err := checkExtension(ext, pol); err != nil {
		return nil, err
	}
	br := bufio.NewReaderSize(part, sniffLen)
	head, err := br.Peek(sniffLen)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, readError(err, pol.maxBytes)
	}
	if len(head) == 0 {
		return nil, httpx.NewError(http.StatusUnprocessableEntity, CodeEmptyFile, "不能上传空文件")
	}
	mimeType, ok := checkContent(ext, head)
	if !ok {
		return nil, httpx.NewError(http.StatusUnprocessableEntity, CodeContentMismatch, "文件内容与扩展名不符")
	}
	key, err := storage.NewKey(m.now(), ext)
	if err != nil {
		return nil, err
	}
	meter := &meteredReader{r: br, limit: pol.maxBytes, hash: sha256.New()}
	if err := m.storage.Put(ctx, key, meter, -1, mimeType); err != nil {
		m.discard(key)
		switch {
		case meter.exceeded:
			return nil, tooLarge(pol.maxBytes)
		case meter.readErr != nil:
			return nil, readError(meter.readErr, pol.maxBytes)
		default:
			return nil, storageFailed(err)
		}
	}
	return &storedObject{
		key: key, name: name, ext: ext, kind: typeOf(ext).kind, mime: mimeType,
		sha256: hex.EncodeToString(meter.hash.Sum(nil)), size: meter.n,
	}, nil
}

func checkExtension(ext string, pol effectivePolicy) error {
	switch {
	case ext == "":
		return typeNotAllowed("文件缺少扩展名")
	case blockedExts[ext] || !slices.Contains(pol.exts, ext):
		return typeNotAllowed("不允许上传 ." + ext + " 类型的文件")
	}
	return nil
}

// insertFile records an uploaded object. It runs detached from the request:
// a client disconnect must not leave it unknown whether the row exists (the
// object would then be discarded under a committed row).
func (m *Module) insertFile(ctx context.Context, obj *storedObject, categoryID *int64) (int64, error) {
	uid := actor.From(ctx).UserID()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	db := m.db.WithContext(ctx)
	id, err := database.InsertID(db, `INSERT INTO sys_files (category_id, original_name, object_key, storage, size, mime_type, ext, kind, sha256, uploaded_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		categoryID, obj.name, obj.key, m.storage.Name(), obj.size, obj.mime, obj.ext, obj.kind, obj.sha256,
		uid)
	if err != nil {
		if database.IsForeignKeyViolation(err) {
			return 0, fieldError("categoryId", "分类不存在")
		}
		return 0, err
	}
	return id, nil
}

// discard deletes an object that has no database row, even when the
// request context has ended.
func (m *Module) discard(key string) {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	if err := m.storage.Delete(ctx, key); err != nil {
		m.log.Warn("delete orphaned upload", zap.String("key", key), zap.Error(err))
	}
}

func readField(part *multipart.Part) (string, error) {
	b, err := io.ReadAll(io.LimitReader(part, maxFieldBytes+1))
	if err != nil {
		return "", err
	}
	if len(b) > maxFieldBytes {
		return "", httpx.BadRequest("表单字段过长")
	}
	return strings.TrimSpace(string(b)), nil
}

// parseOptionalID parses a category id form value; "" means none.
func parseOptionalID(raw string) (*int64, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := ids.Parse(raw)
	if err != nil {
		return nil, fieldError("categoryId", "分类不存在")
	}
	n := int64(id)
	return &n, nil
}

// readError maps a request body read failure to an API error.
func readError(err error, maxBytes int64) error {
	var apiErr *httpx.Error
	if errors.As(err, &apiErr) {
		return apiErr
	}
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) || errors.Is(err, errTooLarge) {
		return tooLarge(maxBytes)
	}
	return httpx.BadRequest("上传内容读取失败").WithCause(err)
}

// meteredReader hashes and counts what storage reads and fails once more
// than limit bytes arrive.
type meteredReader struct {
	r        io.Reader
	limit, n int64
	hash     hash.Hash
	exceeded bool
	readErr  error
}

func (m *meteredReader) Read(p []byte) (int, error) {
	n, err := m.r.Read(p)
	m.n += int64(n)
	if m.n > m.limit {
		m.exceeded = true
		return 0, errTooLarge
	}
	m.hash.Write(p[:n])
	if err != nil && !errors.Is(err, io.EOF) {
		m.readErr = err
	}
	return n, err
}
