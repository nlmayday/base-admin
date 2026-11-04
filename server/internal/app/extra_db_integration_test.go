//go:build integration

package app

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/testinfra"
)

func extraConfig(url string, required bool) config.ExtraDatabaseConfig {
	return config.ExtraDatabaseConfig{URL: url, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
		ConnMaxIdleTime: time.Minute, ConnectTimeout: time.Second, Required: required}
}

func TestExtraDatabases(t *testing.T) {
	main := testinfra.Database(t)
	// A port nothing listens on: unreachable at once.
	down := "postgres://u:p@127.0.0.1:1/none?sslmode=disable"
	main.Extra = map[string]config.ExtraDatabaseConfig{
		"game": extraConfig(main.URL, true),
		"logs": extraConfig(down, false),
	}
	cfg := config.Config{App: config.AppConfig{Name: "extra-test", Env: config.EnvTest},
		Database: main, Redis: testinfra.RedisDB(t)}
	ctx := context.Background()

	d, err := openDeps(ctx, cfg, zap.NewNop())
	require.NoError(t, err, "an unreachable optional database does not stop the start")
	t.Cleanup(func() { _ = d.close() })

	game := d.extraDB("game")
	require.NotNil(t, game)
	var one int
	require.NoError(t, game.Raw("SELECT 1").Scan(&one).Error)
	assert.Equal(t, 1, one)
	assert.NotNil(t, d.extraDB("logs"), "the pool exists and reconnects later")
	assert.NoError(t, d.missingExtraError())
	assert.Nil(t, d.extraDB("pay"))
	assert.ErrorContains(t, d.missingExtraError(), "ADMIN_DATABASE_EXTRA_PAY_URL")

	checks := d.extraChecks()
	require.Len(t, checks, 1, "only required databases count for readiness")
	assert.Equal(t, "extra_game", checks[0].Name())
	assert.NoError(t, checks[0].Check(ctx))

	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(d.extraMetrics()[0]))
	families, err := reg.Gather()
	require.NoError(t, err)
	assert.NotEmpty(t, families)
	assert.Len(t, d.extraMetrics(), 2)

	cfg.Database.Extra = map[string]config.ExtraDatabaseConfig{"game": extraConfig(down, true)}
	_, err = openDeps(ctx, cfg, zap.NewNop())
	assert.ErrorContains(t, err, "database.extra.game", "a required database must be reachable at startup")
}
