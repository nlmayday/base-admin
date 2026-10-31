package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/config"
)

func TestOpenSQLParseErrorDoesNotLeakPassword(t *testing.T) {
	cases := []string{
		"postgres://admin:supersecret@localhost:notaport/db",
		"postgres://admin:supersecret@localhost:5432/db?sslmode=bogus",
		"postgres://admin:super%zzsecret@localhost:5432/db",
	}
	for _, url := range cases {
		_, err := OpenSQL(context.Background(), config.DatabaseConfig{URL: url, ConnectTimeout: time.Second}, "test")
		require.Error(t, err, url)
		assert.NotContains(t, err.Error(), "supersecret", url)
		assert.NotContains(t, err.Error(), "zzsecret", url)
	}
}
