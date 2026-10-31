package redisx

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestValidateEvictionPolicy(t *testing.T) {
	cases := map[string]bool{
		"noeviction":      true,
		"volatile-lru":    true,
		"volatile-lfu":    true,
		"volatile-ttl":    true,
		"volatile-random": true,
		"allkeys-lru":     false,
		"allkeys-lfu":     false,
		"allkeys-random":  false,
		"":                false,
	}
	for policy, ok := range cases {
		err := ValidateEvictionPolicy(policy)
		if ok {
			assert.NoError(t, err, policy)
		} else {
			assert.Error(t, err, policy)
		}
	}
}

func TestResolveUnreadablePolicy(t *testing.T) {
	readErr := errors.New("ERR unknown command 'CONFIG'")
	core, logs := observer.New(zapcore.WarnLevel)
	logger := zap.New(core)

	err := resolveUnreadablePolicy("", readErr, logger)
	require.Error(t, err, "an undeclared policy must fail closed")
	assert.ErrorIs(t, err, readErr)

	require.NoError(t, resolveUnreadablePolicy("noeviction", readErr, logger))
	assert.Equal(t, 1, logs.FilterMessage("cannot verify redis maxmemory-policy; trusting the declared value").Len())

	assert.Error(t, resolveUnreadablePolicy("allkeys-lru", readErr, logger))
}
