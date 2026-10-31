// Package file implements the media library: uploads into a pluggable
// storage driver, categories, authenticated and signed downloads, and
// storage statistics (docs/p1-p3-plan.md, file).
package file

import (
	"context"
	"crypto/rand"
	"slices"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/core/perm"
	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/storage"
)

// UploadPolicy supplies the runtime upload rules (the system module
// implements it). exts are lowercase extensions without the dot; maxBytes
// is lowered further by config.FileConfig.MaxUploadBytes.
type UploadPolicy interface {
	UploadPolicy(ctx context.Context) (exts []string, maxBytes int64, err error)
}

// Deps are the module's dependencies.
type Deps struct {
	DB      *gorm.DB
	Storage storage.Storage
	// Policy is optional; without it every known safe type is allowed up
	// to Config.MaxUploadBytes.
	Policy UploadPolicy
	Config config.FileConfig
	Logger *zap.Logger
	// Now is optional and only overridden in tests.
	Now func() time.Time
}

// multipartOverhead is the room left for multipart headers and form fields
// on top of the file itself.
const multipartOverhead = 1 << 20

// transferTimeout replaces the global request timeout on uploads and
// downloads, which can take minutes for large files on slow links.
const transferTimeout = 30 * time.Minute

// Module is the file module.
type Module struct {
	db      *gorm.DB
	storage storage.Storage
	policy  UploadPolicy
	cfg     config.FileConfig
	log     *zap.Logger
	now     func() time.Time
	signer  signer
}

var _ module.Module = (*Module)(nil)

// New creates the file module. An empty Config.SigningKey (only possible
// outside prod, config validation enforces it there) gets a random key.
func New(d Deps) *Module {
	logger := d.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	now := d.Now
	if now == nil {
		now = time.Now
	}
	key := []byte(d.Config.SigningKey)
	if len(key) == 0 {
		key = make([]byte, config.MinSigningKeyBytes)
		_, _ = rand.Read(key)
		logger.Warn("file.signing_key is empty; using a random key, signed links will not survive a restart")
	}
	return &Module{
		db: d.DB, storage: d.Storage, policy: d.Policy, cfg: d.Config, log: logger, now: now,
		signer: signer{key: key, now: now},
	}
}

// Name implements module.Module.
func (m *Module) Name() string { return Name }

// Permissions implements module.Module.
func (m *Module) Permissions() []perm.Def { return slices.Clone(permissionDefs) }

// Menus implements module.Module.
func (m *Module) Menus() []menu.Def { return slices.Clone(menuDefs) }

// Seed implements module.Module. The module has no seed data.
func (m *Module) Seed(context.Context, *gorm.DB) error { return nil }

// Routes implements module.Module.
func (m *Module) Routes(r *module.Router) {
	files := r.Group("/files")
	files.GET("", access.Perm(permFileView), handle(m.listFiles)).Doc(module.Doc{Summary: "文件列表", Query: fileFilter{}, Response: httpx.Page[fileView]{}})
	files.POST("", access.Perm(permFileUpload), handle(m.upload)).
		Doc(module.Doc{Summary: "上传文件", Description: "multipart/form-data：file 为文件，categoryId 可选", Response: fileView{}}).
		MaxBody(m.cfg.MaxUploadBytes + multipartOverhead).Timeout(transferTimeout)
	files.POST("/attachments", access.Authenticated(), handle(m.uploadAttachment)).
		Doc(module.Doc{Summary: "上传附件", Description: "业务表单用（头像、图片、附件），登录即可上传，规则同文件上传；?kind=image 只接受图片。返回的 url 存入业务表", Response: attachmentView{}}).
		MaxBody(m.cfg.MaxUploadBytes + multipartOverhead).Timeout(transferTimeout)
	files.GET("/a/*key", access.Authenticated(), m.viewAttachment).
		Doc(module.Doc{Summary: "访问附件", Description: "按上传附件返回的 url 访问，需要登录；?download=1 强制下载"}).Timeout(transferTimeout)
	files.GET("/upload-policy", access.Authenticated(), handle(m.uploadPolicyView)).Doc(module.Doc{Summary: "上传规则", Response: policyView{}})
	files.GET("/storage", access.Perm(permStorageView), handle(m.storageStats)).Doc(module.Doc{Summary: "存储概况", Response: storageView{}})
	files.GET("/signed/:token", access.Public(), m.signedDownload).
		Doc(module.Doc{Summary: "签名链接访问文件", Description: "?download=1 强制以附件下载"}).Timeout(transferTimeout)

	cats := files.Group("/categories")
	cats.GET("", access.Perm(permFileView), handle(m.listCategories)).Doc(module.Doc{Summary: "文件分类列表", Response: categoryList{}})
	cats.POST("", access.Perm(permFileUpdate), handle(m.createCategory)).Doc(module.Doc{Summary: "新增文件分类", Body: categoryRequest{}, Response: categoryView{}})
	cats.PUT("/:id", access.Perm(permFileUpdate), handle(m.updateCategory)).Doc(module.Doc{Summary: "编辑文件分类", Body: categoryRequest{}, Response: categoryView{}})
	cats.DELETE("/:id", access.Perm(permFileDelete), handle(m.deleteCategory)).Doc(module.Doc{Summary: "删除文件分类"})

	files.GET("/:id", access.Perm(permFileView), handle(m.getFile)).Doc(module.Doc{Summary: "文件详情", Response: fileView{}})
	files.PUT("/:id", access.Perm(permFileUpdate), handle(m.updateFile)).Doc(module.Doc{Summary: "重命名/移动文件", Body: updateFileRequest{}, Response: fileView{}})
	files.DELETE("/:id", access.Perm(permFileDelete), handle(m.deleteFile)).Doc(module.Doc{Summary: "删除文件"})
	files.GET("/:id/download", access.Perm(permFileView), m.download).Doc(module.Doc{Summary: "下载文件", Sensitive: true}).Timeout(transferTimeout)
	files.GET("/:id/signed-url", access.Perm(permFileView), handle(m.signedURL)).Doc(module.Doc{Summary: "生成文件签名链接", Response: signedURLView{}, Sensitive: true})
}
