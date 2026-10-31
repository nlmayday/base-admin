package httpx

import (
	"net/http"
	"time"
)

// deadlineGrace leaves time to write the error response after the request
// context expires.
const deadlineGrace = 10 * time.Second

// ExtendDeadlines lifts the connection's read and write deadlines (set by
// the server's read_timeout/write_timeout) for one long transfer. Call it
// only after the request is authorized.
func ExtendDeadlines(w http.ResponseWriter, d time.Duration) {
	rc := http.NewResponseController(w)
	// ErrNotSupported (e.g. httptest recorders) only means there is no
	// connection deadline to extend.
	_ = rc.SetReadDeadline(time.Now().Add(d))
	_ = rc.SetWriteDeadline(time.Now().Add(d + deadlineGrace))
}
