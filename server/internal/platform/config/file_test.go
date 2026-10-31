package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSigningKey = "0123456789abcdef0123456789abcdef"

func TestFileDefaults(t *testing.T) {
	cfg := validConfig(t)
	assert.Equal(t, FileDriverLocal, cfg.File.Driver)
	assert.Equal(t, "data/uploads", cfg.File.LocalDir)
	assert.Equal(t, 10*time.Minute, cfg.File.SignedURLTTL)
	assert.Equal(t, int64(100<<20), cfg.File.MaxUploadBytes)
	assert.True(t, cfg.File.S3.UseSSL)
	assert.GreaterOrEqual(t, len(cfg.File.SigningKey), MinSigningKeyBytes, "dev must generate a signing key")

	other := validConfig(t)
	assert.NotEqual(t, cfg.File.SigningKey, other.File.SigningKey, "generated keys must be random")
}

func TestFileEnvOverrides(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
	t.Setenv("ADMIN_FILE_DRIVER", "s3")
	t.Setenv("ADMIN_FILE_S3_ENDPOINT", "minio.local:9000")
	t.Setenv("ADMIN_FILE_S3_BUCKET", "uploads")
	t.Setenv("ADMIN_FILE_S3_USE_PATH_STYLE", "true")
	t.Setenv("ADMIN_FILE_S3_ACCESS_KEY", "ak")
	t.Setenv("ADMIN_FILE_S3_SECRET_KEY", "sk")
	t.Setenv("ADMIN_FILE_SIGNING_KEY", testSigningKey)

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, FileDriverS3, cfg.File.Driver)
	assert.Equal(t, "minio.local:9000", cfg.File.S3.Endpoint)
	assert.Equal(t, "uploads", cfg.File.S3.Bucket)
	assert.True(t, cfg.File.S3.UsePathStyle)
	assert.Equal(t, "ak", cfg.File.S3.AccessKey)
	assert.Equal(t, "sk", cfg.File.S3.SecretKey)
	assert.Equal(t, testSigningKey, cfg.File.SigningKey, "a configured key must not be replaced")
}

func TestFileSecretsRejectedInFile(t *testing.T) {
	cases := map[string]string{
		"access key":  "file:\n  s3:\n    access_key: AKIASECRET\n",
		"secret key":  "file:\n  s3:\n    secret_key: AKIASECRET\n",
		"signing key": "file:\n  signing_key: AKIASECRET\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
			_, err := Load(writeFile(t, content))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must not be set in the config file")
			assert.Contains(t, err.Error(), "ADMIN_FILE_")
			assert.NotContains(t, err.Error(), "AKIASECRET")
		})
	}
}

func TestProdRequiresSigningKey(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
	t.Setenv("ADMIN_APP_ENV", "prod")
	_, err := Load("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file.signing_key is required in prod")
}

func TestValidateFile(t *testing.T) {
	s3 := func(c *Config) {
		c.File.Driver = FileDriverS3
		c.File.S3 = S3Config{Endpoint: "s3.example.com", Bucket: "b"}
	}
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"unknown driver", func(c *Config) { c.File.Driver = "ftp" }, "file.driver must be local or s3"},
		{"empty local dir", func(c *Config) { c.File.LocalDir = " " }, "file.local_dir is required"},
		{"s3 without endpoint", func(c *Config) { s3(c); c.File.S3.Endpoint = "" }, "file.s3.endpoint is required"},
		{"s3 endpoint with scheme", func(c *Config) { s3(c); c.File.S3.Endpoint = "https://s3.example.com" }, "without a scheme"},
		{"s3 endpoint with path", func(c *Config) { s3(c); c.File.S3.Endpoint = "s3.example.com/x" }, "without a scheme"},
		{"s3 without bucket", func(c *Config) { s3(c); c.File.S3.Bucket = "" }, "file.s3.bucket is required"},
		{"s3 half credentials", func(c *Config) { s3(c); c.File.S3.AccessKey = "ak" }, "must be set together"},
		{"zero ttl", func(c *Config) { c.File.SignedURLTTL = 0 }, "file.signed_url_ttl"},
		{"ttl too long", func(c *Config) { c.File.SignedURLTTL = 8 * 24 * time.Hour }, "file.signed_url_ttl"},
		{"zero ceiling", func(c *Config) { c.File.MaxUploadBytes = 0 }, "file.max_upload_bytes"},
		{"ceiling too large", func(c *Config) { c.File.MaxUploadBytes = MaxUploadCeiling + 1 }, "file.max_upload_bytes"},
		{"short signing key", func(c *Config) { c.File.SigningKey = "short" }, "at least 32 bytes"},
		{"prod without key", func(c *Config) { c.App.Env = EnvProd; c.File.SigningKey = "" }, "required in prod"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig(t)
			tc.mutate(&cfg)
			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidateFileAcceptsS3(t *testing.T) {
	cfg := validConfig(t)
	cfg.File.Driver = FileDriverS3
	cfg.File.S3 = S3Config{Endpoint: "127.0.0.1:9000", Bucket: "b", AccessKey: "a", SecretKey: "s"}
	assert.NoError(t, cfg.Validate())
	cfg.File.S3.AccessKey, cfg.File.S3.SecretKey = "", ""
	assert.NoError(t, cfg.Validate(), "ambient credentials are allowed")
}

func TestDevSigningKeyNotGeneratedInProd(t *testing.T) {
	f := FileConfig{}.withDevSigningKey(EnvProd)
	assert.Empty(t, f.SigningKey)
	f = FileConfig{}.withDevSigningKey(EnvTest)
	assert.Len(t, f.SigningKey, 2*devSigningKeyBytes)
	assert.False(t, strings.ContainsAny(f.SigningKey, " \n"))
}
