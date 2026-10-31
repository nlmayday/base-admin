package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIPAllowed(t *testing.T) {
	list := []string{"203.0.113.8", "10.0.0.0/8", "2001:db8::/32"}
	for ip, want := range map[string]bool{
		"203.0.113.8":        true,
		"203.0.113.9":        false,
		"10.1.2.3":           true,
		"11.0.0.1":           false,
		"::ffff:10.0.0.1":    true, // IPv4-mapped
		"2001:db8::1":        true,
		"2001:db9::1":        false,
		"not-an-ip":          false,
		"":                   false,
		"10.0.0.1:443":       false,
		"::ffff:203.0.113.8": true,
	} {
		assert.Equal(t, want, ipAllowed(list, ip), ip)
	}
}

func TestLoginWindow(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	assert.NoError(t, err)
	at := func(h, m int) time.Time { return time.Date(2026, 10, 31, h, m, 0, 0, shanghai).UTC() }

	day := Policy{LoginTimeStart: "08:00", LoginTimeEnd: "23:00", LoginTimeZone: "Asia/Shanghai"}
	assert.True(t, day.inLoginWindow(at(8, 0)), "start is inclusive")
	assert.True(t, day.inLoginWindow(at(22, 59)))
	assert.False(t, day.inLoginWindow(at(23, 0)), "end is exclusive")
	assert.False(t, day.inLoginWindow(at(7, 59)))

	night := Policy{LoginTimeStart: "22:00", LoginTimeEnd: "06:00", LoginTimeZone: "Asia/Shanghai"}
	assert.True(t, night.inLoginWindow(at(23, 30)), "spans midnight")
	assert.True(t, night.inLoginWindow(at(5, 59)))
	assert.False(t, night.inLoginWindow(at(12, 0)))

	utc := Policy{LoginTimeStart: "08:00", LoginTimeEnd: "09:00", LoginTimeZone: "UTC"}
	assert.True(t, utc.inLoginWindow(at(16, 30)), "16:30 Shanghai is 08:30 UTC")
}

func TestLoginRefusal(t *testing.T) {
	now := time.Now()
	none := Policy{LoginTimeZone: "UTC"}
	reason, err := none.loginRefusal("1.2.3.4", nil, now)
	assert.Empty(t, reason, "no restriction configured")
	assert.NoError(t, err)

	p := Policy{IPAllowlist: []string{"10.0.0.0/8"}, LoginExemptRoles: []string{"ops"}, LoginTimeZone: "UTC"}
	reason, err = p.loginRefusal("1.2.3.4", []string{"sales"}, now)
	assert.Equal(t, ReasonIPDenied, reason)
	assert.Error(t, err)
	reason, _ = p.loginRefusal("1.2.3.4", []string{"sales", "ops"}, now)
	assert.Empty(t, reason, "an exempt role skips the allowlist")
	reason, _ = p.loginRefusal("10.9.9.9", nil, now)
	assert.Empty(t, reason)

	closed := Policy{LoginTimeStart: "00:00", LoginTimeEnd: "00:01", LoginTimeZone: "UTC"}
	noon := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	reason, _ = closed.loginRefusal("1.2.3.4", nil, noon)
	assert.Equal(t, ReasonTimeWindow, reason)
}

func TestNormalizeRestrictions(t *testing.T) {
	p := Policy{
		IPAllowlist:      []string{" 10.0.0.0/8 ", "", "10.0.0.0/8", "bad", "300.1.1.1"},
		LoginExemptRoles: []string{" ops ", "ops", ""},
		LoginTimeStart:   "08:00",
	}
	problems := p.normalizeRestrictions()
	assert.Equal(t, []string{"10.0.0.0/8", "bad", "300.1.1.1"}, p.IPAllowlist, "trimmed and deduplicated")
	assert.Equal(t, []string{"ops"}, p.LoginExemptRoles)
	assert.Equal(t, defaultLoginTimeZone, p.LoginTimeZone, "empty zone defaults")
	fields := map[string]int{}
	for _, pr := range problems {
		fields[pr.Field]++
	}
	assert.Equal(t, map[string]int{"ipAllowlist": 2, "loginTimeEnd": 1}, fields)

	same := Policy{LoginTimeStart: "08:00", LoginTimeEnd: "08:00", LoginTimeZone: "Mars/Base"}
	fields = map[string]int{}
	for _, pr := range same.normalizeRestrictions() {
		fields[pr.Field]++
	}
	assert.Equal(t, map[string]int{"loginTimeEnd": 1, "loginTimeZone": 1}, fields)
}
