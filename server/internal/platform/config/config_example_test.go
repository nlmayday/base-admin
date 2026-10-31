package config

import (
	"slices"
	"sort"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const examplePath = "../../../configs/config.example.yaml"

// TestExampleConfigListsEveryKey keeps configs/config.example.yaml in step
// with the defaults: every non-secret key appears, and nothing else.
func TestExampleConfigListsEveryKey(t *testing.T) {
	v := viper.New()
	v.SetConfigFile(examplePath)
	require.NoError(t, v.ReadInConfig())
	got := v.AllKeys()
	var want []string
	for key := range defaultValues() {
		if !slices.Contains(secretKeys, key) {
			want = append(want, key)
		}
	}
	sort.Strings(got)
	sort.Strings(want)
	assert.Equal(t, want, got, "add new config keys to configs/config.example.yaml")
}

// TestExampleConfigShowsDefaults loads the example and the bare defaults and
// expects the same configuration, so the documented values are the real ones.
func TestExampleConfigShowsDefaults(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", testDatabaseURL)
	fromExample, err := Load(examplePath)
	require.NoError(t, err)
	defaults, err := Load("")
	require.NoError(t, err)
	// The dev signing key is random per load.
	fromExample.File.SigningKey, defaults.File.SigningKey = "", ""
	assert.Equal(t, defaults, fromExample)
}
