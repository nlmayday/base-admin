//go:build integration

package ops

import (
	"net/http"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitorEndpoints(t *testing.T) {
	e := newEnv(t)
	a := e.newInstance("inst-a", nil)

	r := a.do(http.MethodGet, "/monitor/server", nil)
	require.Equal(t, http.StatusOK, r.Status, string(r.Data))
	srv := decode[serverStats](t, r)
	assert.NotEmpty(t, srv.Host.Hostname)
	assert.Equal(t, runtime.GOOS, srv.Host.OS)
	assert.Positive(t, srv.Host.UptimeSeconds)
	assert.Positive(t, srv.CPU.LogicalCores)
	assert.Len(t, srv.CPU.PerCore, srv.CPU.LogicalCores)
	assert.GreaterOrEqual(t, srv.CPU.Percent, 0.0)
	assert.LessOrEqual(t, srv.CPU.Percent, 100.0)
	assert.Positive(t, srv.Memory.Total)
	assert.Positive(t, srv.Memory.Percent)
	require.NotEmpty(t, srv.Disks)
	for _, d := range srv.Disks {
		assert.NotContains(t, []string{"proc", "sysfs", "tmpfs", "devfs", "devtmpfs"}, d.FSType)
		assert.Positive(t, d.Total)
	}
	if runtime.GOOS != "windows" {
		assert.NotNil(t, srv.Load)
	}

	// Within the cache TTL the same snapshot is returned.
	again := decode[serverStats](t, a.do(http.MethodGet, "/monitor/server", nil))
	assert.Equal(t, srv.CollectedAt.UnixNano(), again.CollectedAt.UnixNano())

	r = a.do(http.MethodGet, "/monitor/app", nil)
	require.Equal(t, http.StatusOK, r.Status, string(r.Data))
	app := decode[appStats](t, r)
	assert.Equal(t, runtime.Version(), app.Runtime.GoVersion)
	assert.Positive(t, app.Runtime.Goroutines)
	assert.Positive(t, app.Runtime.HeapAlloc)
	assert.Equal(t, "inst-a", app.Process.InstanceID)
	assert.Equal(t, "test", app.Process.Version)
	assert.GreaterOrEqual(t, app.Process.UptimeSeconds, int64(59))
	assert.Equal(t, 5, app.Database.MaxOpenConnections)
	assert.Positive(t, app.Database.OpenConnections)
	assert.True(t, app.Redis.Available)
	assert.NotEmpty(t, app.Redis.Version)
	assert.NotEmpty(t, app.Redis.UsedMemoryHuman)
	assert.Positive(t, app.Redis.ConnectedClients)
	assert.Positive(t, app.Redis.UptimeSeconds)
}
