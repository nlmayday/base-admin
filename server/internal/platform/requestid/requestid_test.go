package requestid

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	a, b := New(), New()
	assert.True(t, strings.HasPrefix(a, "req_"))
	assert.Len(t, a, len("req_")+24)
	assert.NotEqual(t, a, b)
	assert.True(t, Valid(a))
}

func TestValid(t *testing.T) {
	cases := map[string]bool{
		"":                      false,
		"abc-DEF_123.x":         true,
		strings.Repeat("a", 64): true,
		strings.Repeat("a", 65): false,
		"has space":             false,
		"inject\r\nX-Evil: 1":   false,
		"<script>":              false,
		"中文":                    false,
	}
	for id, want := range cases {
		assert.Equal(t, want, Valid(id), "id %q", id)
	}
}

func TestContextRoundTrip(t *testing.T) {
	assert.Empty(t, From(context.Background()))
	ctx := With(context.Background(), "req_1")
	assert.Equal(t, "req_1", From(ctx))
}
