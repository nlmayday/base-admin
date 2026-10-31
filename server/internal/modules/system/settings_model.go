package system

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"base-admin/server/internal/platform/httpx"
)

// Built-in setting keys, readable by other modules through GetString/GetInt.
const (
	KeySiteName                = "site.name"
	KeySiteLogoURL             = "site.logo_url"
	KeyUploadAllowedExtensions = "upload.allowed_extensions"
	KeyUploadMaxSizeMB         = "upload.max_size_mb"
	KeyAuditRetentionDays      = "audit.retention_days"
)

// Setting limits.
const (
	maxSiteNameLen      = 64
	maxLogoURLLen       = 512
	maxExtensions       = 100
	minUploadMaxSizeMB  = 1
	maxUploadMaxSizeMB  = 1024
	minRetentionDays    = 7
	maxRetentionDays    = 3650
	bytesPerMB          = 1 << 20
	extensionListSep    = ","
	defaultExtensionCSV = "jpg,jpeg,png,gif,webp,pdf,docx,xlsx,pptx,zip,txt,csv,md"
)

type builtinSetting struct {
	Key, Value, ValueType, Group, Name, Description string
}

// builtinSettings are seeded once and then owned by administrators.
var builtinSettings = []builtinSetting{
	{KeySiteName, "Base Admin", TypeString, "站点设置", "站点名称", "显示在登录页和浏览器标题"},
	{KeySiteLogoURL, "", TypeString, "站点设置", "站点 Logo", "http(s) 地址或站内路径，留空使用默认 Logo"},
	{KeyUploadAllowedExtensions, defaultExtensionCSV, TypeString, "上传设置", "允许的文件扩展名", "逗号分隔的小写扩展名"},
	{KeyUploadMaxSizeMB, "20", TypeNumber, "上传设置", "单文件大小上限（MB）", "1–1024"},
	{KeyAuditRetentionDays, "180", TypeNumber, "日志设置", "日志保留天数", "超过天数的登录日志与操作日志会被清理"},
}

func settingKeys() []string {
	keys := make([]string, len(builtinSettings))
	for i, s := range builtinSettings {
		keys[i] = s.Key
	}
	return keys
}

// deniedExtensions can never be allowed for upload: they execute or render
// as active content in a browser or on a desktop.
var deniedExtensions = map[string]bool{
	"html": true, "htm": true, "xhtml": true, "shtml": true, "svg": true, "svgz": true, "xml": true,
	"js": true, "mjs": true, "cjs": true, "exe": true, "msi": true, "dll": true, "com": true, "scr": true,
	"bat": true, "cmd": true, "sh": true, "ps1": true, "vbs": true, "jar": true, "hta": true,
	"php": true, "jsp": true, "asp": true, "aspx": true,
}

var extensionPattern = regexp.MustCompile(`^[a-z0-9]{1,16}$`)

// Settings is the typed view of the built-in settings.
type Settings struct {
	SiteName                string   `json:"siteName"`
	LogoURL                 string   `json:"logoUrl"`
	UploadAllowedExtensions []string `json:"uploadAllowedExtensions"`
	UploadMaxSizeMB         int      `json:"uploadMaxSizeMB"`
	AuditRetentionDays      int      `json:"auditRetentionDays"`
}

// normalized returns a cleaned copy of s, or the invalid fields.
func (s Settings) normalized() (Settings, []httpx.FieldError) {
	var fields []httpx.FieldError
	add := func(field, msg string) { fields = append(fields, httpx.FieldError{Field: field, Message: msg}) }
	out := Settings{
		SiteName:           strings.TrimSpace(s.SiteName),
		LogoURL:            strings.TrimSpace(s.LogoURL),
		UploadMaxSizeMB:    s.UploadMaxSizeMB,
		AuditRetentionDays: s.AuditRetentionDays,
	}
	if n := utf8.RuneCountInString(out.SiteName); n == 0 || n > maxSiteNameLen {
		add("siteName", fmt.Sprintf("长度须为 1–%d 个字符", maxSiteNameLen))
	}
	if msg := validateLogoURL(out.LogoURL); msg != "" {
		add("logoUrl", msg)
	}
	exts, msg := normalizeExtensions(s.UploadAllowedExtensions)
	if msg != "" {
		add("uploadAllowedExtensions", msg)
	}
	out.UploadAllowedExtensions = exts
	if out.UploadMaxSizeMB < minUploadMaxSizeMB || out.UploadMaxSizeMB > maxUploadMaxSizeMB {
		add("uploadMaxSizeMB", fmt.Sprintf("须在 %d 到 %d 之间", minUploadMaxSizeMB, maxUploadMaxSizeMB))
	}
	if out.AuditRetentionDays < minRetentionDays || out.AuditRetentionDays > maxRetentionDays {
		add("auditRetentionDays", fmt.Sprintf("须在 %d 到 %d 之间", minRetentionDays, maxRetentionDays))
	}
	return out, fields
}

