package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBizSectionPassesStrictChecks(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
	t.Setenv("ADMIN_BIZ_GAME__INNER_TOKEN", "s3cret")
	t.Setenv("ADMIN_BIZ_GAME__TIMEOUT", "3s")
	t.Setenv("ADMIN_BIZ_REGION", "sg")
	path := writeFile(t, `
biz:
  game:
    grpc_addr: game-server:9000
    retries: 3
    timeout: 1s
`)
	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "game-server:9000", cfg.Biz.String("game.grpc_addr"))
	assert.Equal(t, "s3cret", cfg.Biz.String("game.inner_token"))
	assert.Equal(t, "sg", cfg.Biz.String("region"))
	assert.Empty(t, cfg.Biz.String("game.missing"))

	var game struct {
		GRPCAddr   string        `mapstructure:"grpc_addr"`
		InnerToken string        `mapstructure:"inner_token"`
		Retries    int           `mapstructure:"retries"`
		Timeout    time.Duration `mapstructure:"timeout"`
	}
	require.NoError(t, cfg.Biz.Decode("game", &game))
	assert.Equal(t, "game-server:9000", game.GRPCAddr)
	assert.Equal(t, "s3cret", game.InnerToken)
	assert.Equal(t, 3, game.Retries)
	assert.Equal(t, 3*time.Second, game.Timeout, "the environment overrides the file")

	var strict struct {
		GRPCAddr string `mapstructure:"grpc_addr"`
	}
	assert.ErrorContains(t, cfg.Biz.Decode("game", &strict), "inner_token", "Decode is strict")

	kept := struct {
		Name string `mapstructure:"name"`
	}{Name: "default"}
	require.NoError(t, cfg.Biz.Decode("absent", &kept))
	assert.Equal(t, "default", kept.Name, "a missing section keeps the defaults")
}

func TestBizSecretsAreRefusedInFile(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
	for _, key := range []string{"inner_token", "password", "api_secret"} {
		path := writeFile(t, "biz:\n  game:\n    "+key+": x\n")
		_, err := Load(path)
		require.Error(t, err, key)
		assert.Contains(t, err.Error(), "ADMIN_BIZ_GAME__"+upper(key), key)
	}
	path := writeFile(t, "biz:\n  game:\n    token_ttl: 5m\n    public_key_id: k1\n")
	_, err := Load(path)
	assert.NoError(t, err, "only names ending in password, secret or token are secrets")
}

func TestBizEnvConflicts(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
	t.Setenv("ADMIN_BIZ_GAME", "flat")
	path := writeFile(t, "biz:\n  game:\n    grpc_addr: x\n")
	_, err := Load(path)
	assert.ErrorContains(t, err, "biz.game is a section")
}

func TestExtraDatabases(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
	t.Setenv("ADMIN_DATABASE_EXTRA_GAME_URL", "mysql://u:p@db:3306/game")
	t.Setenv("ADMIN_DATABASE_EXTRA_PAY_LOG_URL", "postgres://u:p@db/pay")
	path := writeFile(t, `
database:
  extra:
    game:
      max_open_conns: 30
      required: true
`)
	cfg, err := Load(path)
	require.NoError(t, err)
	game := cfg.Database.Extra["game"]
	assert.Equal(t, "mysql://u:p@db:3306/game", game.URL)
	assert.Equal(t, 30, game.MaxOpenConns)
	assert.Equal(t, 5, game.MaxIdleConns, "defaults fill what the file leaves out")
	assert.Equal(t, 5*time.Second, game.ConnectTimeout)
	assert.True(t, game.Required)
	pay := cfg.Database.Extra["pay_log"]
	assert.Equal(t, "postgres://u:p@db/pay", pay.URL, "the variable alone declares the database")
	assert.False(t, pay.Required)
	assert.Equal(t, 10, pay.MaxOpenConns)
}

func TestExtraDatabaseProblems(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
	_, err := Load(writeFile(t, "database:\n  extra:\n    game:\n      max_open_conns: 5\n"))
	assert.ErrorContains(t, err, "ADMIN_DATABASE_EXTRA_GAME_URL")

	_, err = Load(writeFile(t, "database:\n  extra:\n    game:\n      url: mysql://u:p@db/game\n"))
	assert.ErrorContains(t, err, "must not be set in the config file")

	_, err = Load(writeFile(t, "database:\n  extra:\n    game:\n      max_conns: 5\n"))
	assert.ErrorContains(t, err, "max_conns", "unknown keys of an extra database are errors")

	t.Setenv("ADMIN_DATABASE_EXTRA_GAME_URL", "redis://x")
	_, err = Load("")
	assert.ErrorContains(t, err, "postgres:// or mysql://")
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}
