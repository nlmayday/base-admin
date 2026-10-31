package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"base-admin/server/internal/platform/config"
)

const (
	// s3PartSize bounds memory per streaming upload of unknown size; with
	// 10000 parts it allows objects up to 160 GiB.
	s3PartSize = 16 << 20
	// s3SinglePutLimit: bodies up to this size are buffered and sent in one
	// PUT instead of a multipart upload.
	s3SinglePutLimit = 8 << 20
)

// S3 stores objects in an S3-compatible bucket (AWS S3, MinIO, Aliyun OSS,
// Tencent COS...).
type S3 struct {
	client *minio.Client
	bucket string
}

var _ Storage = (*S3)(nil)

// NewS3 creates the client. It does not contact the server.
func NewS3(cfg config.S3Config) (*S3, error) {
	return newS3(cfg, nil)
}

func newS3(cfg config.S3Config, transport http.RoundTripper) (*S3, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		return nil, errors.New("storage: s3 endpoint and bucket are required")
	}
	lookup := minio.BucketLookupAuto
	if cfg.UsePathStyle {
		lookup = minio.BucketLookupPath
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:        s3Credentials(cfg),
		Secure:       cfg.UseSSL,
		Region:       cfg.Region,
		BucketLookup: lookup,
		Transport:    transport,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: s3 client: %w", err)
	}
	return &S3{client: client, bucket: cfg.Bucket}, nil
}

// s3Credentials uses the configured static keys, or else the standard
// environment variables and the instance role.
func s3Credentials(cfg config.S3Config) *credentials.Credentials {
	if cfg.AccessKey != "" {
		return credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, "")
	}
	return credentials.NewChainCredentials([]credentials.Provider{
		&credentials.EnvAWS{},
		&credentials.EnvMinio{},
		&credentials.IAM{},
	})
}

// Name implements Storage.
func (s *S3) Name() string { return "s3" }

// Bucket returns the bucket name.
func (s *S3) Bucket() string { return s.bucket }

// Put implements Storage. Small bodies of unknown size are buffered and
// uploaded in one request; larger ones stream as a multipart upload, which
// minio-go aborts on error.
func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	opts := minio.PutObjectOptions{ContentType: contentType, PartSize: s3PartSize}
	if size < 0 {
		// Read up to the single-request limit into a buffer that grows with
		// the data, so a small upload does not cost s3SinglePutLimit bytes.
		var head bytes.Buffer
		n, err := head.ReadFrom(io.LimitReader(r, s3SinglePutLimit))
		switch {
		case err != nil:
			return fmt.Errorf("storage: read: %w", err)
		case n < s3SinglePutLimit:
			r, size = bytes.NewReader(head.Bytes()), n
		default:
			r = io.MultiReader(bytes.NewReader(head.Bytes()), r)
		}
	}
	if _, err := s.client.PutObject(ctx, s.bucket, key, r, size, opts); err != nil {
		return fmt.Errorf("storage: s3 put: %w", err)
	}
	return nil
}

// Open implements Storage. *minio.Object is an io.ReadSeeker.
func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, Object, error) {
	if err := ValidateKey(key); err != nil {
		return nil, Object{}, err
	}
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, Object{}, mapS3Error(err)
	}
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, Object{}, mapS3Error(err)
	}
	return obj, Object{Key: key, Size: info.Size, ModTime: info.LastModified}, nil
}

// Stat implements Storage.
func (s *S3) Stat(ctx context.Context, key string) (Object, error) {
	if err := ValidateKey(key); err != nil {
		return Object{}, err
	}
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return Object{}, mapS3Error(err)
	}
	return Object{Key: key, Size: info.Size, ModTime: info.LastModified}, nil
}

// Delete implements Storage. S3 deletes are idempotent.
func (s *S3) Delete(ctx context.Context, key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		if errors.Is(mapS3Error(err), ErrNotFound) {
			return nil
		}
		return fmt.Errorf("storage: s3 delete: %w", err)
	}
	return nil
}

func mapS3Error(err error) error {
	resp := minio.ToErrorResponse(err)
	if resp.Code == minio.NoSuchKey {
		return ErrNotFound
	}
	return fmt.Errorf("storage: s3: %w", err)
}
