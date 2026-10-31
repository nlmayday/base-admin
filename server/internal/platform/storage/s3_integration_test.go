//go:build integration

package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/config"
)

// TestS3AgainstRealServer runs the driver contract against a real
// S3-compatible server. It is skipped unless ADMIN_TEST_S3_ENDPOINT is set:
//
//	ADMIN_TEST_S3_ENDPOINT    host:port, e.g. 127.0.0.1:9000 (MinIO)
//	ADMIN_TEST_S3_BUCKET      bucket name (default base-admin-test; created if missing)
//	ADMIN_TEST_S3_ACCESS_KEY  access key
//	ADMIN_TEST_S3_SECRET_KEY  secret key
//	ADMIN_TEST_S3_REGION      optional region
//	ADMIN_TEST_S3_USE_SSL     "true" for https (default false)
//	ADMIN_TEST_S3_PATH_STYLE  "false" for virtual-host addressing (default true)
func TestS3AgainstRealServer(t *testing.T) {
	endpoint := os.Getenv("ADMIN_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("ADMIN_TEST_S3_ENDPOINT is not set")
	}
	bucket := os.Getenv("ADMIN_TEST_S3_BUCKET")
	if bucket == "" {
		bucket = "base-admin-test"
	}
	s, err := NewS3(config.S3Config{
		Endpoint: endpoint, Bucket: bucket, Region: os.Getenv("ADMIN_TEST_S3_REGION"),
		UseSSL:       os.Getenv("ADMIN_TEST_S3_USE_SSL") == "true",
		UsePathStyle: os.Getenv("ADMIN_TEST_S3_PATH_STYLE") != "false",
		AccessKey:    os.Getenv("ADMIN_TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("ADMIN_TEST_S3_SECRET_KEY"),
	})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	exists, err := s.client.BucketExists(ctx, bucket)
	require.NoError(t, err)
	if !exists {
		require.NoError(t, s.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: os.Getenv("ADMIN_TEST_S3_REGION")}))
	}

	driverContract(t, s)

	key, err := NewKey(time.Now(), "bin")
	require.NoError(t, err)
	data := bytes.Repeat([]byte("0123456789abcdef"), (s3SinglePutLimit+(1<<20))/16)
	require.NoError(t, s.Put(ctx, key, bytes.NewReader(data), -1, "application/octet-stream"))
	t.Cleanup(func() { _ = s.Delete(context.Background(), key) })
	rc, obj, err := s.Open(ctx, key)
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()
	assert.Equal(t, int64(len(data)), obj.Size)
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, data, got, "multipart upload round-trips")
}
