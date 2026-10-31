package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"base-admin/server/internal/platform/httpx"
)

// ReasonDenied is the login log reason when a LoginGuard refuses a login.
const ReasonDenied = "denied"

// LoginAttempt is a login whose credentials have been verified.
type LoginAttempt struct {
	UserID    int64
	Username  string // normalized (trimmed, lower case)
	IP        string
	UserAgent string
	At        time.Time
}

// LoginGuard lets another module refuse a login. It runs after the password
// check (so it knows the user) and before the session is created; returning
// an error refuses the login. An *httpx.Error reaches the client as is (use
// status 403 and a code of the module's range); any other error is reported
// as 403 / CodeLoginDenied. Refusals are recorded in the login log with
// reason "denied".
//
// Because a guard runs after the password check, its refusal tells the
// client the password was right: guard only on rules about the account,
// not as a way to hide whether credentials are valid.
type LoginGuard interface {
	CheckLogin(ctx context.Context, attempt LoginAttempt) error
}

// LoginGuardFunc adapts a function to LoginGuard.
type LoginGuardFunc func(ctx context.Context, attempt LoginAttempt) error

// CheckLogin implements LoginGuard.
func (f LoginGuardFunc) CheckLogin(ctx context.Context, attempt LoginAttempt) error {
	return f(ctx, attempt)
}

var errLoginDenied = httpx.NewError(http.StatusForbidden, CodeLoginDenied, "当前不允许登录，请联系管理员")

// AddLoginGuard registers a guard; guards run in registration order. Call
// it while wiring modules, before the server starts.
func (c *Core) AddLoginGuard(g LoginGuard) {
	c.guards = append(c.guards, g)
}

func (c *Core) checkGuards(ctx context.Context, attempt LoginAttempt) error {
	for _, g := range c.guards {
		err := g.CheckLogin(ctx, attempt)
		if err == nil {
			continue
		}
		var he *httpx.Error
		if errors.As(err, &he) {
			return err
		}
		return errLoginDenied.WithCause(err)
	}
	return nil
}
