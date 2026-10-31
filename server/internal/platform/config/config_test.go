package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDatabaseURL = "postgres://u:p@localhost:5432/db?sslmode=disable"

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)

	cfg, err := Load("")
	require.NoError(t, err)

	assert.Equal(t, EnvDev, cfg.App.Env)
	assert.Equal(t, ":8080", cfg.HTTP.Addr)
	assert.Equal(t, 20*time.Second, cfg.HTTP.RequestTimeout)
	assert.Equal(t, int64(2<<20), cfg.HTTP.MaxBodyBytes)
	assert.Empty(t, cfg.HTTP.TrustedProxies)
	assert.Equal(t, testDatabaseURL, cfg.Database.URL)
	assert.True(t, cfg.Redis.RequireSafeEvictionPolicy)
}

func TestLoadEnvOverridesFile(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
	t.Setenv("ADMIN_HTTP_ADDR", ":9000")
	t.Setenv("ADMIN_HTTP_TRUSTED_PROXIES", "10.0.0.0/8,192.168.1.10")
	t.Setenv("ADMIN_REDIS_PASSWORD", "from-env")
	path := writeFile(t, `
http:
  addr: ":7000"
  request_timeout: 5s
log:
  level: debug
`)

	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, ":9000", cfg.HTTP.Addr, "env must win over file")
	assert.Equal(t, 5*time.Second, cfg.HTTP.RequestTimeout, "file must win over defaults")
	assert.Equal(t, "debug", cfg.Log.Level)
	assert.Equal(t, []string{"10.0.0.0/8", "192.168.1.10"}, cfg.HTTP.TrustedProxies)
	assert.Equal(t, "from-env", cfg.Redis.Password)
}

