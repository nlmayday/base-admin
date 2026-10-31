package file

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxNameRunes bounds stored file names (sys_files.original_name).
const maxNameRunes = 255

// sanitizeName keeps only the last path element of a client-supplied file
// name, drops control and format characters (including bidi overrides that
// can disguise an extension) and trims it to maxNameRunes, keeping the
// extension. It returns "" when nothing usable remains.
func sanitizeName(raw string) string {
	if i := strings.LastIndexAny(raw, `/\`); i >= 0 {
		raw = raw[i+1:]
	}
	raw = strings.ToValidUTF8(raw, "")
	var b strings.Builder
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			continue
		}
		b.WriteRune(r)
	}
	name := strings.TrimSpace(b.String())
	name = strings.TrimRight(name, ". ")
	if name == "" || strings.Trim(name, ".") == "" {
		return ""
	}
	if utf8.RuneCountInString(name) > maxNameRunes {
		ext := extOf(name)
		keep := maxNameRunes
		if ext != "" {
			keep -= len(ext) + 1
		}
		name = string([]rune(name)[:keep])
		if ext != "" {
			name += "." + ext
		}
	}
	return name
}

// withExt returns name ending in ".ext": the stored extension decides the
// content type, so renames cannot change it.
func withExt(name, ext string) string {
	if ext == "" || extOf(name) == ext {
		return name
	}
	return name + "." + ext
}

// contentDisposition builds an RFC 6266 header with an ASCII fallback
// filename and the UTF-8 name in RFC 5987 filename*.
func contentDisposition(inline bool, name string) string {
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	return disposition + `; filename="` + asciiFallback(name) + `"; filename*=UTF-8''` + rfc5987(name)
}

// asciiFallback replaces everything outside printable ASCII, and quote and
// backslash characters, with "_".
func asciiFallback(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// rfc5987 percent-encodes everything except attr-char (RFC 5987 3.2.1).
func rfc5987(s string) string {
	const attrChars = "!#$&+-.^_`|~"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < utf8.RuneSelf && (unicode.IsLetter(rune(c)) || unicode.IsDigit(rune(c)) || strings.IndexByte(attrChars, c) >= 0) {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}
