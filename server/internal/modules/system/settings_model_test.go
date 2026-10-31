package system

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/httpx"
)

func validSettings() Settings {
	return Settings{SiteName: "Admin", LogoURL: "", UploadAllowedExtensions: []string{"png"},
		UploadMaxSizeMB: 20, AuditRetentionDays: 180}
}

func fieldNames(fields []httpx.FieldError) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = f.Field
	}
	return out
}

func TestSettingsNormalizedValid(t *testing.T) {
	in := Settings{
		SiteName: "  My Admin ", LogoURL: " https://cdn.example.com/logo.png ",
		UploadAllowedExtensions: []string{" .PNG", "jpg", "png", "", "Pdf"},
		UploadMaxSizeMB:         1024, AuditRetentionDays: 7,
	}
	out, fields := in.normalized()
	require.Empty(t, fields)
	assert.Equal(t, "My Admin", out.SiteName)
	assert.Equal(t, "https://cdn.example.com/logo.png", out.LogoURL)
	assert.Equal(t, []string{"png", "jpg", "pdf"}, out.UploadAllowedExtensions)
	assert.Equal(t, " .PNG", in.UploadAllowedExtensions[0], "input must not be mutated")
}

func TestSettingsNormalizedInvalid(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Settings)
		field  string
	}{
		{"empty site name", func(s *Settings) { s.SiteName = "   " }, "siteName"},
		{"long site name", func(s *Settings) { s.SiteName = strings.Repeat("名", maxSiteNameLen+1) }, "siteName"},
		{"javascript logo", func(s *Settings) { s.LogoURL = "javascript:alert(1)" }, "logoUrl"},
		{"data logo", func(s *Settings) { s.LogoURL = "data:image/png;base64,AAA" }, "logoUrl"},
		{"protocol relative logo", func(s *Settings) { s.LogoURL = "//evil.com/x.png" }, "logoUrl"},
		{"backslash logo", func(s *Settings) { s.LogoURL = `/\evil.com` }, "logoUrl"},
		{"relative without slash", func(s *Settings) { s.LogoURL = "logo.png" }, "logoUrl"},
		{"no extensions", func(s *Settings) { s.UploadAllowedExtensions = []string{" ", ""} }, "uploadAllowedExtensions"},
		{"html", func(s *Settings) { s.UploadAllowedExtensions = []string{"png", "html"} }, "uploadAllowedExtensions"},
		{"HTM upper", func(s *Settings) { s.UploadAllowedExtensions = []string{"HTM"} }, "uploadAllowedExtensions"},
		{"svg", func(s *Settings) { s.UploadAllowedExtensions = []string{".svg"} }, "uploadAllowedExtensions"},
		{"js", func(s *Settings) { s.UploadAllowedExtensions = []string{"js"} }, "uploadAllowedExtensions"},
		{"exe", func(s *Settings) { s.UploadAllowedExtensions = []string{"exe"} }, "uploadAllowedExtensions"},
		{"punctuation", func(s *Settings) { s.UploadAllowedExtensions = []string{"tar.gz"} }, "uploadAllowedExtensions"},
		{"size zero", func(s *Settings) { s.UploadMaxSizeMB = 0 }, "uploadMaxSizeMB"},
		{"size too big", func(s *Settings) { s.UploadMaxSizeMB = 1025 }, "uploadMaxSizeMB"},
		{"retention zero", func(s *Settings) { s.AuditRetentionDays = 0 }, "auditRetentionDays"},
		{"retention too big", func(s *Settings) { s.AuditRetentionDays = maxRetentionDays + 1 }, "auditRetentionDays"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validSettings()
			tc.mutate(&s)
			_, fields := s.normalized()
			assert.Equal(t, []string{tc.field}, fieldNames(fields))
		})
	}
}

func TestTooManyExtensions(t *testing.T) {
	exts := make([]string, maxExtensions+1)
	for i := range exts {
		exts[i] = "e" + strings.Repeat("a", i%10) + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	_, msg := normalizeExtensions(exts)
	assert.NotEmpty(t, msg)
}

func TestValidateLogoURL(t *testing.T) {
	for _, ok := range []string{"", "/static/logo.png", "http://a.com/x.png", "https://a.com:8443/x?y=1"} {
		assert.Empty(t, validateLogoURL(ok), ok)
	}
	for _, bad := range []string{"ftp://a.com/x", "https://", "https://user:pw@a.com/x", "/a b", "//a.com",
		"https://a.com/\n", "/" + strings.Repeat("a", maxLogoURLLen)} {
		assert.NotEmpty(t, validateLogoURL(bad), bad)
	}
}

func TestStoredValuesRoundTrip(t *testing.T) {
	in, fields := Settings{SiteName: "X", LogoURL: "/l.png", UploadAllowedExtensions: []string{"png", "zip"},
		UploadMaxSizeMB: 5, AuditRetentionDays: 30}.normalized()
	require.Empty(t, fields)
	values := in.storedValues()
	assert.Equal(t, "png,zip", values[KeyUploadAllowedExtensions])
	assert.Equal(t, "5", values[KeyUploadMaxSizeMB])
	assert.Len(t, values, len(builtinSettings))
	out, err := settingsFromValues(values)
	require.NoError(t, err)
	assert.Equal(t, in, out)
}

func TestSettingsFromValuesFiltersDangerousExtensions(t *testing.T) {
	values := Settings{SiteName: "X", UploadAllowedExtensions: []string{"png"}, UploadMaxSizeMB: 1, AuditRetentionDays: 1}.storedValues()
	values[KeyUploadAllowedExtensions] = "png, HTML ,svg,.PDF,bad-ext,png"
	out, err := settingsFromValues(values)
	require.NoError(t, err)
	assert.Equal(t, []string{"png", "pdf"}, out.UploadAllowedExtensions)
}

func TestSettingsFromValuesErrors(t *testing.T) {
	values := validSettings().storedValues()
	delete(values, KeyUploadMaxSizeMB)
	_, err := settingsFromValues(values)
	assert.Error(t, err)

	values = validSettings().storedValues()
	values[KeyAuditRetentionDays] = "abc"
	_, err = settingsFromValues(values)
	assert.Error(t, err)
}

func TestBuiltinDefaultsAreValid(t *testing.T) {
	values := map[string]string{}
	for _, s := range builtinSettings {
		values[s.Key] = s.Value
		_, msg := normalizeValue(s.ValueType, s.Value)
		assert.Empty(t, msg, s.Key)
		assert.True(t, validConfigKey(s.Key), s.Key)
	}
	s, err := settingsFromValues(values)
	require.NoError(t, err)
	_, fields := s.normalized()
	assert.Empty(t, fields)
	assert.Equal(t, strings.Split(defaultExtensionCSV, ","), s.UploadAllowedExtensions)
}