func TestLoadRejectsSecretsInFile(t *testing.T) {
	cases := map[string]string{
		"database url":   "database:\n  url: postgres://x\n",
		"redis password": "redis:\n  password: secret\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
			_, err := Load(writeFile(t, content))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must not be set in the config file")
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", "")
	_, err := Load("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database.url is required")
}

func validConfig(t *testing.T) Config {
	t.Helper()
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
	cfg, err := Load("")
	require.NoError(t, err)
	return cfg
}

func TestValidateReportsAllErrors(t *testing.T) {
	cfg := validConfig(t)
	cfg.App.Env = "staging"
	cfg.HTTP.Addr = "8080"
	cfg.HTTP.RequestTimeout = cfg.HTTP.WriteTimeout
	cfg.HTTP.TrustedProxies = []string{"not-an-ip"}
	cfg.Database.MaxIdleConns = cfg.Database.MaxOpenConns + 1
	cfg.Redis.PoolSize = 0
	cfg.Log.Format = "xml"
	cfg.Metrics.Addr = cfg.HTTP.Addr

	err := cfg.Validate()
	require.Error(t, err)
	for _, want := range []string{
		"app.env", "http.addr", "http.request_timeout", "http.trusted_proxies",
		"database.max_idle_conns", "redis.pool_size", "log.format", "metrics.addr",
	} {
		assert.Contains(t, err.Error(), want)
	}
}

func TestValidateBoundaries(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"negative drain delay", func(c *Config) { c.HTTP.DrainDelay = -time.Second }, "http.drain_delay"},
		{"zero body limit", func(c *Config) { c.HTTP.MaxBodyBytes = 0 }, "http.max_body_bytes"},
		{"negative statement timeout", func(c *Config) { c.Database.StatementTimeout = -1 }, "database.statement_timeout"},
		{"zero max open conns", func(c *Config) { c.Database.MaxOpenConns = 0 }, "database.max_open_conns"},
		{"negative redis db", func(c *Config) { c.Redis.DB = -1 }, "redis.db"},
		{"bad log level", func(c *Config) { c.Log.Level = "loud" }, "log.level"},
		{"empty app name", func(c *Config) { c.App.Name = "" }, "app.name"},
		{"zero read timeout", func(c *Config) { c.HTTP.ReadTimeout = 0 }, "http.read_timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig(t)
			tc.mutate(&cfg)
			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidateAcceptsDisabledMetricsAndZeroStatementTimeout(t *testing.T) {
	cfg := validConfig(t)
	cfg.Metrics.Enabled = false
	cfg.Metrics.Addr = ""
	cfg.Database.StatementTimeout = 0
	cfg.HTTP.TrustedProxies = []string{"127.0.0.1", "10.0.0.0/8", "::1"}
	assert.NoError(t, cfg.Validate())
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
	_, err := Load(writeFile(t, "http:\n  trusted_proxeis: [\"10.0.0.0/8\"]\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trusted_proxeis")
}

func TestLoadRejectsNumericDurations(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
	for _, content := range []string{"http:\n  shutdown_timeout: 30\n", "http:\n  read_timeout: 1.5\n"} {
		_, err := Load(writeFile(t, content))
		require.Error(t, err, content)
		assert.Contains(t, err.Error(), "must be a string with a unit")
	}
}

func TestLoadRejectsUnitlessDurationFromEnv(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
	t.Setenv("ADMIN_HTTP_SHUTDOWN_TIMEOUT", "30")
	_, err := Load("")
	require.Error(t, err)
}

func TestValidateNetworkAndStopRules(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"catch-all ipv4 proxy", func(c *Config) { c.HTTP.TrustedProxies = []string{"0.0.0.0/0"} }, "would trust every client"},
		{"catch-all ipv6 proxy", func(c *Config) { c.HTTP.TrustedProxies = []string{"::/0"} }, "would trust every client"},
		{"zoned proxy", func(c *Config) { c.HTTP.TrustedProxies = []string{"fe80::1%eth0"} }, "must not have a zone"},
		{"non-numeric port", func(c *Config) { c.Redis.Addr = "localhost:redis" }, "invalid port"},
		{"port out of range", func(c *Config) { c.HTTP.Addr = ":70000" }, "invalid port"},
		{"nanosecond timeout", func(c *Config) { c.HTTP.ReadHeaderTimeout = 5 }, "must be at least 1ms"},
		{"stop too long", func(c *Config) {
			c.HTTP.DrainDelay = 3 * time.Minute
			c.HTTP.ShutdownTimeout = 3 * time.Minute
		}, "must not exceed 5m0s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig(t)
			tc.mutate(&cfg)
			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidateAuthAndBootstrap(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"host prefix without secure", func(c *Config) { c.Auth.CookieName = "__Host-sid"; c.Auth.CookieSecure = false }, "requires auth.cookie_secure"},
		{"prod without secure", func(c *Config) { c.App.Env = EnvProd; c.Auth.CookieSecure = false }, "must be true in prod"},
		{"prod without host prefix", func(c *Config) { c.App.Env = EnvProd; c.Auth.CookieSecure = true }, "__Host- prefix in prod"},
		{"prod with localhost origin", func(c *Config) { c.App.Env = EnvProd }, `must not contain "http://localhost:8848" in prod`},
		{"empty cookie name", func(c *Config) { c.Auth.CookieName = "" }, "auth.cookie_name is required"},
		{"idle beyond absolute", func(c *Config) { c.Auth.IdleTimeout = 13 * time.Hour }, "must not exceed auth.absolute_timeout"},
		{"touch beyond idle", func(c *Config) { c.Auth.TouchInterval = 3 * time.Hour }, "touch_interval must be shorter"},
		{"no origins", func(c *Config) { c.Auth.AllowedOrigins = nil }, "must list the frontend origin"},
		{"origin with path", func(c *Config) { c.Auth.AllowedOrigins = []string{"https://a.example/app"} }, "must be scheme://host[:port]"},
		{"origin bad scheme", func(c *Config) { c.Auth.AllowedOrigins = []string{"ftp://a.example"} }, "must be scheme://host[:port]"},
		{"zero login limit", func(c *Config) { c.Auth.LoginMaxPerIP = 0 }, "must be positive"},
		{"short bootstrap password", func(c *Config) { c.Bootstrap.AdminPassword = "short" }, "12–72 characters"},
		{"empty bootstrap username", func(c *Config) { c.Bootstrap.AdminUsername = "" }, "admin_username is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig(t)
			tc.mutate(&cfg)
			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestLoadRejectsBootstrapPasswordInFile(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
	_, err := Load(writeFile(t, "bootstrap:\n  admin_password: \"supersecret123\"\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ADMIN_BOOTSTRAP_ADMIN_PASSWORD")
	assert.NotContains(t, err.Error(), "supersecret123")
}

func TestValidateProdAuthAccepted(t *testing.T) {
	cfg := validConfig(t)
	cfg.App.Env = EnvProd
	cfg.Auth.CookieName = "__Host-sid"
	cfg.Auth.CookieSecure = true
	cfg.Auth.AllowedOrigins = []string{"https://admin.example.com"}
	assert.NoError(t, cfg.Validate())
}
