//go:build integration

package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/database"
	"base-admin/server/internal/platform/dbmigrate"
	"base-admin/server/internal/platform/testinfra"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func integrationConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		App:       config.AppConfig{Name: "base-admin-test", Env: config.EnvTest},
		HTTP:      testHTTPConfig(),
		Database:  testinfra.Database(t),
		Redis:     testinfra.RedisDB(t, redisDBApp),
		Metrics:   config.MetricsConfig{Enabled: true},
		Auth:      testAuthConfig(),
		Bootstrap: config.BootstrapConfig{AdminUsername: "admin", AdminPassword: "bootstrap-pass-123"},
		File: config.FileConfig{
			Driver: "local", LocalDir: t.TempDir(), SignedURLTTL: 10 * time.Minute,
			MaxUploadBytes: 100 << 20, SigningKey: "test-signing-key-0123456789abcdef0123",
		},
	}
}

func TestMigrateCommand(t *testing.T) {
	cfg := integrationConfig(t)
	ctx := context.Background()
	probe, err := database.OpenSQL(ctx, cfg.Database, "probe")
	require.NoError(t, err)
	defer func() { _ = probe.Close() }()

	migrate := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := Migrate(ctx, cfg, zap.NewNop(), &out, args)
		return out.String(), err
	}
	tableExists := func() bool {
		var exists bool
		require.NoError(t, probe.QueryRow("SELECT to_regclass('sys_auth_version') IS NOT NULL").Scan(&exists))
		return exists
	}

	out, err := migrate("up")
	require.NoError(t, err)
	assert.Equal(t, latestStatus(t), out)
	assert.True(t, tableExists(), "up must create the schema")

	out, err = migrate("version")
	require.NoError(t, err)
	assert.Equal(t, latestStatus(t), out)

	out, err = migrate("down", strconv.Itoa(int(latestVersion(t))))
	require.NoError(t, err)
	assert.Equal(t, "version=0 dirty=false applied=false\n", out)
	assert.False(t, tableExists(), "down must drop the schema")

	_, err = migrate("force", "999")
	assert.ErrorContains(t, err, "does not exist", "forcing an unknown version must be refused")

	out, err = migrate("force", "1")
	require.NoError(t, err)
	assert.Equal(t, "version=1 dirty=false applied=true\n", out)
	assert.False(t, tableExists(), "force must not run migrations")

	out, err = migrate("force", "-1")
	require.NoError(t, err)
	assert.Equal(t, "version=0 dirty=false applied=false\n", out)
	_, err = migrate("up")
	require.NoError(t, err, "up must work again after force -1")
	assert.True(t, tableExists())

	_, err = migrate("down")
	assert.ErrorIs(t, err, ErrUsage)
}

func TestMigrateVersionFailsOnDirtySchema(t *testing.T) {
	cfg := integrationConfig(t)
	ctx := context.Background()
	require.NoError(t, Migrate(ctx, cfg, zap.NewNop(), io.Discard, []string{"up"}))

	probe, err := database.OpenSQL(ctx, cfg.Database, "probe")
	require.NoError(t, err)
	defer func() { _ = probe.Close() }()
	_, err = probe.Exec("UPDATE schema_migrations SET dirty = true")
	require.NoError(t, err)

	var out bytes.Buffer
	err = Migrate(ctx, cfg, zap.NewNop(), &out, []string{"version"})
	assert.ErrorIs(t, err, ErrDirtySchema)
	assert.Equal(t, strings.Replace(latestStatus(t), "dirty=false", "dirty=true", 1), out.String())
}

func TestMigrateCommandUnreachableDatabase(t *testing.T) {
	cfg := integrationConfig(t)
	cfg.Database.URL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"
	cfg.Database.ConnectTimeout = 500 * time.Millisecond
	assert.Error(t, Migrate(context.Background(), cfg, zap.NewNop(), io.Discard, []string{"up"}))
}

func TestServeRefusesUnmigratedDatabase(t *testing.T) {
	cfg := integrationConfig(t)
	cfg.HTTP.Addr = freeAddr(t)
	cfg.Metrics.Enabled = false
	assert.ErrorContains(t, Serve(context.Background(), cfg, zap.NewNop()), "migrate up")
}

func TestServeLifecycle(t *testing.T) {
	cfg := integrationConfig(t)
	require.NoError(t, Migrate(context.Background(), cfg, zap.NewNop(), io.Discard, []string{"up"}))
	cfg.HTTP.Addr = freeAddr(t)
	cfg.Metrics.Addr = freeAddr(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg, zap.NewNop()) }()

	require.Eventually(t, func() bool {
		return statusOf("http://"+cfg.HTTP.Addr+"/health/ready") == http.StatusOK
	}, 10*time.Second, 50*time.Millisecond)
	assert.Equal(t, http.StatusOK, statusOf("http://"+cfg.Metrics.Addr+"/metrics"))
	assert.Equal(t, http.StatusNotFound, statusOf("http://"+cfg.HTTP.Addr+"/metrics"),
		"metrics must not be exposed on the API listener")

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not stop after cancellation")
	}
}

func TestServeFailsWhenPortIsBusy(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	cfg := integrationConfig(t)
	require.NoError(t, Migrate(context.Background(), cfg, zap.NewNop(), io.Discard, []string{"up"}))
	cfg.HTTP.Addr = ln.Addr().String()
	cfg.Metrics.Enabled = false
	assert.ErrorContains(t, Serve(context.Background(), cfg, zap.NewNop()), "listen api")
}

func statusOf(url string) int {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return 0
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func latestVersion(t *testing.T) uint {
	t.Helper()
	v, err := dbmigrate.Latest()
	require.NoError(t, err)
	return v
}

func latestStatus(t *testing.T) string {
	return fmt.Sprintf("version=%d dirty=false applied=true\n", latestVersion(t))
}
