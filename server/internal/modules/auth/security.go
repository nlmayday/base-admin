package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/core/menu"
	"base-admin/server/internal/core/perm"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

// Security centre: online sessions and the security policy.

var securityPermissions = []perm.Def{
	{Code: "auth:session:view", Name: "查看在线会话", Menu: "auth.sessions"},
	{Code: "auth:session:revoke", Name: "强制下线", Menu: "auth.sessions"},
	{Code: "auth:policy:view", Name: "查看安全策略", Menu: "auth.policy"},
	{Code: "auth:policy:update", Name: "修改安全策略", Menu: "auth.policy"},
}

var securityMenus = []menu.Def{
	{Code: "auth.security", Type: menu.TypeDir, Title: "安全中心", Path: "/security", Icon: "ri:shield-keyhole-line", Sort: 92},
	{Code: "auth.sessions", Parent: "auth.security", Type: menu.TypeMenu, Title: "在线会话", Path: "/security/sessions",
		Component: "security/sessions/index", Permission: "auth:session:view", Icon: "ri:computer-line", Sort: 1},
	{Code: "auth.policy", Parent: "auth.security", Type: menu.TypeMenu, Title: "安全策略", Path: "/security/policy",
		Component: "security/policy/index", Permission: "auth:policy:view", Icon: "ri:lock-password-line", Sort: 2},
}

var errProtectedSession = httpx.NewError(http.StatusConflict, 30002, "只有超级管理员可以下线超级管理员")

type sessionView struct {
	ID                string    `json:"id"`
	UserID            ids.ID    `json:"userId"`
	Username          string    `json:"username"`
	Nickname          string    `json:"nickname"`
	IP                string    `json:"ip"`
	UserAgent         string    `json:"userAgent"`
	CreatedAt         time.Time `json:"createdAt"`
	LastSeenAt        time.Time `json:"lastSeenAt"`
	AbsoluteExpiresAt time.Time `json:"absoluteExpiresAt"`
	Current           bool      `json:"current"`
}

type sessionFilter struct {
	Keyword string `form:"keyword" binding:"max=64"`
}

// listSessions returns valid sessions: a session whose generation was
// revoked is hidden (and its key removed) even if the key still exists.
func (m *Module) listSessions(c *gin.Context) (any, error) {
	pq, err := httpx.ParsePage(c)
	if err != nil {
		return nil, err
	}
	var f sessionFilter
	if err := c.ShouldBindQuery(&f); err != nil {
		return nil, httpx.BadRequest("查询参数错误").WithCause(err)
	}
	ctx := c.Request.Context()
	all, err := m.core.Sessions.Active(ctx, m.core.now())
	if err != nil {
		return nil, unavailable(err)
	}
	valid := m.validSessions(ctx, all)
	views, err := m.sessionViews(ctx, valid, actor.From(ctx).SessionHash)
	if err != nil {
		return nil, err
	}
	if kw := strings.ToLower(strings.TrimSpace(f.Keyword)); kw != "" {
		views = slices.DeleteFunc(views, func(v sessionView) bool {
			return !strings.Contains(strings.ToLower(v.Username+" "+v.Nickname+" "+v.IP), kw)
		})
	}
	total := len(views)
	start := min(pq.Offset(), total)
	end := min(start+pq.PageSize, total)
	return httpx.Page[sessionView]{List: views[start:end], Total: int64(total), Page: pq.Page, PageSize: pq.PageSize}, nil
}

func (m *Module) validSessions(ctx context.Context, all []Session) []Session {
	now := m.core.now()
	out := make([]Session, 0, len(all))
	for _, s := range all {
		gen, found, err := m.core.Sessions.GenMirror(ctx, s.UserID)
		if err != nil {
			continue
		}
		if !now.Before(s.AbsoluteExpiresAt) || (found && s.Gen < gen) {
			_ = m.core.Sessions.Delete(ctx, s)
			continue
		}
		out = append(out, s)
	}
	return out
}

