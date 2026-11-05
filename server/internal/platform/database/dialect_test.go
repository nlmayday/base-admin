package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/config"
)

func TestParseDialect(t *testing.T) {
	for in, want := range map[string]Dialect{
		"postgres://u:p@h/db":   Postgres,
		"postgresql://u:p@h/db": Postgres,
		"MYSQL://u:p@h/db":      MySQL,
	} {
		got, err := ParseDialect(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"", "u:p@tcp(h)/db", "sqlite://x"} {
		_, err := ParseDialect(bad)
		assert.Error(t, err, bad)
	}
}

func TestMySQLConfig(t *testing.T) {
	cfg := config.DatabaseConfig{
		URL:              "mysql://app:p%40ss%3Aw%2Frd@db.internal/base_admin?tls=skip-verify&readTimeout=3s&lock_wait_timeout=20",
		ConnectTimeout:   2 * time.Second,
		StatementTimeout: 15 * time.Second,
	}
	mc, err := mysqlConfig(cfg, "base-admin", false)
	require.NoError(t, err)
	assert.Equal(t, "app", mc.User)
	assert.Equal(t, "p@ss:w/rd", mc.Passwd, "percent-escapes and separators survive")
	assert.Equal(t, "db.internal:3306", mc.Addr, "the default port is added")
	assert.Equal(t, "base_admin", mc.DBName)
	assert.Equal(t, "skip-verify", mc.TLSConfig)
	assert.Equal(t, 3*time.Second, mc.ReadTimeout)
	assert.Equal(t, "20", mc.Params["lock_wait_timeout"], "unknown parameters are session variables")
	assert.True(t, mc.ParseTime)
	assert.Equal(t, time.UTC, mc.Loc)
	assert.Equal(t, MySQLCollation, mc.Collation)
	assert.True(t, mc.ClientFoundRows)
	assert.False(t, mc.MultiStatements)
	assert.Equal(t, "'+00:00'", mc.Params["time_zone"])
	assert.Equal(t, "'READ-COMMITTED'", mc.Params["transaction_isolation"])
	assert.Equal(t, "15000", mc.Params["max_execution_time"])
	assert.Contains(t, mc.Params["sql_mode"], "ANSI_QUOTES")

	migrate, err := mysqlConfig(cfg, "base-admin", true)
	require.NoError(t, err)
	assert.True(t, migrate.MultiStatements)

	cfg.StatementTimeout = 0
	mc, err = mysqlConfig(cfg, "base-admin", false)
	require.NoError(t, err)
	assert.NotContains(t, mc.Params, "max_execution_time")
}

func TestMySQLConfigRejectsBadURLs(t *testing.T) {
	for _, bad := range []string{
		"mysql://u:secret@h",
		"mysql://u:secret@h/a/b",
		"mysql://u:secret@h/db?parseTime=false",
		"mysql://u:secret@h/db?sql_mode=ANSI",
		"mysql://u:secret@h/db?multiStatements=true",
		"mysql://localhost/base_admin",
		"mysql://:secret@h/db",
		"mysql://u:secret@/db",
		"mysql://u:secret@:3306/db",
	} {
		_, err := mysqlConfig(config.DatabaseConfig{URL: bad}, "x", false)
		require.Error(t, err, bad)
		assert.NotContains(t, err.Error(), "secret", bad)
	}
}

func TestMySQLDuplicateKey(t *testing.T) {
	assert.Equal(t, "sys_users_username_uq",
		mysqlDuplicateKey("Duplicate entry 'alice' for key 'sys_users.sys_users_username_uq'"))
	assert.Equal(t, "PRIMARY", mysqlDuplicateKey("Duplicate entry '1' for key 'PRIMARY'"))
	assert.Equal(t, "x_uq", mysqlDuplicateKey("Duplicate entry 'it''s for key ' for key 't.x_uq'"))
	assert.Empty(t, mysqlDuplicateKey("something else"))
}

func TestTextArrayScansJSON(t *testing.T) {
	var a TextArray
	require.NoError(t, a.Scan([]byte(`["a","b \"c\""]`)))
	assert.Equal(t, TextArray{"a", `b "c"`}, a)
	require.NoError(t, a.Scan("[]"))
	assert.Equal(t, TextArray{}, a)
	assert.Error(t, a.Scan("[broken"))
}

func TestMySQLVersionProblem(t *testing.T) {
	for v, ok := range map[string]bool{
		"8.0.46": true, "8.0.30": true, "8.4.13": true, "9.1.0": true, "8.0.30-log": true,
		"8.0.29": false, "5.7.44-log": false, "10.11.6-MariaDB": false, "x": false,
	} {
		err := mysqlVersionProblem(v)
		if ok {
			assert.NoError(t, err, v)
		} else {
			assert.Error(t, err, v)
		}
	}
}
