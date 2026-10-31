package main

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunUsageErrors(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", "postgres://u:p@127.0.0.1:1/db")
	t.Setenv("ADMIN_CONFIG", "")

	cases := map[string][]string{
		"unknown command":   {"explode"},
		"serve with args":   {"serve", "extra"},
		"migrate no action": {"migrate"},
		"unknown flag":      {"-nope"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			var stderr bytes.Buffer
			assert.Equal(t, 2, run(args, io.Discard, &stderr))
			assert.Contains(t, stderr.String(), "usage: admin")
		})
	}
}

func TestRunConfigError(t *testing.T) {
	t.Setenv("ADMIN_DATABASE_URL", "")
	t.Setenv("ADMIN_CONFIG", "")
	var stderr bytes.Buffer
	assert.Equal(t, 1, run([]string{"serve"}, io.Discard, &stderr))
	assert.Contains(t, stderr.String(), "database.url is required")
}

func TestIndent(t *testing.T) {
	assert.Equal(t, "a\n  b", indent("a\nb"))
}