func (m *Module) sessionViews(ctx context.Context, sessions []Session, current string) ([]sessionView, error) {
	userIDs := make([]int64, 0, len(sessions))
	for _, s := range sessions {
		userIDs = append(userIDs, s.UserID)
	}
	names := map[int64][2]string{}
	if len(userIDs) > 0 {
		var rows []struct {
			ID       int64
			Username string
			Nickname string
		}
		if err := m.core.db.WithContext(ctx).Raw("SELECT id, username, nickname FROM sys_users WHERE id IN ?",
			userIDs).Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			names[r.ID] = [2]string{r.Username, r.Nickname}
		}
	}
	views := make([]sessionView, 0, len(sessions))
	for _, s := range sessions {
		n := names[s.UserID]
		views = append(views, sessionView{
			ID: s.Hash, UserID: ids.ID(s.UserID), Username: n[0], Nickname: n[1], IP: s.IP, UserAgent: s.UserAgent,
			CreatedAt: s.CreatedAt, LastSeenAt: s.LastSeenAt, AbsoluteExpiresAt: s.AbsoluteExpiresAt,
			Current: s.Hash == current,
		})
	}
	return views, nil
}

// revokeSession ends one session.
func (m *Module) revokeSession(c *gin.Context) (any, error) {
	ctx := c.Request.Context()
	hash := c.Param("id")
	if !isSessionHash(hash) {
		return nil, httpx.NotFound("会话不存在或已结束")
	}
	sess, ok, err := m.core.Sessions.ByHash(ctx, hash)
	if err != nil {
		return nil, unavailable(err)
	}
	if !ok {
		return nil, httpx.NotFound("会话不存在或已结束")
	}
	if err := m.guardTarget(ctx, sess.UserID); err != nil {
		return nil, err
	}
	if err := m.core.Sessions.Delete(ctx, sess); err != nil {
		return nil, unavailable(err)
	}
	return nil, nil
}

// isSessionHash reports whether s looks like a session id (hex SHA-256), so
// arbitrary input never reaches the Redis key space.
func isSessionHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// revokeUserSessions ends every session of a user by raising the session
// generation (design doc 6.5.3).
func (m *Module) revokeUserSessions(c *gin.Context) (any, error) {
	uid, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	if err := m.guardTarget(ctx, uid); err != nil {
		return nil, err
	}
	_, err = m.core.Revoker.Revoke(ctx, uid, RevokeSpec{})
	return nil, err
}

// guardTarget applies the injected anti-escalation rule, or at least lets
// only a super admin end a super admin's sessions.
func (m *Module) guardTarget(ctx context.Context, uid int64) error {
	if actor.From(ctx).Snapshot.IsSuper {
		return nil
	}
	if m.canManage != nil {
		return m.canManage(ctx, uid)
	}
	snap, err := Build(ctx, m.core.db, uid)
	if errors.Is(err, ErrUserNotFound) {
		return httpx.NotFound("用户不存在")
	}
	if err != nil {
		return err
	}
	if snap.IsSuper {
		return errProtectedSession
	}
	return nil
}

type policyView struct {
	Policy
	// Session timeouts come from configuration and are shown read-only.
	SessionIdleMinutes     int `json:"sessionIdleMinutes"`
	SessionAbsoluteMinutes int `json:"sessionAbsoluteMinutes"`
	// ClientIP is the caller's address as the server sees it, for the
	// allowlist.
	ClientIP string `json:"clientIp" doc:"当前请求的 IP（服务端看到的地址）"`
}

type policyRole struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// listPolicyRoles lists the roles that can be made exempt from the login
// restrictions.
func (m *Module) listPolicyRoles(c *gin.Context) (any, error) {
	roles := []policyRole{}
	err := m.core.db.WithContext(c.Request.Context()).
		Raw(`SELECT code, name FROM sys_roles ORDER BY sort, id`).Scan(&roles).Error
	return roles, err
}

