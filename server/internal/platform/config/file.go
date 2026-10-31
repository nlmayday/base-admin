package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// File storage drivers.
const (
	FileDriverLocal = "local"
	FileDriverS3    = "s3"
)

// File limits.
const (
	// MinSigningKeyBytes is the minimum length of file.signing_key.
	MinSigningKeyBytes = 32
	// MaxUploadCeiling bounds file.max_upload_bytes (5 GiB).
	MaxUploadCeiling int64 = 5 << 30
	// MaxSignedURLTTL bounds file.signed_url_ttl.
	MaxSignedURLTTL    = 7 * 24 * time.Hour
	devSigningKeyBytes = 32
)

// FileConfig holds file storage settings (docs/p1-p3-plan.md, file).
type FileConfig struct {
	// Driver is "local" or "s3".
	Driver string `mapstructure:"driver"`
	// LocalDir is the base directory of the local driver.
	LocalDir string   `mapstructure:"local_dir"`
	S3       S3Config `mapstructure:"s3"`
	// SignedURLTTL is how long a signed download link stays valid.
	SignedURLTTL time.Duration `mapstructure:"signed_url_ttl"`
	// MaxUploadBytes is the hard ceiling for one upload; the runtime upload
	// policy can only lower it.
	MaxUploadBytes int64 `mapstructure:"max_upload_bytes"`
	// SigningKey signs download links. It is a secret and must come from
	// ADMIN_FILE_SIGNING_KEY; outside prod a random key is generated when it
	// is empty, so links do not survive a restart.
	SigningKey string `mapstructure:"signing_key"`
}

// S3Config holds the settings of an S3-compatible object store (AWS S3,
// MinIO, Aliyun OSS, Tencent COS...).
type S3Config struct {
	// Endpoint is host[:port] without a scheme, e.g. "s3.amazonaws.com".
	Endpoint string `mapstructure:"endpoint"`
	Region   string `mapstructure:"region"`
	Bucket   string `mapstructure:"bucket"`
	// UsePathStyle forces path-style bucket addressing (needed by MinIO);
	// otherwise the client chooses automatically.
	UsePathStyle bool `mapstructure:"use_path_style"`
	UseSSL       bool `mapstructure:"use_ssl"`
	// AccessKey and SecretKey are secrets and must come from
	// ADMIN_FILE_S3_ACCESS_KEY and ADMIN_FILE_S3_SECRET_KEY. When both are
	// empty, credentials come from the standard AWS/MinIO environment
	// variables or the instance role.
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
}

func fileDefaults() map[string]any {
	return map[string]any{
		"file.driver":           FileDriverLocal,
		"file.local_dir":        "data/uploads",
		"file.signed_url_ttl":   10 * time.Minute,
		"file.max_upload_bytes": int64(100 << 20),
		"file.signing_key":      "",

		"file.s3.endpoint":       "",
		"file.s3.region":         "",
		"file.s3.bucket":         "",
		"file.s3.use_path_style": false,
		"file.s3.use_ssl":        true,
		"file.s3.access_key":     "",
		"file.s3.secret_key":     "",
	}
}

// withDevSigningKey returns f with a random signing key when none is set
// outside prod. Prod must configure one (see validate).
func (f FileConfig) withDevSigningKey(env Env) FileConfig {
	if f.SigningKey != "" || env == EnvProd {
		return f
	}
	b := make([]byte, devSigningKeyBytes)
	_, _ = rand.Read(b) // crypto/rand.Read never fails (Go 1.24+)
	f.SigningKey = hex.EncodeToString(b)
	return f
}

func (f FileConfig) validate(env Env) error {
	errs := []error{f.validateDriver()}
	if f.SignedURLTTL < time.Second || f.SignedURLTTL > MaxSignedURLTTL {
		errs = append(errs, fmt.Errorf("file.signed_url_ttl must be between 1s and %s", MaxSignedURLTTL))
	}
	if f.MaxUploadBytes <= 0 || f.MaxUploadBytes > MaxUploadCeiling {
		errs = append(errs, fmt.Errorf("file.max_upload_bytes must be between 1 and %d", MaxUploadCeiling))
	}
	switch {
	case f.SigningKey == "" && env == EnvProd:
		errs = append(errs, errors.New("file.signing_key is required in prod (set ADMIN_FILE_SIGNING_KEY)"))
	case f.SigningKey != "" && len(f.SigningKey) < MinSigningKeyBytes:
		errs = append(errs, fmt.Errorf("file.signing_key must be at least %d bytes", MinSigningKeyBytes))
	}
	return errors.Join(errs...)
}

func (f FileConfig) validateDriver() error {
	switch f.Driver {
	case FileDriverLocal:
		if strings.TrimSpace(f.LocalDir) == "" {
			return errors.New("file.local_dir is required for the local driver")
		}
		return nil
	case FileDriverS3:
		return f.S3.validate()
	default:
		return fmt.Errorf("file.driver must be local or s3; got %q", f.Driver)
	}
}

func (s S3Config) validate() error {
	var errs []error
	switch {
	case s.Endpoint == "":
		errs = append(errs, errors.New("file.s3.endpoint is required for the s3 driver"))
	case strings.Contains(s.Endpoint, "://") || strings.ContainsAny(s.Endpoint, "/?#@ "):
		errs = append(errs, fmt.Errorf("file.s3.endpoint must be host[:port] without a scheme or path; got %q", s.Endpoint))
	}
	if s.Bucket == "" {
		errs = append(errs, errors.New("file.s3.bucket is required for the s3 driver"))
	}
	if (s.AccessKey == "") != (s.SecretKey == "") {
		errs = append(errs, errors.New("file.s3.access_key and file.s3.secret_key must be set together (ADMIN_FILE_S3_ACCESS_KEY, ADMIN_FILE_S3_SECRET_KEY)"))
	}
	return errors.Join(errs...)
}
