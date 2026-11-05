package config

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A raw password with URL-special characters reaches the driver unchanged:
// the database container and the API read the same value.
func TestLoadDatabasePassword(t *testing.T) {
	const raw = "p@ss:w/rd#1%40?&"
	for _, base := range []string{
		"mysql://admin@mysql:3306/base_admin",
		"postgres://admin@postgres:5432/base_admin?sslmode=disable",
	} {
		t.Run(base, func(t *testing.T) {
			t.Setenv("ADMIN_DATABASE_URL", base)
			t.Setenv("ADMIN_DATABASE_PASSWORD", raw)
			cfg, err := Load("")
			require.NoError(t, err)
			u, err := url.Parse(cfg.Database.URL)
			require.NoError(t, err)
			pw, _ := u.User.Password()
			assert.Equal(t, raw, pw)
			assert.Equal(t, "admin", u.User.Username())
			assert.Empty(t, cfg.Database.Password, "cleared once merged")
		})
	}
}

func TestLoadDatabasePasswordUnset(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
	t.Setenv("ADMIN_DATABASE_PASSWORD", "")
	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, testDatabaseURL, cfg.Database.URL, "the URL is used as it is")
}

func TestLoadDatabasePasswordConflicts(t *testing.T) {
	for name, dbURL := range map[string]string{
		"password in the url too": "mysql://admin:secret@mysql:3306/base_admin",
		"no user name":            "mysql://mysql:3306/base_admin",
		"malformed url":           "mysql://admin@mysql:badport\x7f/base_admin",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("ADMIN_DATABASE_URL", dbURL)
			t.Setenv("ADMIN_DATABASE_PASSWORD", "hunter2-raw")
			_, err := Load("")
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "secret")
			assert.NotContains(t, err.Error(), "hunter2")
		})
	}
}
