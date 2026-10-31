package file

import (
	"strings"

	"github.com/gin-gonic/gin"

	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
	"base-admin/server/internal/platform/storage"
)

// Attachments: files uploaded from business forms (avatars, images and
// documents attached to records). Any signed-in user may upload one, under
// the same upload rules as the media library, and read it back by its URL.
// The URL carries the object key, whose 128 random bits make it the
// capability: records store the URL, and whoever can see the record (and is
// signed in) can open the file, without the media library permission and
// without IDs that could be enumerated.

// attachmentPath is the route prefix that serves attachments by key.
const attachmentPath = "/files/a/"

type attachmentView struct {
	ID       ids.ID `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url" doc:"站内地址，存入业务表；访问需要登录"`
	Size     int64  `json:"size"`
	Kind     string `json:"kind" doc:"image / document / media / archive / other"`
	MimeType string `json:"mimeType"`
}

// uploadAttachment stores one attachment. With ?kind=image only images are
// accepted.
func (m *Module) uploadAttachment(c *gin.Context) (any, error) {
	ctx := c.Request.Context()
	obj, _, err := m.receive(c)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			m.discard(obj.key)
		}
	}()
	if c.Query("kind") == "image" && obj.kind != "image" {
		return nil, typeNotAllowed("只能上传图片")
	}
	id, err := m.insertFile(ctx, obj, nil)
	if err != nil {
		return nil, err
	}
	committed = true
	return attachmentView{
		ID: ids.ID(id), Name: obj.name, URL: m.attachmentURL(c, obj.key),
		Size: obj.size, Kind: obj.kind, MimeType: obj.mime,
	}, nil
}

// attachmentURL is the site-relative URL of key, under the same prefix as
// the current route.
func (m *Module) attachmentURL(c *gin.Context, key string) string {
	base := strings.TrimSuffix(c.FullPath(), "/files/attachments")
	return base + attachmentPath + key
}

// viewAttachment serves a live file by its object key.
func (m *Module) viewAttachment(c *gin.Context) {
	key := strings.TrimPrefix(c.Param("key"), "/")
	if storage.ValidateKey(key) != nil {
		httpx.Fail(c, httpx.NotFound("文件不存在"))
		return
	}
	var rows []fileRow
	if err := liveFiles(m.db.WithContext(c.Request.Context())).Select(fileColumns).
		Where("f.object_key = ?", key).Scan(&rows).Error; err != nil {
		httpx.Fail(c, err)
		return
	}
	if len(rows) == 0 {
		httpx.Fail(c, httpx.NotFound("文件不存在"))
		return
	}
	m.serve(c, rows[0], c.Query("download") == "1", "private, max-age=86400")
}