// validateLogoURL accepts "", an absolute http(s) URL or a site-relative
// path ("/..."), never a protocol-relative or script URL.
func validateLogoURL(v string) string {
	if v == "" {
		return ""
	}
	if utf8.RuneCountInString(v) > maxLogoURLLen {
		return fmt.Sprintf("不能多于 %d 个字符", maxLogoURLLen)
	}
	if strings.ContainsFunc(v, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || r == '\\' }) {
		return "地址不能包含空白或反斜杠"
	}
	if strings.HasPrefix(v, "/") {
		if strings.HasPrefix(v, "//") {
			return "须为 http(s) 地址或以 / 开头的站内路径"
		}
		return ""
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "须为 http(s) 地址或以 / 开头的站内路径"
	}
	return ""
}

// normalizeExtensions lowercases, strips a leading dot, drops blanks and
// duplicates, and rejects invalid or dangerous extensions.
func normalizeExtensions(in []string) ([]string, string) {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		ext := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(raw), "."))
		if ext == "" || slices.Contains(out, ext) {
			continue
		}
		if !extensionPattern.MatchString(ext) {
			return nil, fmt.Sprintf("扩展名「%s」无效，只能包含小写字母和数字", ext)
		}
		if deniedExtensions[ext] {
			return nil, fmt.Sprintf("出于安全考虑，不允许上传 .%s 文件", ext)
		}
		out = append(out, ext)
	}
	if len(out) == 0 {
		return nil, "至少需要一个扩展名"
	}
	if len(out) > maxExtensions {
		return nil, fmt.Sprintf("不能多于 %d 项", maxExtensions)
	}
	return out, ""
}

// storedValues maps a normalized Settings to config values.
func (s Settings) storedValues() map[string]string {
	return map[string]string{
		KeySiteName:                s.SiteName,
		KeySiteLogoURL:             s.LogoURL,
		KeyUploadAllowedExtensions: strings.Join(s.UploadAllowedExtensions, extensionListSep),
		KeyUploadMaxSizeMB:         strconv.Itoa(s.UploadMaxSizeMB),
		KeyAuditRetentionDays:      strconv.Itoa(s.AuditRetentionDays),
	}
}

// settingsFromValues builds Settings from stored config values. Stored
// values are re-filtered so that a hand-edited row cannot re-enable a
// dangerous extension.
func settingsFromValues(values map[string]string) (Settings, error) {
	maxMB, err := parseInt(values, KeyUploadMaxSizeMB)
	if err != nil {
		return Settings{}, err
	}
	days, err := parseInt(values, KeyAuditRetentionDays)
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		SiteName:                values[KeySiteName],
		LogoURL:                 values[KeySiteLogoURL],
		UploadAllowedExtensions: safeExtensions(values[KeyUploadAllowedExtensions]),
		UploadMaxSizeMB:         maxMB,
		AuditRetentionDays:      days,
	}, nil
}

// safeExtensions parses a stored extension list, keeping only valid and
// allowed entries.
func safeExtensions(csv string) []string {
	out := []string{}
	for _, raw := range strings.Split(csv, extensionListSep) {
		ext := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(raw), "."))
		if extensionPattern.MatchString(ext) && !deniedExtensions[ext] && !slices.Contains(out, ext) {
			out = append(out, ext)
		}
	}
	return out
}

func parseInt(values map[string]string, key string) (int, error) {
	raw, ok := values[key]
	if !ok {
		return 0, fmt.Errorf("setting %s is missing", key)
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("setting %s is not an integer: %w", key, err)
	}
	return n, nil
}
