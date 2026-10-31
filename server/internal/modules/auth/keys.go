// Package auth implements sessions, CSRF protection, the access guard,
// session revocation and the auth version mirror (design doc sections 5.6
// and 6). It owns the authentication columns of sys_users (password_hash,
// session_gen, status checks) and reads IAM tables only to build snapshots.
package auth

import (
	"fmt"

	"github.com/redis/go-redis/v9"
)

const (
	versionKey = "auth:ver"
	// activeKey is a sorted set of session hashes scored by last activity,
	// used to list online sessions.
	activeKey = "sess:active"
)

func sessionKey(hash string) string      { return "sess:" + hash }
func userSessionsKey(uid int64) string   { return fmt.Sprintf("user_sess:%d", uid) }
func genKey(uid int64) string            { return fmt.Sprintf("sess:gen:%d", uid) }
func snapshotKey(ver, uid int64) string  { return fmt.Sprintf("auth:snapshot:v%d:user:%d", ver, uid) }
func loginIPKey(ip string) string        { return "rl:login:ip:" + ip }
func loginAccountKey(name string) string { return "rl:login:acct:" + name }

// maxSetScript writes ARGV[1] only if it is greater than the current value,
// so mirrors are monotonic even when writers race (design doc 5.6, 6.5.1).
var maxSetScript = redis.NewScript(`
local cur = tonumber(redis.call('GET', KEYS[1]))
local v = tonumber(ARGV[1])
if cur == nil or v > cur then
  redis.call('SET', KEYS[1], string.format('%d', v))
  return v
end
return cur`)

// revokeGenScript computes n = max(mirror, pg) + 1 and stores it, so the new
// generation exceeds every generation ever stamped (design doc 6.5.3).
var revokeGenScript = redis.NewScript(`
local m = tonumber(redis.call('GET', KEYS[1])) or 0
local g = tonumber(ARGV[1])
local n = math.max(m, g) + 1
redis.call('SET', KEYS[1], string.format('%d', n))
return n`)
