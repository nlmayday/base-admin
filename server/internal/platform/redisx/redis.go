// Package redisx opens the Redis client and enforces the deployment
// requirements from design doc 6.5.6.
package redisx

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"base-admin/server/internal/platform/config"
)

// Open creates a client, verifies connectivity and, when required, rejects
// eviction policies that could silently drop session generation keys.
func Open(ctx context.Context, cfg config.RedisConfig, logger *zap.Logger) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	if cfg.RequireSafeEvictionPolicy {
		if err := checkEvictionPolicy(ctx, client, cfg.DeclaredEvictionPolicy, logger); err != nil {
			_ = client.Close()
			return nil, err
		}
	}
	return client, nil
}

func checkEvictionPolicy(ctx context.Context, client *redis.Client, declared string, logger *zap.Logger) error {
	res, err := client.ConfigGet(ctx, "maxmemory-policy").Result()
	if err != nil {
		return resolveUnreadablePolicy(declared, err, logger)
	}
	return ValidateEvictionPolicy(res["maxmemory-policy"])
}

// resolveUnreadablePolicy handles servers that disable CONFIG, as managed
// Redis often does: the operator must declare the policy, which is then
// validated and logged as unverified (design doc 6.5.6).
func resolveUnreadablePolicy(declared string, readErr error, logger *zap.Logger) error {
	if declared == "" {
		return fmt.Errorf("cannot read redis maxmemory-policy; set redis.declared_eviction_policy: %w", readErr)
	}
	logger.Warn("cannot verify redis maxmemory-policy; trusting the declared value",
		zap.String("declared", declared), zap.Error(readErr))
	return ValidateEvictionPolicy(declared)
}

// ValidateEvictionPolicy accepts noeviction and volatile-* policies only.
// allkeys-* may evict keys without a TTL, such as session generations.
func ValidateEvictionPolicy(policy string) error {
	if policy == "noeviction" || strings.HasPrefix(policy, "volatile-") {
		return nil
	}
	return fmt.Errorf("redis maxmemory-policy %q is not allowed; use noeviction or volatile-*", policy)
}
