package storage

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/config"
)

// fakeS3 is a minimal in-memory S3 endpoint (path-style, one bucket) that
// understands single PUT, multipart upload, HEAD, GET, DELETE.
type fakeS3 struct {
	mu       sync.Mutex
	bucket   string
	objects  map[string][]byte
	uploads  map[string]map[int][]byte
	aborted  int
	failPart bool
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	prefix := "/" + f.bucket + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeS3Error(w, http.StatusNotFound, "NoSuchBucket")
		return
	}
	key := strings.TrimPrefix(r.URL.Path, prefix)
	q := r.URL.Query()
	body, _ := io.ReadAll(r.Body)
	switch {
	case r.Method == http.MethodPost && q.Has("uploads"):
		id := strconv.Itoa(len(f.uploads) + 1)
		f.uploads[id] = map[int][]byte{}
		fmt.Fprintf(w, `<InitiateMultipartUploadResult><Bucket>%s</Bucket><Key>%s</Key><UploadId>%s</UploadId></InitiateMultipartUploadResult>`, f.bucket, key, id)
	case r.Method == http.MethodPut && q.Has("uploadId"):
		if f.failPart {
			writeS3Error(w, http.StatusInternalServerError, "InternalError")
			return
		}
		n, _ := strconv.Atoi(q.Get("partNumber"))
		f.uploads[q.Get("uploadId")][n] = body
		w.Header().Set("ETag", fmt.Sprintf(`"part%d"`, n))
	case r.Method == http.MethodPost && q.Has("uploadId"):
		parts := f.uploads[q.Get("uploadId")]
		nums := make([]int, 0, len(parts))
		for n := range parts {
			nums = append(nums, n)
		}
		sort.Ints(nums)
		var all bytes.Buffer
		for _, n := range nums {
			all.Write(parts[n])
		}
		f.objects[key] = all.Bytes()
		delete(f.uploads, q.Get("uploadId"))
		fmt.Fprintf(w, `<CompleteMultipartUploadResult><Bucket>%s</Bucket><Key>%s</Key><ETag>"done"</ETag></CompleteMultipartUploadResult>`, f.bucket, key)
	case r.Method == http.MethodDelete && q.Has("uploadId"):
		f.aborted++
		delete(f.uploads, q.Get("uploadId"))
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPut:
		f.objects[key] = body
		w.Header().Set("ETag", `"etag"`)
	case r.Method == http.MethodHead || r.Method == http.MethodGet:
		f.serveObject(w, r, key)
	case r.Method == http.MethodDelete:
		if _, ok := f.objects[key]; !ok {
			writeS3Error(w, http.StatusNotFound, "NoSuchKey")
			return
		}
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeS3Error(w, http.StatusNotImplemented, "NotImplemented")
	}
}

func (f *fakeS3) serveObject(w http.ResponseWriter, r *http.Request, key string) {
	data, ok := f.objects[key]
	if !ok {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeS3Error(w, http.StatusNotFound, "NoSuchKey")
		return
	}
	w.Header().Set("ETag", `"etag"`)
	w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	http.ServeContent(w, r, "", time.Now(), bytes.NewReader(data))
}

func writeS3Error(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<Error><Code>%s</Code><Message>%s</Message></Error>`, code, code)
}

func newFakeS3(t *testing.T) (*S3, *fakeS3) {
	t.Helper()
	fake := &fakeS3{bucket: "bkt", objects: map[string][]byte{}, uploads: map[string]map[int][]byte{}}
	srv := httptest.NewTLSServer(fake)
	t.Cleanup(srv.Close)
	s, err := newS3(config.S3Config{
		Endpoint: strings.TrimPrefix(srv.URL, "https://"), Bucket: "bkt", Region: "us-east-1",
		UsePathStyle: true, UseSSL: true, AccessKey: "ak", SecretKey: "sk",
	}, srv.Client().Transport)
	require.NoError(t, err)
	return s, fake
}
