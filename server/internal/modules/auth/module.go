package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/core/access"
	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/module"
	"base-admin/server/internal/core/perm"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/requestid"
)

// MeFunc renders the current user (the actor in ctx) for responses. The IAM
// module provides it.
type MeFunc func(ctx context.Context) (any, error)

// Module exposes login, logout and password change.
type Module struct {
	core *Core
	me   MeFunc
	// canManage, when set, decides whether the caller may end a user's
	// sessions (the IAM anti-escalation rule); otherwise only super admins
	// are protected.
	canManage func(ctx context.Context, uid int64) error
}

// ModuleOption configures the auth module.
type ModuleOption func(*Module)

// WithTargetGuard lets another module (IAM) decide whether the caller may
// end a given user's sessions. auth cannot depend on IAM directly.
func WithTargetGuard(fn func(ctx context.Context, uid int64) error) ModuleOption {
	return func(m *Module) { m.canManage = fn }
}

var _ module.Module = (*Module)(nil)

// NewModule creates the auth module.
func NewModule(core *Core, me MeFunc, opts ...ModuleOption) *Module {
	m := &Module{core: core, me: me}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Name implements module.Module.
func (m *Module) Name() string { return "auth" }

// Permissions implements module.Module.
func (m *Module) Permissions() []perm.Def { return slices.Clone(securityPermissions) }

// Menus implements module.Module.
func (m *Module) Menus() []menu.Def { return slices.Clone(securityMenus) }

// Seed implements module.Module.
func (m *Module) Seed(context.Context, *gorm.DB) error { return nil }

// Routes implements module.Module.
func (m *Module) Routes(r *module.Router) {
	r.POST("/auth/login", access.Public(), m.login).Doc(module.Doc{Summary: "登录", Body: loginRequest{}})
	r.POST("/auth/logout", access.Authenticated(), m.logout).Doc(module.Doc{Summary: "退出登录"})
	r.PUT("/me/password", access.Authenticated(), m.changePassword).Doc(module.Doc{Summary: "修改密码", Body: changePasswordRequest{}})
	r.GET("/auth/captcha", access.Public(), handle(m.issueCaptcha)).Doc(module.Doc{Summary: "获取验证码", Response: captchaView{}})
	r.GET("/auth/login-options", access.Public(), handle(m.loginOptions)).Doc(module.Doc{Summary: "登录选项", Response: loginOptions{}})

	s := r.Group("/sessions")
	s.GET("", access.Perm("auth:session:view"), handle(m.listSessions)).
		Doc(module.Doc{Summary: "在线会话", Query: sessionFilter{}, Response: httpx.Page[sessionView]{}})
	s.DELETE("/:id", access.Perm("auth:session:revoke"), handle(m.revokeSession)).Doc(module.Doc{Summary: "强制下线会话"})
	s.DELETE("/users/:id", access.Perm("auth:session:revoke"), handle(m.revokeUserSessions)).
		Doc(module.Doc{Summary: "下线用户全部会话"})

	p := r.Group("/security")
	p.GET("/policy", access.Perm("auth:policy:view"), handle(m.getPolicy)).Doc(module.Doc{Summary: "安全策略", Response: policyView{}})
	p.PUT("/policy", access.Perm("auth:policy:update"), handle(m.updatePolicy)).
		Doc(module.Doc{Summary: "修改安全策略", Body: Policy{}, Response: policyView{}})
	p.GET("/roles", access.Perm("auth:policy:view"), handle(m.listPolicyRoles)).
		Doc(module.Doc{Summary: "可豁免登录限制的角色", Response: []policyRole{}})
	p.POST("/unlock", access.Perm("auth:policy:update"), handle(m.unlockAccount)).
		Doc(module.Doc{Summary: "解除账号锁定", Body: unlockRequest{}})
}

type loginRequest struct {
	Username    string `json:"username" binding:"required,max=64"`
	Password    string `json:"password" binding:"required,max=128"`
	CaptchaID   string `json:"captchaId" binding:"max=64"`
	CaptchaCode string `json:"captchaCode" binding:"max=16"`
}

func (m *Module) login(c *gin.Context) {
	var req loginRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx, ip := c.Request.Context(), c.ClientIP()
	account := strings.ToLower(strings.TrimSpace(req.Username))

	blocked, err := m.core.limiter.reserve(ctx, ip, account)
	if err != nil {
		m.recordLogin(c, account, 0, ReasonError)
		httpx.Fail(c, unavailable(err))
		return
	}
	if blocked {
		m.recordLogin(c, account, 0, ReasonRateLimited)
		httpx.Fail(c, httpx.TooManyRequests())
		return
	}

	policy, gateErr := m.loginGate(c, account, req)
	if gateErr != nil {
		httpx.Fail(c, gateErr)
		return
	}

	uid, hash, ok, err := m.core.checkCredentials(ctx, account, req.Password)
	if err != nil {
		m.recordLogin(c, account, 0, ReasonError)
		httpx.Fail(c, unavailable(err))
		return
	}
	if ok {
		if err := m.core.checkGuards(ctx, LoginAttempt{
			UserID: uid, Username: account, IP: ip, UserAgent: c.Request.UserAgent(), At: m.core.now(),
		}); err != nil {
			m.recordLogin(c, account, uid, ReasonDenied)
			httpx.Fail(c, err)
			return
		}
		token, sess, err := m.core.establishSession(ctx, uid, hash, ip, c.Request.UserAgent())
		switch {
		case err == nil:
			_ = m.core.lockout.clear(ctx, account, ip)
			m.recordLogin(c, account, uid, "")
			m.completeLogin(c, account, token, sess)
			return
		case !errors.Is(err, errAccountUnavailable):
			m.recordLogin(c, account, uid, ReasonError)
			httpx.Fail(c, err)
			return
		}
	}
	m.recordLogin(c, account, 0, ReasonBadCredentials)
	failures, err := m.core.lockout.recordFailure(ctx, account, ip, policy)
	if err != nil {
		m.core.logger.Warn("login failure not counted for lockout", zap.Error(err))
	}
	httpx.Fail(c, errBadCredentials.WithDetails(map[string]bool{
		"captchaRequired": captchaRequired(policy, failures),
	}))
}

// loginGate applies the login restrictions, the account lock and the
// captcha requirement before any password is checked (security policy).
func (m *Module) loginGate(c *gin.Context, account string, req loginRequest) (Policy, error) {
	ctx := c.Request.Context()
	policy, err := m.core.policies.get(ctx)
	if err != nil {
		return Policy{}, unavailable(err)
	}
	if policy.restricted() {
		roles, err := accountRoles(ctx, m.core.db, account)
		if err != nil {
			return Policy{}, unavailable(err)
		}
		if reason, err := policy.loginRefusal(c.ClientIP(), roles, m.core.now()); err != nil {
			m.recordLogin(c, account, 0, reason)
			return Policy{}, err
		}
	}
	lockedFor, err := m.core.lockout.locked(ctx, account, c.ClientIP())
	if err != nil {
		return Policy{}, unavailable(err)
	}
	if lockedFor > 0 {
		m.recordLogin(c, account, 0, ReasonLocked)
		minutes := int(lockedFor.Minutes()) + 1
		return Policy{}, httpx.NewError(http.StatusForbidden, CodeAccountLocked,
			fmt.Sprintf("账号已锁定，请 %d 分钟后再试", minutes)).WithDetails(map[string]int{"retryAfterSeconds": int(lockedFor.Seconds())})
	}
	failures, err := m.core.lockout.failures(ctx, account)
	if err != nil {
		return Policy{}, unavailable(err)
	}
	if captchaRequired(policy, failures) {
		ok, err := m.core.captchas.verify(ctx, req.CaptchaID, req.CaptchaCode)
		if err != nil {
			return Policy{}, unavailable(err)
		}
		if !ok {
			m.recordLogin(c, account, 0, ReasonCaptcha)
			return Policy{}, errCaptcha
		}
	}
	return policy, nil
}

// recordLogin reports a login attempt; an empty reason means success.
func (m *Module) recordLogin(c *gin.Context, account string, uid int64, reason string) {
	m.core.observer.RecordLogin(c.Request.Context(), LoginEvent{
		Kind: EventLogin, Username: account, UserID: uid, Success: reason == "", Reason: reason,
		IP: c.ClientIP(), UserAgent: truncate(c.Request.UserAgent(), 256),
		RequestID: requestid.From(c.Request.Context()), At: m.core.now(),
	})
}

func (m *Module) completeLogin(c *gin.Context, account, token string, sess Session) {
	ctx := c.Request.Context()
	if err := m.core.limiter.reset(ctx, account); err != nil {
		m.core.logger.Warn("login counter not reset", zap.Error(err))
	}
	m.endPreviousSession(c)
	if err := m.core.db.WithContext(ctx).Exec(`UPDATE sys_users SET last_login_at = CURRENT_TIMESTAMP(6), last_login_ip = ?
		WHERE id = ?`, c.ClientIP(), sess.UserID).Error; err != nil {
		m.core.logger.Warn("last login not recorded", zap.Error(err))
	}
	m.setCookies(c, token, sess.CSRF)
	m.respondMe(c, sess)
}

// endPreviousSession drops the session the client presented before logging
// in, preventing session fixation.
func (m *Module) endPreviousSession(c *gin.Context) {
	sess, ok, err := m.core.Guard.currentSession(c)
	if err == nil && ok {
		_ = m.core.Sessions.Delete(c.Request.Context(), sess)
	}
}

func (m *Module) respondMe(c *gin.Context, sess Session) {
	a, err := m.core.actorFor(c.Request.Context(), sess, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	data, err := m.me(actor.With(c.Request.Context(), a))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, data)
}

func (m *Module) logout(c *gin.Context) {
	a := actor.From(c.Request.Context())
	if err := m.core.Sessions.Delete(c.Request.Context(), Session{Hash: a.SessionHash, UserID: a.UserID()}); err != nil {
		httpx.Fail(c, unavailable(err))
		return
	}
	var username string
	_ = m.core.db.WithContext(c.Request.Context()).Raw("SELECT username FROM sys_users WHERE id = ?", a.UserID()).Scan(&username).Error
	m.core.observer.RecordLogin(c.Request.Context(), LoginEvent{
		Kind: EventLogout, Username: username, UserID: a.UserID(), Success: true,
		IP: c.ClientIP(), UserAgent: truncate(c.Request.UserAgent(), 256),
		RequestID: requestid.From(c.Request.Context()), At: m.core.now(),
	})
	m.clearCookies(c)
	httpx.OK(c, nil)
}

type changePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required,max=128"`
	NewPassword string `json:"newPassword" binding:"required,min=8,max=72"`
}

