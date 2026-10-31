package file

import (
	"encoding/base64"
	"hash"
)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// nopHash satisfies hash.Hash for reader tests.
type nopHash struct{}

func (nopHash) Write(p []byte) (int, error) { return len(p), nil }
func (nopHash) Sum(b []byte) []byte         { return b }
func (nopHash) Reset()                      {}
func (nopHash) Size() int                   { return 0 }
func (nopHash) BlockSize() int              { return 1 }

var _ hash.Hash = nopHash{}
