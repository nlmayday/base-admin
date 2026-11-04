package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadDefaults(t *testing.T) {
	byColumn := map[string]FieldInfo{}
	for _, f := range mustDescribe(t, productTable()) {
		byColumn[f.Column] = f
	}
	assert.Equal(t, UploadImage, byColumn["cover_url"].Upload, "named like an image")
	assert.Equal(t, UploadImage, byColumn["photo"].Upload)
	assert.Equal(t, SearchNone, byColumn["cover_url"].Search)
	assert.Equal(t, UploadFiles, byColumn["attachments"].Upload)
	assert.Empty(t, byColumn["specs"].Upload, "other jsonb stays JSON text")
	assert.Equal(t, []string{UploadFiles}, byColumn["specs"].UploadModes)
	assert.Equal(t, []string{}, byColumn["name"].UploadModes, "short text cannot hold a URL")
}

func TestUploadGenerated(t *testing.T) {
	spec := Spec{Module: "shop", Resource: "product", Name: "商品", NewModule: true}
	res, err := Generate(productTable(), spec)
	require.NoError(t, err)
	src := fileByPath(t, res, "/product.go").Content
	assert.Contains(t, src, `"base-admin/server/internal/core/attachment"`)
	assert.Contains(t, src, `attachment.URLProblem(req.CoverURL)`)
	assert.Contains(t, src, "if req.Photo != nil {")
	assert.Contains(t, src, `attachment.RefsProblem(req.Attachments)`)
	assert.Contains(t, src, "Attachments []attachment.Ref")
	assert.Contains(t, src, "attachment.Decode(&r.Attachments)")
	assert.Contains(t, src, "attachment.JSON(req.Attachments)")

	form := fileByPath(t, res, "Dialog.vue").Content
	assert.Contains(t, form, `import ReImageUpload from "@/components/ReUpload/ImageUpload.vue";`)
	assert.Contains(t, form, `<ReImageUpload v-model="form.coverUrl" />`)
	assert.Contains(t, form, `<ReFileUpload v-model="form.attachments" />`)
	assert.Contains(t, form, "attachments: [...r.attachments]")
	assert.NotContains(t, form, "请选择附件", "an empty attachment list is allowed")
	api := fileByPath(t, res, "/product.ts").Content
	assert.Contains(t, api, `import type { AttachmentRef } from "@/api/file";`)
	assert.Contains(t, api, "attachments: AttachmentRef[];")
	index := fileByPath(t, res, "/product/index.vue").Content
	assert.Contains(t, index, `:preview-src-list="[row.coverUrl]"`)

	// Turned off: plain inputs, no attachment code.
	off := ""
	spec.Fields = []FieldOption{{Column: "cover_url", Upload: &off}, {Column: "photo", Upload: &off}, {Column: "attachments", Upload: &off}}
	res, err = Generate(productTable(), spec)
	require.NoError(t, err)
	assert.NotContains(t, fileByPath(t, res, "/product.go").Content, "attachment.")
	assert.NotContains(t, fileByPath(t, res, "Dialog.vue").Content, "ReImageUpload")

	// Chosen on another column.
	files := UploadFiles
	spec.Fields = []FieldOption{{Column: "specs", Upload: &files}}
	res, err = Generate(productTable(), spec)
	require.NoError(t, err)
	assert.Contains(t, fileByPath(t, res, "/product.go").Content, "attachment.Decode(r.Specs)", "nullable column")
}

func TestUploadProblems(t *testing.T) {
	image, files := UploadImage, UploadFiles
	spec := Spec{Module: "shop", Resource: "product", Name: "商品", NewModule: true, Fields: []FieldOption{
		{Column: "attachments", Upload: &image},
		{Column: "name", Upload: &files},
		{Column: "cover_url", Upload: &image, Dict: ptr("cover_kind")},
		{Column: "photo", Upload: &image, Search: SearchLike},
	}}
	var msgs []string
	for _, p := range spec.FieldProblems(productTable()) {
		msgs = append(msgs, p.Message)
	}
	assert.Equal(t, []string{
		"字段 attachments 不能使用该上传方式（图片需要长度不小于 255 的文本字段，附件需要 JSON（jsonb 或 json）字段）",
		"字段 name 不能使用该上传方式（图片需要长度不小于 255 的文本字段，附件需要 JSON（jsonb 或 json）字段）",
		"字段 cover_url 不能同时使用上传和下拉选项",
		"字段 photo 使用上传后不能搜索",
	}, msgs)

	// A dictionary chosen for a default image column replaces the upload.
	spec.Fields = []FieldOption{{Column: "cover_url", Dict: ptr("cover_kind")}}
	res, err := Generate(productTable(), spec)
	require.NoError(t, err)
	assert.NotContains(t, fileByPath(t, res, "Dialog.vue").Content, `v-model="form.coverUrl" />`)
}
