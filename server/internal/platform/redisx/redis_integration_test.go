//go:build integration

package redisx

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"base-admin/server/internal/platform/testinfra"
)

func TestOpen(t *testing.T) {
	client, err := Open(context.Background(), testinfra.Redis(t), zap.NewNop())
	require.NoError(t, err)
	defer func() { _ = client.Close() }()
	assert.NoError(t, client.Ping(context.Background()).Err())
}

func TestOpenUnreachable(t *testing.T) {
	cfg := testinfra.Redis(t)
	cfg.Addr = "127.0.0.1:1"
	cfg.DialTimeout = 200 * time.Millisecond
	_, err := Open(context.Background(), cfg, zap.NewNop())
	assert.ErrorContains(t, err, "ping redis")
}
