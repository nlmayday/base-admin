package ops

import (
	"bufio"
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

type runtimeInfo struct {
	GoVersion    string     `json:"goVersion"`
	GOOS         string     `json:"goos"`
	GOARCH       string     `json:"goarch"`
	NumCPU       int        `json:"numCpu"`
	GOMAXPROCS   int        `json:"gomaxprocs"`
	Goroutines   int        `json:"goroutines"`
	HeapAlloc    uint64     `json:"heapAlloc"`
	HeapInuse    uint64     `json:"heapInuse"`
	HeapSys      uint64     `json:"heapSys"`
	HeapObjects  uint64     `json:"heapObjects"`
	StackInuse   uint64     `json:"stackInuse"`
	Sys          uint64     `json:"sys"`
	NextGC       uint64     `json:"nextGc"`
	NumGC        uint32     `json:"numGc"`
	PauseTotalMs float64    `json:"pauseTotalMs"`
	LastPauseMs  float64    `json:"lastPauseMs"`
	LastGCAt     *time.Time `json:"lastGcAt"`
}

type processInfo struct {
	InstanceID    string    `json:"instanceId"`
	Version       string    `json:"version"`
	PID           int       `json:"pid"`
	StartedAt     time.Time `json:"startedAt"`
	UptimeSeconds int64     `json:"uptimeSeconds"`
}

type dbPoolInfo struct {
	MaxOpenConnections int   `json:"maxOpenConnections"`
	OpenConnections    int   `json:"openConnections"`
	InUse              int   `json:"inUse"`
	Idle               int   `json:"idle"`
	WaitCount          int64 `json:"waitCount"`
	WaitDurationMs     int64 `json:"waitDurationMs"`
	MaxIdleClosed      int64 `json:"maxIdleClosed"`
	MaxIdleTimeClosed  int64 `json:"maxIdleTimeClosed"`
	MaxLifetimeClosed  int64 `json:"maxLifetimeClosed"`
}

type redisInfo struct {
	Available        bool   `json:"available"`
	Version          string `json:"version"`
	Mode             string `json:"mode"`
	UsedMemoryHuman  string `json:"usedMemoryHuman"`
	UsedMemoryPeak   string `json:"usedMemoryPeakHuman"`
	MaxMemoryHuman   string `json:"maxMemoryHuman"`
	ConnectedClients int    `json:"connectedClients"`
	UptimeSeconds    int64  `json:"uptimeSeconds"`
}

// appStats is the application view: Go runtime, process, pools.
type appStats struct {
	CollectedAt time.Time   `json:"collectedAt"`
	Runtime     runtimeInfo `json:"runtime"`
	Process     processInfo `json:"process"`
	Database    dbPoolInfo  `json:"database"`
	Redis       redisInfo   `json:"redis"`
}

func (mo *monitor) collectApp(ctx context.Context) (appStats, error) {
	now := time.Now()
	out := appStats{
		CollectedAt: now,
		Runtime:     readRuntime(),
		Process: processInfo{InstanceID: mo.instance, Version: mo.version, PID: os.Getpid(),
			StartedAt: mo.startedAt, UptimeSeconds: int64(now.Sub(mo.startedAt) / time.Second)},
	}
	if mo.sqlDB != nil {
		s := mo.sqlDB.Stats()
		out.Database = dbPoolInfo{
			MaxOpenConnections: s.MaxOpenConnections, OpenConnections: s.OpenConnections, InUse: s.InUse,
			Idle: s.Idle, WaitCount: s.WaitCount, WaitDurationMs: s.WaitDuration.Milliseconds(),
			MaxIdleClosed: s.MaxIdleClosed, MaxIdleTimeClosed: s.MaxIdleTimeClosed, MaxLifetimeClosed: s.MaxLifetimeClosed,
		}
	}
	out.Redis = mo.readRedis(ctx)
	return out, nil
}

func readRuntime() runtimeInfo {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	info := runtimeInfo{
		GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		NumCPU: runtime.NumCPU(), GOMAXPROCS: runtime.GOMAXPROCS(0), Goroutines: runtime.NumGoroutine(),
		HeapAlloc: ms.HeapAlloc, HeapInuse: ms.HeapInuse, HeapSys: ms.HeapSys, HeapObjects: ms.HeapObjects,
		StackInuse: ms.StackInuse, Sys: ms.Sys, NextGC: ms.NextGC, NumGC: ms.NumGC,
		PauseTotalMs: float64(ms.PauseTotalNs) / 1e6,
	}
	if ms.NumGC > 0 {
		info.LastPauseMs = float64(ms.PauseNs[(ms.NumGC+255)%256]) / 1e6
		last := time.Unix(0, int64(ms.LastGC)) // #nosec G115 -- nanoseconds since 1970 fit int64
		info.LastGCAt = &last
	}
	return info
}

// readRedis returns a subset of INFO. Failures are reported as unavailable
// rather than failing the whole response.
func (mo *monitor) readRedis(ctx context.Context) redisInfo {
	if mo.rdb == nil {
		return redisInfo{}
	}
	raw, err := mo.rdb.Info(ctx, "server", "memory", "clients").Result()
	if err != nil {
		mo.logger.Warn("redis INFO failed", zap.Error(err))
		return redisInfo{}
	}
	kv := parseRedisInfo(raw)
	clients, _ := strconv.Atoi(kv["connected_clients"])
	uptime, _ := strconv.ParseInt(kv["uptime_in_seconds"], 10, 64)
	return redisInfo{
		Available: true, Version: kv["redis_version"], Mode: kv["redis_mode"],
		UsedMemoryHuman: kv["used_memory_human"], UsedMemoryPeak: kv["used_memory_peak_human"],
		MaxMemoryHuman: kv["maxmemory_human"], ConnectedClients: clients, UptimeSeconds: uptime,
	}
}

// parseRedisInfo parses "key:value" lines, skipping "# Section" headers.
func parseRedisInfo(raw string) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(raw))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			out[k] = v
		}
	}
	return out
}
