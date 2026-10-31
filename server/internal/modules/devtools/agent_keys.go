package devtools

import (
	"cmp"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"base-admin/server/internal/core/actor"
	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/ids"
)

// Keys of the AI agent API (开发工具 → AI 接口). A key is shown once when
// created; only its SHA-256 is stored. It acts as its creator (agent.go).

const (
	agentKeyPrefix = "bak_"
	// A key always expires: it ends up in agent conversations, terminal
	// history and logs. defaultKeyDays applies when none is given.
	defaultKeyDays = 7
	maxKeyDays     = 30
)

type agentKeyView struct {
	ID         ids.ID     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix" doc:"密钥开头几位，用于区分"`
	UserID     ids.ID     `json:"userId"`
	UserName   string     `json:"userName" doc:"创建人（密钥以其身份调用）"`
	ExpiresAt  *time.Time `json:"expiresAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	RevokedAt  *time.Time `json:"revokedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
}

type agentKeyList struct {
	// Enabled: the agent API works in this environment (app.env=dev).
	Enabled bool `json:"enabled"`
	// Endpoint is the site-relative base path of the agent API.
	Endpoint string         `json:"endpoint"`
	Keys     []agentKeyView `json:"keys"`
}

func (m *Module) listAgentKeys(c *gin.Context) (any, error) {
	keys := []agentKeyView{}
	err := m.deps.DB.WithContext(c.Request.Context()).Raw(`SELECT k.id, k.name, k.prefix, k.user_id,
			COALESCE(NULLIF(u.nickname, ''), u.username, '') AS user_name,
			k.expires_at, k.last_used_at, k.revoked_at, k.created_at
		FROM sys_agent_keys k LEFT JOIN sys_users u ON u.id = k.user_id
		ORDER BY k.revoked_at IS NOT NULL, k.id DESC`).Scan(&keys).Error
	if err != nil {
		return nil, err
	}
	return agentKeyList{Enabled: m.requireDev() == nil, Endpoint: m.deps.APIPrefix + agentPath, Keys: keys}, nil
}

type createAgentKeyRequest struct {
	Name          string `json:"name" binding:"required" doc:"用途，如「Claude Code 本机」"`
	ExpiresInDays int    `json:"expiresInDays" binding:"min=0" doc:"有效天数，1–30，不传为 7"`
}

type createdAgentKey struct {
	// Key is shown only in this response.
	Key  string       `json:"key" doc:"完整密钥，只在此时返回一次"`
	Item agentKeyView `json:"item"`
}

func (m *Module) createAgentKey(c *gin.Context) (any, error) {
	var req createAgentKeyRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	switch {
	case name == "" || utf8.RuneCountInString(name) > 64:
		return nil, fieldError("name", "名称不能为空，最长 64 个字符")
	case req.ExpiresInDays > maxKeyDays:
		return nil, fieldError("expiresInDays", fmt.Sprintf("有效期最长 %d 天", maxKeyDays))
	}
	key, hash := newAgentKey()
	expires := time.Now().AddDate(0, 0, cmp.Or(req.ExpiresInDays, defaultKeyDays))
	ctx := c.Request.Context()
	var id int64
	err := m.deps.DB.WithContext(ctx).Raw(`INSERT INTO sys_agent_keys (name, prefix, key_hash, user_id, expires_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id`, name, key[:len(agentKeyPrefix)+8], hash, actor.From(ctx).UserID(), expires).
		Scan(&id).Error
	if err != nil {
		return nil, err
	}
	var item agentKeyView
	err = m.deps.DB.WithContext(ctx).Raw(`SELECT k.id, k.name, k.prefix, k.user_id,
			COALESCE(NULLIF(u.nickname, ''), u.username, '') AS user_name,
			k.expires_at, k.last_used_at, k.revoked_at, k.created_at
		FROM sys_agent_keys k LEFT JOIN sys_users u ON u.id = k.user_id WHERE k.id = ?`, id).Scan(&item).Error
	return createdAgentKey{Key: key, Item: item}, err
}

func (m *Module) revokeAgentKey(c *gin.Context) (any, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	res := m.deps.DB.WithContext(c.Request.Context()).Exec(
		`UPDATE sys_agent_keys SET revoked_at = COALESCE(revoked_at, now()) WHERE id = ?`, id)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, httpx.NotFound("密钥不存在")
	}
	return nil, nil
}

// newAgentKey returns a fresh key ("bak_" + 256 random bits) and its hash.
func newAgentKey() (key, hash string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never fails (Go 1.24+)
	key = agentKeyPrefix + base64.RawURLEncoding.EncodeToString(b)
	return key, hashAgentKey(key)
}

func hashAgentKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func fieldError(field, msg string) error {
	return httpx.Validation([]httpx.FieldError{{Field: field, Message: msg}})
}
