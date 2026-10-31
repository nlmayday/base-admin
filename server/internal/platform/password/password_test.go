package password

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fast keeps tests quick; production uses Default.
var fast = Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

func TestHashAndVerify(t *testing.T) {
	h, err := Hash("correct horse", fast)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(h, "$argon2id$v=19$m=1024,t=1,p=1$"))

	ok, err := Verify("correct horse", h)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = Verify("wrong horse", h)
	require.NoError(t, err)
	assert.False(t, ok)

	h2, err := Hash("correct horse", fast)
	require.NoError(t, err)
	assert.NotEqual(t, h, h2, "salts must differ")
}

func TestDefaultParamsRoundTrip(t *testing.T) {
	h, err := Hash("pw", Default)
	require.NoError(t, err)
	assert.Contains(t, h, "m=19456,t=2,p=1")
	ok, err := Verify("pw", h)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestVerifyMalformed(t *testing.T) {
	for _, bad := range []string{
		"", "plain", "$bcrypt$x$y$z$w", "$argon2id$v=18$m=1,t=1,p=1$AAAA$AAAA",
		"$argon2id$v=19$m=x,t=1,p=1$AAAA$AAAA", "$argon2id$v=19$m=1,t=1,p=1$!!$AAAA", "$argon2id$v=19$m=1,t=1,p=1$AAAA$",
	} {
		_, err := Verify("pw", bad)
		assert.ErrorIs(t, err, ErrMalformedHash, bad)
	}
}
