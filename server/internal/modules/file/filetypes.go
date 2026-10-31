package file

import (
	"bytes"
	"net/http"
	"path"
	"slices"
	"strings"
)

// Kinds group files for filtering and statistics.
const (
	KindImage    = "image"
	KindDocument = "document"
	KindMedia    = "media"
	KindArchive  = "archive"
	KindOther    = "other"
)

var allKinds = []string{KindImage, KindDocument, KindMedia, KindArchive, KindOther}

// family decides which sniffed content types are acceptable for an
// extension.
type family int

const (
	famImage family = iota + 1
	famPDF
	famZip  // zip container: OOXML, ODF, zip
	famOLE  // legacy Office compound documents
	famText // plain text only, never HTML/XML
	famGzip
	famRar
	famAudio
	famVideo
	famBinary // unknown extension allowed by policy: opaque bytes or plain text
)

type fileType struct {
	family family
	kind   string
	mime   string
	// sniffed lists the exact http.DetectContentType results accepted; nil
	// means use the family default.
	sniffed []string
}

var knownTypes = map[string]fileType{
	"jpg":  {famImage, KindImage, "image/jpeg", []string{"image/jpeg"}},
	"jpeg": {famImage, KindImage, "image/jpeg", []string{"image/jpeg"}},
	"png":  {famImage, KindImage, "image/png", []string{"image/png"}},
	"gif":  {famImage, KindImage, "image/gif", []string{"image/gif"}},
	"webp": {famImage, KindImage, "image/webp", []string{"image/webp"}},
	"bmp":  {famImage, KindImage, "image/bmp", []string{"image/bmp"}},
	"ico":  {famImage, KindImage, "image/x-icon", []string{"image/x-icon"}},

	"pdf":  {famPDF, KindDocument, "application/pdf", nil},
	"docx": {famZip, KindDocument, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", nil},
	"xlsx": {famZip, KindDocument, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", nil},
	"pptx": {famZip, KindDocument, "application/vnd.openxmlformats-officedocument.presentationml.presentation", nil},
	"odt":  {famZip, KindDocument, "application/vnd.oasis.opendocument.text", nil},
	"ods":  {famZip, KindDocument, "application/vnd.oasis.opendocument.spreadsheet", nil},
	"odp":  {famZip, KindDocument, "application/vnd.oasis.opendocument.presentation", nil},
	"doc":  {famOLE, KindDocument, "application/msword", nil},
	"xls":  {famOLE, KindDocument, "application/vnd.ms-excel", nil},
	"ppt":  {famOLE, KindDocument, "application/vnd.ms-powerpoint", nil},
	"txt":  {famText, KindDocument, "text/plain; charset=utf-8", nil},
	"csv":  {famText, KindDocument, "text/csv; charset=utf-8", nil},
	"md":   {famText, KindDocument, "text/markdown; charset=utf-8", nil},
	"log":  {famText, KindDocument, "text/plain; charset=utf-8", nil},
	"json": {famText, KindDocument, "application/json", nil},

	"zip": {famZip, KindArchive, "application/zip", nil},
	"gz":  {famGzip, KindArchive, "application/gzip", nil},
	"rar": {famRar, KindArchive, "application/vnd.rar", nil},

	"mp3":  {famAudio, KindMedia, "audio/mpeg", []string{"audio/mpeg"}},
	"wav":  {famAudio, KindMedia, "audio/wav", []string{"audio/wave"}},
	"mp4":  {famVideo, KindMedia, "video/mp4", []string{"video/mp4"}},
	"webm": {famVideo, KindMedia, "video/webm", []string{"video/webm"}},
}

var familySniffed = map[family][]string{
	famPDF:    {"application/pdf"},
	famZip:    {"application/zip"},
	famOLE:    {"application/octet-stream"},
	famText:   {"text/plain; charset=utf-8", "text/plain; charset=utf-16be", "text/plain; charset=utf-16le"},
	famGzip:   {"application/x-gzip"},
	famRar:    {"application/x-rar-compressed"},
	famBinary: {"application/octet-stream", "text/plain; charset=utf-8"},
}

// blockedExts can never be uploaded, whatever the policy says: they render
// as active content in a browser or execute on a desktop.
var blockedExts = map[string]bool{
	"html": true, "htm": true, "xhtml": true, "shtml": true, "mht": true, "mhtml": true,
	"svg": true, "svgz": true, "xml": true, "xsl": true, "xslt": true,
	"js": true, "mjs": true, "cjs": true, "jsx": true, "ts": true, "wasm": true,
	"exe": true, "msi": true, "dll": true, "com": true, "scr": true, "sys": true, "cpl": true,
	"bat": true, "cmd": true, "sh": true, "bash": true, "zsh": true, "ps1": true, "psm1": true,
	"vbs": true, "vbe": true, "wsf": true, "wsh": true, "hta": true, "lnk": true, "reg": true,
	"jar": true, "class": true, "apk": true, "app": true, "dmg": true, "pkg": true, "deb": true, "rpm": true,
	"php": true, "phtml": true, "jsp": true, "jspx": true, "asp": true, "aspx": true, "cgi": true,
	"pl": true, "py": true, "rb": true, "swf": true,
}

// olePrefix is the magic number of OLE compound files (.doc/.xls/.ppt).
var olePrefix = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// defaultExts is the allow-list used when no policy is configured.
func defaultExts() []string {
	exts := make([]string, 0, len(knownTypes))
	for ext := range knownTypes {
		exts = append(exts, ext)
	}
	slices.Sort(exts)
	return exts
}

// extOf returns the lowercase extension of name without the dot.
func extOf(name string) string {
	return strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
}

// typeOf returns the rules for an extension; unknown extensions are opaque
// binaries.
func typeOf(ext string) fileType {
	if t, ok := knownTypes[ext]; ok {
		return t
	}
	return fileType{family: famBinary, kind: KindOther, mime: "application/octet-stream"}
}

// isInline reports whether a browser may render the content type in place.
func isInline(mime string) bool {
	return strings.HasPrefix(mime, "image/") || mime == "application/pdf"
}

// checkContent verifies that the first bytes of a file match its extension.
// It returns the content type to store.
func checkContent(ext string, head []byte) (string, bool) {
	t := typeOf(ext)
	sniffed := http.DetectContentType(head)
	accepted := t.sniffed
	if accepted == nil {
		accepted = familySniffed[t.family]
	}
	if !slices.Contains(accepted, sniffed) {
		return "", false
	}
	if t.family == famOLE && !bytes.HasPrefix(head, olePrefix) {
		return "", false
	}
	return t.mime, true
}
