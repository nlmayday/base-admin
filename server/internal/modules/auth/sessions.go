package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Session is a server-side session. Only the SHA-256 of the token is
// stored, so a Redis dump does not yield usable tokens.
type Session struct {
	Hash              string
	UserID            int64
	Gen               int64
	CSRF              string
	CreatedAt         time.Time
	LastSeenAt        time.Time
	AbsoluteExpiresAt time.Time
	IP                string
	UserAgent         string
}

// SessionStore keeps sessions and session generation mirrors in Redis.
type SessionStore struct {
	rdb      *redis.Client
	idle     time.Duration
	absolute time.Duration
	touch    time.Duration
}

// NewSessionStore creates a store with the given expiry policy.
func NewSessionStore(rdb *redis.Client, idle, absolute, touch time.Duration) *SessionStore {
	return &SessionStore{rdb: rdb, idle: idle, absolute: absolute, touch: touch}
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken returns the storage key component for a token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Create stores a new session stamped with gen and returns its token.
func (s *SessionStore) Create(ctx context.Context, uid, gen int64, ip, ua string, now time.Time) (string, Session, error) {
	token, err := randomToken()
	if err != nil {
		return "", Session{}, err
	}
	csrf, err := randomToken()
	if err != nil {
		return "", Session{}, err
	}
	sess := Session{
		Hash: HashToken(token), UserID: uid, Gen: gen, CSRF: csrf,
		CreatedAt: now, LastSeenAt: now, AbsoluteExpiresAt: now.Add(s.absolute),
		IP: ip, UserAgent: truncate(ua, 256),
	}
	_, err = s.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.HSet(ctx, sessionKey(sess.Hash), map[string]any{
			"uid": uid, "gen": gen, "csrf": csrf,
			"created": now.Unix(), "last": now.Unix(), "abs": sess.AbsoluteExpiresAt.Unix(),
			"ip": ip, "ua": sess.UserAgent,
		})
		p.Expire(ctx, sessionKey(sess.Hash), s.idle)
		p.SAdd(ctx, userSessionsKey(uid), sess.Hash)
		p.Expire(ctx, userSessionsKey(uid), s.absolute)
		p.ZAdd(ctx, activeKey, redis.Z{Score: float64(now.Unix()), Member: sess.Hash})
		return nil
	})
	if err != nil {
		return "", Session{}, fmt.Errorf("store session: %w", err)
	}
	return token, sess, nil
}

// Get loads the session for token. ok is false when it does not exist.
func (s *SessionStore) Get(ctx context.Context, token string) (Session, bool, error) {
	hash := HashToken(token)
	m, err := s.rdb.HGetAll(ctx, sessionKey(hash)).Result()
	if err != nil {
		return Session{}, false, fmt.Errorf("load session: %w", err)
	}
	if len(m) == 0 {
		return Session{}, false, nil
	}
	sess, err := parseSession(hash, m)
	if err != nil {
		return Session{}, false, err
	}
	return sess, true, nil
}

func parseSession(hash string, m map[string]string) (Session, error) {
	ints := map[string]int64{}
	for _, k := range []string{"uid", "gen", "created", "last", "abs"} {
		n, err := strconv.ParseInt(m[k], 10, 64)
		if err != nil {
			return Session{}, fmt.Errorf("corrupt session field %s", k)
		}
		ints[k] = n
	}
	return Session{
		Hash: hash, UserID: ints["uid"], Gen: ints["gen"], CSRF: m["csrf"],
		CreatedAt: time.Unix(ints["created"], 0), LastSeenAt: time.Unix(ints["last"], 0),
		AbsoluteExpiresAt: time.Unix(ints["abs"], 0), IP: m["ip"], UserAgent: m["ua"],
	}, nil
}

// Touch slides the idle expiry, at most once per touch interval and never
// beyond the absolute expiry.
func (s *SessionStore) Touch(ctx context.Context, sess Session, now time.Time) error {
	if now.Sub(sess.LastSeenAt) < s.touch {
		return nil
	}
	ttl := min(s.idle, sess.AbsoluteExpiresAt.Sub(now))
	if ttl <= 0 {
		return nil
	}
	_, err := s.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.HSet(ctx, sessionKey(sess.Hash), "last", now.Unix())
		p.Expire(ctx, sessionKey(sess.Hash), ttl)
		p.ZAdd(ctx, activeKey, redis.Z{Score: float64(now.Unix()), Member: sess.Hash})
		return nil
	})
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

