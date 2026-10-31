package auth

import (
	"context"
	"time"
)

// Login event kinds and failure reasons recorded in the login log.
const (
	EventLogin  = "login"
	EventLogout = "logout"

	ReasonBadCredentials = "bad_credentials"
	ReasonRateLimited    = "rate_limited"
	ReasonCaptcha        = "captcha"
	ReasonLocked         = "locked"
	ReasonError          = "error"
)

// LoginEvent describes one login or logout attempt.
type LoginEvent struct {
	Kind      string
	Username  string
	UserID    int64 // 0 when the account is unknown or the login failed
	Success   bool
	Reason    string
	IP        string
	UserAgent string
	RequestID string
	At        time.Time
}

// LoginObserver receives login events (the audit module records them).
// Implementations must not block: login latency must not depend on them.
type LoginObserver interface {
	RecordLogin(ctx context.Context, e LoginEvent)
}

type noopObserver struct{}

func (noopObserver) RecordLogin(context.Context, LoginEvent) {}