func (m *Module) changePassword(c *gin.Context) {
	var req changePasswordRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	a := actor.From(ctx)
	uid := a.UserID()

	// A stolen session must not be able to brute-force the old password.
	budget := fmt.Sprintf("pwchange:%d", uid)
	blocked, err := m.core.limiter.reserve(ctx, c.ClientIP(), budget)
	if err != nil {
		httpx.Fail(c, unavailable(err))
		return
	}
	if blocked {
		httpx.Fail(c, httpx.TooManyRequests())
		return
	}
	var current string
	if err := m.core.db.WithContext(ctx).Raw("SELECT password_hash FROM sys_users WHERE id = ?", uid).Scan(&current).Error; err != nil {
		httpx.Fail(c, err)
		return
	}
	if ok, _ := m.core.verify(ctx, req.OldPassword, current); !ok {
		httpx.Fail(c, httpx.Validation([]httpx.FieldError{{Field: "oldPassword", Message: "原密码不正确"}}))
		return
	}
	if req.OldPassword == req.NewPassword {
		httpx.Fail(c, httpx.Validation([]httpx.FieldError{{Field: "newPassword", Message: "新密码不能与原密码相同"}}))
		return
	}
	if err := m.core.ValidatePassword(ctx, "newPassword", req.NewPassword); err != nil {
		httpx.Fail(c, err)
		return
	}
	_ = m.core.limiter.reset(ctx, budget)
	newHash, err := m.core.HashPassword(ctx, req.NewPassword)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	_, err = m.core.Revoker.Revoke(ctx, uid, RevokeSpec{
		Prepare: func(tx *gorm.DB) error { return requireHash(tx, uid, current) },
		Apply: func(tx *gorm.DB, _ int64) error {
			return tx.Exec(`UPDATE sys_users SET password_hash = ?, password_changed_at = CURRENT_TIMESTAMP(6),
				must_change_password = false, version = version + 1 WHERE id = ?`, newHash, uid).Error
		},
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	// Other devices are now logged out; keep this one by issuing a new
	// session stamped with the new generation.
	token, sess, err := m.core.establishSession(ctx, uid, newHash, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		m.clearCookies(c)
		httpx.Fail(c, errUnauthenticated.WithCause(err))
		return
	}
	_ = m.core.Sessions.Delete(ctx, Session{Hash: a.SessionHash, UserID: uid})
	m.setCookies(c, token, sess.CSRF)
	httpx.OK(c, nil)
}

func requireHash(tx *gorm.DB, uid int64, expected string) error {
	var current string
	if err := tx.Raw("SELECT password_hash FROM sys_users WHERE id = ?", uid).Scan(&current).Error; err != nil {
		return err
	}
	if current != expected {
		return httpx.Conflict("密码已被修改，请重试")
	}
	return nil
}

func (m *Module) setCookies(c *gin.Context, token, csrf string) {
	http.SetCookie(c.Writer, m.cookie(m.core.cfg.CookieName, token, true, 0))
	http.SetCookie(c.Writer, m.cookie(CSRFCookie, csrf, false, 0))
}

func (m *Module) clearCookies(c *gin.Context) {
	http.SetCookie(c.Writer, m.cookie(m.core.cfg.CookieName, "", true, -1))
	http.SetCookie(c.Writer, m.cookie(CSRFCookie, "", false, -1))
}

// cookie builds a session-lifetime cookie; expiry is enforced server-side.
func (m *Module) cookie(name, value string, httpOnly bool, maxAge int) *http.Cookie {
	// Secure comes from config and is enforced true in prod; the XSRF cookie
	// is deliberately readable by JS (double-submit pattern).
	return &http.Cookie{ //nolint:gosec // see comment above
		Name: name, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: httpOnly, Secure: m.core.cfg.CookieSecure, SameSite: http.SameSiteLaxMode,
	}
}