// Delete removes one session.
func (s *SessionStore) Delete(ctx context.Context, sess Session) error {
	_, err := s.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Del(ctx, sessionKey(sess.Hash))
		p.SRem(ctx, userSessionsKey(sess.UserID), sess.Hash)
		p.ZRem(ctx, activeKey, sess.Hash)
		return nil
	})
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteAllForUser removes the session keys of a user. It is cleanup only:
// correctness comes from the session generation.
func (s *SessionStore) DeleteAllForUser(ctx context.Context, uid int64) error {
	hashes, err := s.rdb.SMembers(ctx, userSessionsKey(uid)).Result()
	if err != nil {
		return fmt.Errorf("list user sessions: %w", err)
	}
	keys := make([]string, 0, len(hashes)+1)
	for _, h := range hashes {
		keys = append(keys, sessionKey(h))
	}
	keys = append(keys, userSessionsKey(uid))
	_, err = s.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Del(ctx, keys...)
		if len(hashes) > 0 {
			members := make([]any, len(hashes))
			for i, h := range hashes {
				members[i] = h
			}
			p.ZRem(ctx, activeKey, members...)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

// maxListedSessions bounds the online session listing.
const maxListedSessions = 5000

// Active returns stored sessions, most recently active first. Entries whose
// session key has expired are pruned from the index as a side effect.
func (s *SessionStore) Active(ctx context.Context, now time.Time) ([]Session, error) {
	// Anything idle for longer than the absolute lifetime cannot be alive.
	cutoff := fmt.Sprintf("(%d", now.Add(-s.absolute).Unix())
	if err := s.rdb.ZRemRangeByScore(ctx, activeKey, "-inf", cutoff).Err(); err != nil {
		return nil, fmt.Errorf("prune session index: %w", err)
	}
	hashes, err := s.rdb.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key: activeKey, Start: 0, Stop: maxListedSessions - 1, Rev: true,
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("list session index: %w", err)
	}
	pipe := s.rdb.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(hashes))
	for i, h := range hashes {
		cmds[i] = pipe.HGetAll(ctx, sessionKey(h))
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("load sessions: %w", err)
	}
	out := make([]Session, 0, len(hashes))
	var gone []any
	for i, cmd := range cmds {
		m := cmd.Val()
		if len(m) == 0 {
			gone = append(gone, hashes[i])
			continue
		}
		if sess, err := parseSession(hashes[i], m); err == nil {
			out = append(out, sess)
		}
	}
	if len(gone) > 0 {
		_ = s.rdb.ZRem(ctx, activeKey, gone...).Err()
	}
	return out, nil
}

// ByHash loads a session by its stored hash (administrator views).
func (s *SessionStore) ByHash(ctx context.Context, hash string) (Session, bool, error) {
	m, err := s.rdb.HGetAll(ctx, sessionKey(hash)).Result()
	if err != nil {
		return Session{}, false, fmt.Errorf("load session: %w", err)
	}
	if len(m) == 0 {
		return Session{}, false, nil
	}
	sess, err := parseSession(hash, m)
	return sess, err == nil, err
}

// GenMirror returns the mirrored session generation; found is false when the
// key is missing, which callers must treat as a cache miss (invariant I2).
func (s *SessionStore) GenMirror(ctx context.Context, uid int64) (gen int64, found bool, err error) {
	gen, err = s.rdb.Get(ctx, genKey(uid)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read session generation: %w", err)
	}
	return gen, true, nil
}

// RaiseGen raises the mirror to at least gen and returns the mirror value.
func (s *SessionStore) RaiseGen(ctx context.Context, uid, gen int64) (int64, error) {
	cur, err := maxSetScript.Run(ctx, s.rdb, []string{genKey(uid)}, gen).Int64()
	if err != nil {
		return 0, fmt.Errorf("raise session generation: %w", err)
	}
	return cur, nil
}

// RevokeGen sets the mirror to max(mirror, pgGen)+1 and returns it.
func (s *SessionStore) RevokeGen(ctx context.Context, uid, pgGen int64) (int64, error) {
	n, err := revokeGenScript.Run(ctx, s.rdb, []string{genKey(uid)}, pgGen).Int64()
	if err != nil {
		return 0, fmt.Errorf("revoke session generation: %w", err)
	}
	return n, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
