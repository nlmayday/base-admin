package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextArrayRoundTrip(t *testing.T) {
	v, err := TextArray{"iam:user:view", `a "quoted", \ value`}.Value()
	require.NoError(t, err)
	assert.Equal(t, `{"iam:user:view","a \"quoted\", \\ value"}`, v)

	var got TextArray
	require.NoError(t, got.Scan(v))
	assert.Equal(t, TextArray{"iam:user:view", `a "quoted", \ value`}, got)

	require.NoError(t, got.Scan([]byte("{}")))
	assert.Equal(t, TextArray{}, got)
	require.NoError(t, got.Scan("{a,b}"))
	assert.Equal(t, TextArray{"a", "b"}, got)
	require.NoError(t, got.Scan(nil))
	assert.Nil(t, got)
	assert.Error(t, got.Scan("not-an-array"))
	assert.Error(t, got.Scan(42))
	assert.Equal(t, "text[]", TextArray{}.GormDataType())
}
