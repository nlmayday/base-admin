package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"time"
	_ "time/tzdata" // the login window's zone must resolve in distroless images

	"gorm.io/gorm"

	"base-admin/server/internal/platform/httpx"
)

// Login restrictions of the security policy: an IP allowlist and a daily
// login window. Both are checked before the password, so a refusal says
// nothing about the credentials; members of an exempt role skip both.

const defaultLoginTimeZone = "Asia/Shanghai"

// Refusal reasons in the login log.
const (
	ReasonIPDenied   = "ip_denied"
	ReasonTimeWindow = "time_window"
)

var (
	errIPDenied   = httpx.NewError(http.StatusForbidden, CodeLoginIPDenied, "当前 IP 不允许登录，请联系管理员")
	errTimeWindow = httpx.NewError(http.StatusForbidden, CodeLoginTimeDenied, "当前时段不允许登录")
)

// restricted reports whether any login restriction is configured.
func (p Policy) restricted() bool {
	return len(p.IPAllowlist) > 0 || p.LoginTimeStart != ""
}

// loginRefusal returns the log reason and error when a login from ip at now
// by a member of roles breaks the policy, or "" and nil.
func (p Policy) loginRefusal(ip string, roles []string, now time.Time) (string, error) {
	if !p.restricted() || slices.ContainsFunc(roles, func(r string) bool { return slices.Contains(p.LoginExemptRoles, r) }) {
		return "", nil
	}
	if len(p.IPAllowlist) > 0 && !ipAllowed(p.IPAllowlist, ip) {
		return ReasonIPDenied, errIPDenied
	}
	if p.LoginTimeStart != "" && !p.inLoginWindow(now) {
		return ReasonTimeWindow, errTimeWindow.WithDetails(map[string]string{
			"start": p.LoginTimeStart, "end": p.LoginTimeEnd, "timeZone": p.LoginTimeZone,
		})
	}
	return "", nil
}

// ipAllowed reports whether ip matches an address or CIDR in list. Entries
// were validated on save; unparsable ones never match.
func ipAllowed(list []string, ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, entry := range list {
		if prefix, err := parseAllowEntry(entry); err == nil && prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// parseAllowEntry reads an address (a single-host prefix) or a CIDR.
func parseAllowEntry(entry string) (netip.Prefix, error) {
	entry = strings.TrimSpace(entry)
	if strings.Contains(entry, "/") {
		prefix, err := netip.ParsePrefix(entry)
		return prefix.Masked(), err
	}
	addr, err := netip.ParseAddr(entry)
	if err != nil {
		return netip.Prefix{}, err
	}
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

// inLoginWindow reports whether now falls in [start, end) in the policy's
// zone; an end at or before the start spans midnight.
func (p Policy) inLoginWindow(now time.Time) bool {
	loc, err := time.LoadLocation(p.LoginTimeZone)
	if err != nil {
		loc = time.UTC
	}
	start, end := minuteOfDay(p.LoginTimeStart), minuteOfDay(p.LoginTimeEnd)
	t := now.In(loc)
	cur := t.Hour()*60 + t.Minute()
	if start < end {
		return cur >= start && cur < end
	}
	return cur >= start || cur < end
}

func minuteOfDay(hhmm string) int {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return 0
	}
	return t.Hour()*60 + t.Minute()
}

// normalizeRestrictions trims the lists and checks the restriction fields,
// returning the problems by field.
func (p *Policy) normalizeRestrictions() []httpx.FieldError {
	var problems []httpx.FieldError
	ips := make([]string, 0, len(p.IPAllowlist))
	for _, entry := range p.IPAllowlist {
		entry = strings.TrimSpace(entry)
		if entry == "" || slices.Contains(ips, entry) {
			continue
		}
		if _, err := parseAllowEntry(entry); err != nil {
			problems = append(problems, httpx.FieldError{Field: "ipAllowlist", Message: entry + " 不是有效的 IP 或网段（如 10.0.0.0/8）"})
		}
		ips = append(ips, entry)
	}
	roles := make([]string, 0, len(p.LoginExemptRoles))
	for _, r := range p.LoginExemptRoles {
		if r = strings.TrimSpace(r); r != "" && !slices.Contains(roles, r) {
			roles = append(roles, r)
		}
	}
	p.IPAllowlist, p.LoginExemptRoles = ips, roles
	p.LoginTimeZone = strings.TrimSpace(p.LoginTimeZone)
	if p.LoginTimeZone == "" {
		p.LoginTimeZone = defaultLoginTimeZone
	}
	if _, err := time.LoadLocation(p.LoginTimeZone); err != nil {
		problems = append(problems, httpx.FieldError{Field: "loginTimeZone", Message: "时区无效，如 Asia/Shanghai"})
	}
	switch {
	case (p.LoginTimeStart == "") != (p.LoginTimeEnd == ""):
		problems = append(problems, httpx.FieldError{Field: "loginTimeEnd", Message: "开始和结束时间要同时填写或同时留空"})
	case p.LoginTimeStart != "" && p.LoginTimeStart == p.LoginTimeEnd:
		problems = append(problems, httpx.FieldError{Field: "loginTimeEnd", Message: "结束时间不能等于开始时间"})
	}
	return problems
}

// unknownRoles returns the exempt role codes that do not exist.
func unknownRoles(ctx context.Context, db *gorm.DB, codes []string) ([]string, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	var found []string
	if err := db.WithContext(ctx).Raw(`SELECT code FROM sys_roles WHERE code IN ?`, codes).Scan(&found).Error; err != nil {
		return nil, err
	}
	var missing []string
	for _, c := range codes {
		if !slices.Contains(found, c) {
			missing = append(missing, c)
		}
	}
	return missing, nil
}

// accountRoles returns the role codes of the active user with the given
// (normalized) username; none when there is no such user.
func accountRoles(ctx context.Context, db *gorm.DB, account string) ([]string, error) {
	var codes []string
	err := db.WithContext(ctx).Raw(`SELECT r.code FROM sys_users u
		JOIN sys_user_roles ur ON ur.user_id = u.id
		JOIN sys_roles r ON r.id = ur.role_id
		WHERE lower(u.username) = ? AND u.deleted_at IS NULL`, account).Scan(&codes).Error
	if err != nil {
		return nil, fmt.Errorf("load account roles: %w", err)
	}
	return codes, nil
}

// userRoles returns the role codes of a user.
func userRoles(ctx context.Context, db *gorm.DB, uid int64) ([]string, error) {
	var codes []string
	err := db.WithContext(ctx).Raw(`SELECT r.code FROM sys_user_roles ur
		JOIN sys_roles r ON r.id = ur.role_id WHERE ur.user_id = ?`, uid).Scan(&codes).Error
	if err != nil {
		return nil, fmt.Errorf("load user roles: %w", err)
	}
	return codes, nil
}
