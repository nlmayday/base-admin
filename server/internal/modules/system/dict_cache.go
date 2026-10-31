package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"base-admin/server/internal/platform/httpx"
)

const (
	// dictCacheTTL bounds staleness if an invalidation is lost.
	dictCacheTTL = 10 * time.Minute
	// dictCacheOpTimeout bounds cache writes and deletes that outlive the
	// request context.
	dictCacheOpTimeout = 2 * time.Second
)

// ErrDictNotFound is returned by DictOptions for an unknown dictionary code.
var ErrDictNotFound = errors.New("system: dictionary not found")

// DictOption is one enabled dictionary item, as used by selects and tags.
type DictOption struct {
	Label   string `json:"label"`
	Value   string `json:"value"`
	TagType string `json:"tagType"`
}

func dictCacheKey(code string) string   { return "dict:" + code }
func dictVersionKey(code string) string { return "dict:ver:" + code }

// fillScript stores a refill only if no invalidation happened since the
// reader captured the version (ARGV[1]); otherwise a reader that loaded old
// rows before a commit could overwrite the invalidation.
var fillScript = redis.NewScript(`
local v = redis.call('GET', KEYS[2]) or '0'
if v == ARGV[1] then
  redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
  return 1
end
return 0`)

func (m *Module) dictOptions(c *gin.Context) (any, error) {
	code := c.Param("code")
	opts, err := m.DictOptions(c.Request.Context(), code)
	if errors.Is(err, ErrDictNotFound) {
		return nil, httpx.NotFound("字典不存在")
	}
	return opts, err
}

// DictOptions returns the enabled items of dictionary code in display
// order; a disabled dictionary has no items. Results are cached in Redis
// and the database is used whenever Redis fails.
func (m *Module) DictOptions(ctx context.Context, code string) ([]DictOption, error) {
	if !dictCodePattern.MatchString(code) {
		return nil, ErrDictNotFound
	}
	cached, ok := m.readDictCache(ctx, code)
	if ok {
		return cached, nil
	}
	ver := m.dictVersion(ctx, code)
	opts, err := m.loadDictOptions(ctx, code)
	if err != nil {
		return nil, err
	}
	if ver != "" {
		m.writeDictCache(ctx, code, opts, ver)
	}
	return opts, nil
}

// HasDictValue reports whether value is an enabled item of dictionary code.
// Business modules use it to validate fields backed by a dictionary.
func (m *Module) HasDictValue(ctx context.Context, code, value string) (bool, error) {
	opts, err := m.DictOptions(ctx, code)
	if errors.Is(err, ErrDictNotFound) {
		return false, nil // a deleted dictionary has no valid values
	}
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(opts, func(o DictOption) bool { return o.Value == value }), nil
}

func (m *Module) loadDictOptions(ctx context.Context, code string) ([]DictOption, error) {
	db := m.db.WithContext(ctx)
	var types []struct {
		ID     int64
		Status int16
	}
	if err := db.Raw("SELECT id, status FROM sys_dict_types WHERE code = ?", code).Scan(&types).Error; err != nil {
		return nil, fmt.Errorf("load dictionary %s: %w", code, err)
	}
	if len(types) == 0 {
		return nil, ErrDictNotFound
	}
	opts := []DictOption{}
	if types[0].Status != 1 {
		return opts, nil
	}
	if err := db.Raw(`SELECT label, value, tag_type FROM sys_dict_items WHERE type_id = ? AND status = 1
		ORDER BY sort, id LIMIT ?`, types[0].ID, maxItemsPerType).Scan(&opts).Error; err != nil {
		return nil, fmt.Errorf("load dictionary items %s: %w", code, err)
	}
	return opts, nil
}

func (m *Module) readDictCache(ctx context.Context, code string) ([]DictOption, bool) {
	if m.redis == nil {
		return nil, false
	}
	raw, err := m.redis.Get(ctx, dictCacheKey(code)).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			m.logger.Warn("dictionary cache read failed; using database", zap.String("code", code), zap.Error(err))
		}
		return nil, false
	}
	var opts []DictOption
	if err := json.Unmarshal(raw, &opts); err != nil || opts == nil {
		m.logger.Warn("dictionary cache entry is corrupt; using database", zap.String("code", code), zap.Error(err))
		return nil, false
	}
	return opts, true
}

// dictVersion returns the invalidation version of code ("0" if never
// invalidated), or "" when Redis is unavailable (the refill is skipped).
func (m *Module) dictVersion(ctx context.Context, code string) string {
	if m.redis == nil {
		return ""
	}
	v, err := m.redis.Get(ctx, dictVersionKey(code)).Result()
	switch {
	case errors.Is(err, redis.Nil):
		return "0"
	case err != nil:
		return ""
	}
	return v
}

func (m *Module) writeDictCache(ctx context.Context, code string, opts []DictOption, ver string) {
	if m.redis == nil {
		return
	}
	raw, err := json.Marshal(opts)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dictCacheOpTimeout)
	defer cancel()
	err = fillScript.Run(ctx, m.redis, []string{dictCacheKey(code), dictVersionKey(code)},
		ver, raw, dictCacheTTL.Milliseconds()).Err()
	if err != nil {
		m.logger.Warn("dictionary cache write failed", zap.String("code", code), zap.Error(err))
	}
}

// invalidateDict drops the cached items of code after a committed change.
// A failure is logged; the entry then expires within dictCacheTTL.
func (m *Module) invalidateDict(ctx context.Context, code string) {
	if m.redis == nil || code == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dictCacheOpTimeout)
	defer cancel()
	// Bump the version first, so a refill in flight cannot store old rows.
	pipe := m.redis.TxPipeline()
	pipe.Incr(ctx, dictVersionKey(code))
	pipe.Del(ctx, dictCacheKey(code))
	if _, err := pipe.Exec(ctx); err != nil {
		m.logger.Warn("dictionary cache invalidation failed", zap.String("code", code), zap.Error(err))
	}
}
