//go:build integration

package app

import (
	"time"

	"base-admin/server/internal/platform/config"
)

// Redis logical databases per package (see testinfra.RedisDB).
const redisDBApp = 1

func testAuthConfig() config.AuthConfig {
	return config.AuthConfig{
		CookieName: "sid", CookieSecure: false,
		AllowedOrigins: []string{testOrigin},
		IdleTimeout:    time.Hour, AbsoluteTimeout: 2 * time.Hour, TouchInterval: time.Minute,
		SnapshotTTL: time.Minute, ReconcileInterval: time.Second, LockTimeout: 2 * time.Second,
		LoginWindow: time.Minute, LoginMaxPerIP: 1000, LoginMaxPerAccount: 5,
	}
}

const testOrigin = "http://admin.test"