func (m *Module) getPolicy(c *gin.Context) (any, error) {
	p, err := m.core.policies.get(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return policyView{Policy: p, ClientIP: c.ClientIP(),
		SessionIdleMinutes:     int(m.core.cfg.IdleTimeout.Minutes()),
		SessionAbsoluteMinutes: int(m.core.cfg.AbsoluteTimeout.Minutes()),
	}, nil
}

func (m *Module) updatePolicy(c *gin.Context) (any, error) {
	var req Policy
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	if err := m.checkRestrictions(c, &req); err != nil {
		return nil, err
	}
	ips, err := json.Marshal(req.IPAllowlist)
	if err != nil {
		return nil, err
	}
	roles, err := json.Marshal(req.LoginExemptRoles)
	if err != nil {
		return nil, err
	}
	res := m.core.db.WithContext(ctx).Exec(`UPDATE sys_security_policy SET
			password_min_length = ?, password_require_upper = ?, password_require_lower = ?,
			password_require_letter = ?, password_require_digit = ?, password_require_symbol = ?, captcha_mode = ?,
			captcha_after_failures = ?, lockout_threshold = ?, lockout_minutes = ?,
			ip_allowlist = ?, login_exempt_roles = ?, login_time_start = ?, login_time_end = ?,
			login_time_zone = ?, version = version + 1, updated_at = CURRENT_TIMESTAMP(6), updated_by = ?
		WHERE id = 1 AND version = ?`,
		req.PasswordMinLength, req.PasswordRequireUpper, req.PasswordRequireLower, req.PasswordRequireLetter, req.PasswordRequireDigit,
		req.PasswordRequireSymbol, req.CaptchaMode, req.CaptchaAfterFailures, req.LockoutThreshold,
		req.LockoutMinutes, string(ips), string(roles), req.LoginTimeStart, req.LoginTimeEnd,
		req.LoginTimeZone, actor.From(ctx).UserID(), req.Version)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, httpx.Conflict("数据已被他人修改，请刷新")
	}
	m.core.policies.invalidate()
	return m.getPolicy(c)
}

// checkRestrictions validates the login restrictions of a new policy and
// refuses one that would keep the administrator saving it from logging in
// again (from this IP, at this time).
func (m *Module) checkRestrictions(c *gin.Context, p *Policy) error {
	if problems := p.normalizeRestrictions(); len(problems) > 0 {
		return httpx.Validation(problems)
	}
	ctx := c.Request.Context()
	missing, err := unknownRoles(ctx, m.core.db, p.LoginExemptRoles)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fieldError("loginExemptRoles", "角色不存在："+strings.Join(missing, "、"))
	}
	roles, err := userRoles(ctx, m.core.db, actor.From(ctx).UserID())
	if err != nil {
		return err
	}
	switch reason, _ := p.loginRefusal(c.ClientIP(), roles, m.core.now()); reason {
	case ReasonIPDenied:
		return fieldError("ipAllowlist", "保存后你将无法登录：当前 IP "+c.ClientIP()+" 不在白名单中（可加入白名单，或把你的角色设为豁免）")
	case ReasonTimeWindow:
		return fieldError("loginTimeEnd", "保存后你将无法登录：现在不在允许登录的时段内（可调整时段，或把你的角色设为豁免）")
	}
	return nil
}

func fieldError(field, msg string) error {
	return httpx.Validation([]httpx.FieldError{{Field: field, Message: msg}})
}

type unlockRequest struct {
	Username string `json:"username" binding:"required,max=64"`
}

// unlockAccount lifts a lockout before it expires.
func (m *Module) unlockAccount(c *gin.Context) (any, error) {
	var req unlockRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	account := strings.ToLower(strings.TrimSpace(req.Username))
	if err := m.core.lockout.unlock(c.Request.Context(), account); err != nil {
		return nil, unavailable(err)
	}
	return nil, nil
}

type captchaView struct {
	ID    string `json:"id"`
	Image string `json:"image"`
}

func (m *Module) issueCaptcha(c *gin.Context) (any, error) {
	blocked, err := m.core.limiter.reserveCaptcha(c.Request.Context(), c.ClientIP())
	if err != nil {
		return nil, unavailable(err)
	}
	if blocked {
		return nil, httpx.TooManyRequests()
	}
	id, image, err := m.core.captchas.issue(c.Request.Context())
	if err != nil {
		return nil, unavailable(err)
	}
	return captchaView{ID: id, Image: image}, nil
}

type loginOptions struct {
	// CaptchaAlways tells the login page to show the captcha up front;
	// otherwise it appears after a failure response asks for it.
	CaptchaAlways bool `json:"captchaAlways"`
}

func (m *Module) loginOptions(c *gin.Context) (any, error) {
	p, err := m.core.policies.get(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return loginOptions{CaptchaAlways: p.CaptchaMode == CaptchaAlways}, nil
}

// handle adapts a function returning (data, error) to a Gin handler.
func handle(fn func(c *gin.Context) (any, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := fn(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, data)
	}
}
