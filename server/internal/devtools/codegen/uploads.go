package codegen

import (
	"regexp"
	"strings"
)

// Upload widgets: an image column stores one attachment URL (text), an
// attachments column a JSON list of {name, url, size} (jsonb). Both are
// filled through the file module's attachment upload and checked with
// internal/core/attachment.
const (
	UploadImage = "image"
	UploadFiles = "files"
)

var (
	imageNamePattern = regexp.MustCompile(`(^|_)(avatar|image|img|photo|picture|pic|cover|logo|thumb|thumbnail)(_url)?$`)
	filesNamePattern = regexp.MustCompile(`(^|_)(attachments?|files?)$`)
)

// uploadModes lists the upload widgets that suit f.
func uploadModes(f *Field) []string {
	modes := []string{}
	if f.Role != "" || !f.canWrite {
		return modes
	}
	switch {
	case f.Kind == kindString && (f.MaxLen == 0 || f.MaxLen >= 255):
		modes = append(modes, UploadImage)
	case f.Kind == kindJSON:
		modes = append(modes, UploadFiles)
	}
	return modes
}

// defaultUploads picks upload widgets from column names and comments, such
// as cover_url or "封面图片" (image) and attachments or "附件" (files).
func (d *tmplData) defaultUploads() {
	for _, f := range d.Fields {
		modes := uploadModes(f)
		switch {
		case len(modes) == 0:
		case modes[0] == UploadImage && (imageNamePattern.MatchString(f.Column) || containsAny(f.comment, "图片", "头像", "封面", "照片")):
			f.setUpload(UploadImage)
		case modes[0] == UploadFiles && (filesNamePattern.MatchString(f.Column) || strings.Contains(f.comment, "附件")):
			f.setUpload(UploadFiles)
		}
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// setUpload records the upload widget of f; deriveUploads applies it once
// the options are known. Uploads are not searched and have no options.
func (f *Field) setUpload(mode string) {
	f.Upload = mode
	if mode != "" {
		f.Search = SearchNone
		f.setChoices("")
	}
}

// deriveUploads turns upload fields into their Go and TypeScript forms.
func (d *tmplData) deriveUploads() {
	for _, f := range d.Fields {
		switch f.Upload {
		case UploadImage:
			f.Widget, f.RuleTrig = "image", "change"
		case UploadFiles:
			d.HasFiles = true
			f.Widget, f.RuleTrig = "files", "change"
			row := "&r." + f.GoName
			if f.Nullable {
				row = "r." + f.GoName
			}
			f.ViewType, f.ViewExpr = "[]attachment.Ref", "attachment.Decode("+row+")"
			f.ReqType, f.ArgExpr, f.Binding = "[]attachment.Ref", "attachment.JSON(req."+f.GoName+")", ""
			f.Required = false // an empty list is stored as []
			f.TSType, f.TSReqType, f.TSForm = "AttachmentRef[]", "AttachmentRef[]", "AttachmentRef[]"
			f.TSDefault, f.TSSubmit = "[]", "form."+f.JSONName
		default:
			continue
		}
		d.NeedsAttachment = d.NeedsAttachment || f.Upload == UploadFiles || f.Writable
		if f.Writable {
			d.UploadFields = append(d.UploadFields, f)
			d.FormImage = d.FormImage || f.Upload == UploadImage
			d.FormFiles = d.FormFiles || f.Upload == UploadFiles
		}
	}
}

// sampleAttachmentURL is accepted by attachment.ValidURL.
const sampleAttachmentURL = "/api/v1/files/a/2026/01/02/0123456789abcdef0123456789abcdef.png"
