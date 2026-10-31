package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/platform/httpx"
)

// CSRFHeader carries the double-submit token; CSRFCookie exposes it to JS.
const (
	CSRFHeader = "X-XSRF-TOKEN"
	CSRFCookie = "XSRF-TOKEN"
)

const sessionCtxKey = "auth.session"

// Guard authenticates requests and enforces access levels. It implements
// module.Guard.
type Guard struct {
	db             *gorm.DB
	cookieName     string
	sessions       *SessionStore
	versions       *Versions
	snapshots      *Snapshots
	logger         *zap.Logger
	allowedOrigins map[string]bool
	// passwordChangeRoutes stay usable while a password change is pending.
	passwordChangeRoutes map[string]bool
	now                  func() time.Time
}

// Require returns middleware enforcing level (design doc 4.2, 5.6, 6.5.2).
func (g *Guard) Require(level access.Level) gin.HandlerFunc {
	if !level.Valid() {
		return func(c *gin.Context) { httpx.Fail(c, errForbidden) }
	}
	if level.Kind() == access.KindPublic {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		a, err := g.authenticate(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		c.Request = c.Request.WithContext(actor.With(c.Request.Context(), a))
		if a.Snapshot.MustChangePassword && !g.passwordChangeRoutes[c.FullPath()] {
			httpx.Fail(c, errPasswordChange)
			return
		}
		if level.Kind() == access.KindPermission && !level.Allows(a.Snapshot.HasPermission) {
			httpx.Fail(c, errForbidden)
			return
		}
		c.Next()
	}
}

// currentSession returns the session from the cookie, reusing the lookup
// done by the CSRF middleware for the same request.
func (g *Guard) currentSession(c *gin.Context) (Session, bool, error) {
	if v, ok := c.Get(sessionCtxKey); ok {
		return v.(Session), true, nil
	}
	token, err := c.Cookie(g.cookieName)
	if err != nil || token == "" {
		return Session{}, false, nil
	}
	sess, ok, err := g.sessions.Get(c.Request.Context(), token)
	if err != nil || !ok {
		return Session{}, false, err
	}
	c.Set(sessionCtxKey, sess)
	return sess, true, nil
}

func (g *Guard) authenticate(c *gin.Context) (*actor.Actor, error) {
	ctx := c.Request.Context()
	sess, ok, err := g.currentSession(c)
	if err != nil {
		return nil, unavailable(err)
	}
	if !ok {
		return nil, errUnauthenticated
	}
	now := g.now()
	if !now.Before(sess.AbsoluteExpiresAt) {
		_ = g.sessions.Delete(ctx, sess)
		return nil, errUnauthenticated
	}
	snap, err := g.validate(ctx, sess)
	if err != nil {
		return nil, err
	}
	if err := g.sessions.Touch(ctx, sess, now); err != nil {
		g.logger.Warn("session touch failed", zap.Error(err))
	}
	return &actor.Actor{Snapshot: snap, SessionHash: sess.Hash, IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}, nil
}

// validate applies design doc 6.5.2: the session is valid iff its stamped
// generation is at least max(mirror, snapshot.SessionGen) and the user is
// enabled.
func (g *Guard) validate(ctx context.Context, sess Session) (*actor.Snapshot, error) {
	mirror, err := g.trustedGen(ctx, sess.UserID)
	if errors.Is(err, ErrUserNotFound) {
		return nil, errUnauthenticated
	}
	if err != nil {
		return nil, unavailable(err)
	}
	ver, err := g.versions.Current(ctx)
	if err != nil {
		return nil, unavailable(err)
	}
	snap, err := g.snapshots.Load(ctx, ver, sess.UserID)
	if errors.Is(err, ErrUserNotFound) {
		return nil, errUnauthenticated
	}
	if err != nil {
		return nil, unavailable(err)
	}
	if sess.Gen < max(mirror, snap.SessionGen) || snap.Status != actor.StatusEnabled {
		_ = g.sessions.Delete(ctx, sess)
		return nil, errUnauthenticated
	}
	return snap, nil
}

// trustedGen returns the mirrored generation, reloading it from PostgreSQL
// when missing (invariant I2: missing is never zero).
func (g *Guard) trustedGen(ctx context.Context, uid int64) (int64, error) {
	gen, found, err := g.sessions.GenMirror(ctx, uid)
	if err != nil || found {
		return gen, err
	}
	pgGen, err := loadSessionGen(ctx, g.db, uid)
	if err != nil {
		return 0, err
	}
	return g.sessions.RaiseGen(ctx, uid, pgGen)
}

func loadSessionGen(ctx context.Context, db *gorm.DB, uid int64) (int64, error) {
	var gen int64
	res := db.WithContext(ctx).Raw("SELECT session_gen FROM sys_users WHERE id = ? AND deleted_at IS NULL", uid).Scan(&gen)
	if res.Error != nil {
		return 0, fmt.Errorf("load session generation: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return 0, ErrUserNotFound
	}
	return gen, nil
}

// CSRF rejects state-changing requests from foreign origins and, when a
// session cookie is present, requires the session's double-submit token
// (design doc 6.4). Requests authenticated by an API key alone (an
// Authorization header and no session cookie, e.g. the AI agent API) are
// exempt: a browser cannot attach that header to a cross-site request, and
// without the cookie no session is at stake.
func (g *Guard) CSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		if g.keyOnly(c.Request) {
			c.Next()
			return
		}
		if !g.originAllowed(c.Request) {
			httpx.Fail(c, errCSRF)
			return
		}
		sess, ok, err := g.currentSession(c)
		if err != nil {
			httpx.Fail(c, unavailable(err))
			return
		}
		if ok && (sess.CSRF == "" || subtle.ConstantTimeCompare([]byte(c.GetHeader(CSRFHeader)), []byte(sess.CSRF)) != 1) {
			httpx.Fail(c, errCSRF)
			return
		}
		c.Next()
	}
}

// keyOnly reports whether the request carries a bearer token and no
// session cookie.
func (g *Guard) keyOnly(r *http.Request) bool {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return false
	}
	_, err := r.Cookie(g.cookieName)
	return err != nil
}

func (g *Guard) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		ref, err := url.Parse(r.Header.Get("Referer"))
		if err != nil || ref.Scheme == "" || ref.Host == "" {
			return false
		}
		origin = ref.Scheme + "://" + ref.Host
	}
	return g.allowedOrigins[origin]
}

func unavailable(err error) error { return errUnavailable.WithCause(err) }
