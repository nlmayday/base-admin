package auth

import (
	"net/http"

	"base-admin/server/internal/platform/httpx"
)

// Auth error codes (2xxxx, design doc 9 and docs/api-p0.md).
const (
	CodeUnauthenticated        = 20001
	CodeForbidden              = 20002
	CodeCSRF                   = 20003
	CodeBadCredentials         = 20004
	CodePasswordChangeRequired = 20005
	CodeAccountLocked          = 20006
	CodeCaptchaRequired        = 20007
	// CodeLoginIPDenied / CodeLoginTimeDenied: the security policy's IP
	// allowlist or login window refuses the login (403).
	CodeLoginIPDenied   = 20008
	CodeLoginTimeDenied = 20009
	// CodeLoginDenied: a LoginGuard refused the login without its own code.
	CodeLoginDenied = 20010
)

var (
	errUnauthenticated = httpx.NewError(http.StatusUnauthorized, CodeUnauthenticated, "未登录或会话已失效")
	errForbidden       = httpx.NewError(http.StatusForbidden, CodeForbidden, "无权限")
	errCSRF            = httpx.NewError(http.StatusForbidden, CodeCSRF, "请求来源校验失败")
	errBadCredentials  = httpx.NewError(http.StatusUnauthorized, CodeBadCredentials, "用户名或密码错误")
	errPasswordChange  = httpx.NewError(http.StatusForbidden, CodePasswordChangeRequired, "请先修改密码")
	errUnavailable     = httpx.Unavailable("认证服务暂不可用")
	errCaptcha         = httpx.NewError(http.StatusUnauthorized, CodeCaptchaRequired, "验证码错误或已过期").
				WithDetails(map[string]bool{"captchaRequired": true})
	errBusy = httpx.Unavailable("用户正在被其他操作修改，请重试")
)
