package system

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// valueCacheTTL bounds how stale GetString/GetInt/UploadPolicy can be on
// other instances; local writes clear the cache immediately.
const valueCacheTTL = 15 * time.Second

// ErrConfigNotFound is returned by GetString/GetInt for an unknown key.
var ErrConfigNotFound = errors.New("system: config not found")

type cachedValue struct {
	value   string
	expires time.Time
}

// valueCache is a small in-process cache of config values by key. gen is
// bumped by clear, so a value read from the database before a local write
// committed cannot be stored after it (put ignores a stale generation).
type valueCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	now     func() time.Time
	entries map[string]cachedValue
	gen     uint64
}

func newValueCache(ttl time.Duration, now func() time.Time) *valueCache {
	return &valueCache{ttl: ttl, now: now, entries: map[string]cachedValue{}}
}

func (c *valueCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || !c.now().Before(e.expires) {
		return "", false
	}
	return e.value, true
}

// generation returns the token to pass to put for a value about to be read.
func (c *valueCache) generation() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

func (c *valueCache) put(key, value string, gen uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.gen {
		return // cleared while the value was being read
	}
	c.entries[key] = cachedValue{value: value, expires: c.now().Add(c.ttl)}
}

func (c *valueCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	c.entries = map[string]cachedValue{}
}

// GetString returns the value of config key, cached for a few seconds. It
// returns ErrConfigNotFound when the key does not exist.
func (m *Module) GetString(ctx context.Context, key string) (string, error) {
	if v, ok := m.values.get(key); ok {
		return v, nil
	}
	gen := m.values.generation()
	var rows []struct{ Value string }
	if err := m.db.WithContext(ctx).Raw(`SELECT value FROM sys_configs WHERE "key" = ?`, key).Scan(&rows).Error; err != nil {
		return "", fmt.Errorf("load config %s: %w", key, err)
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("%w: %s", ErrConfigNotFound, key)
	}
	m.values.put(key, rows[0].Value, gen)
	return rows[0].Value, nil
}

// GetInt returns config key parsed as an integer.
func (m *Module) GetInt(ctx context.Context, key string) (int, error) {
	v, err := m.GetString(ctx, key)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, fmt.Errorf("config %s is not an integer: %w", key, err)
	}
	return n, nil
}

// UploadPolicy returns the allowed upload extensions (lowercase, without
// dot; dangerous types such as html/svg/js/exe are always removed) and the
// maximum upload size in bytes. Callers still apply their own hard ceiling.
func (m *Module) UploadPolicy(ctx context.Context) (exts []string, maxBytes int64, err error) {
	csv, err := m.GetString(ctx, KeyUploadAllowedExtensions)
	if err != nil {
		return nil, 0, err
	}
	mb, err := m.GetInt(ctx, KeyUploadMaxSizeMB)
	if err != nil {
		return nil, 0, err
	}
	if mb < minUploadMaxSizeMB || mb > maxUploadMaxSizeMB {
		return nil, 0, fmt.Errorf("config %s out of range: %d", KeyUploadMaxSizeMB, mb)
	}
	return safeExtensions(csv), int64(mb) * bytesPerMB, nil
}
