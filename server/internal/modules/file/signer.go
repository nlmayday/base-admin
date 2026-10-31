package file

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Token errors.
var (
	errTokenInvalid = errors.New("file: invalid signed token")
	errTokenExpired = errors.New("file: signed token expired")
)

// tokenContext separates file-link signatures from any other use of the
// same key.
const tokenContext = "base-admin/file-link/v1"

// maxTokenLen bounds the work spent on garbage tokens.
const maxTokenLen = 128

// signer issues and checks download tokens "<id>.<expiry>.<mac>", where mac
// is HMAC-SHA256(key, context|id|expiry) in unpadded base64url.
type signer struct {
	key []byte
	now func() time.Time
}

func (s signer) sign(id int64, expires time.Time) string {
	payload := strconv.FormatInt(id, 10) + "." + strconv.FormatInt(expires.Unix(), 10)
	return payload + "." + base64.RawURLEncoding.EncodeToString(s.mac(payload))
}

func (s signer) mac(payload string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(tokenContext + "|" + payload))
	return m.Sum(nil)
}

// verify returns the file id and expiry of a well-formed, authentic,
// unexpired token. The MAC is checked (in constant time) before any field
// is trusted.
func (s signer) verify(token string) (int64, time.Time, error) {
	if len(token) > maxTokenLen {
		return 0, time.Time{}, errTokenInvalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, time.Time{}, errTokenInvalid
	}
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(got, s.mac(parts[0]+"."+parts[1])) {
		return 0, time.Time{}, errTokenInvalid
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, time.Time{}, errTokenInvalid
	}
	unix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, time.Time{}, errTokenInvalid
	}
	exp := time.Unix(unix, 0)
	if !s.now().Before(exp) {
		return 0, time.Time{}, errTokenExpired
	}
	return id, exp, nil
}
