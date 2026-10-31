package access

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLevels(t *testing.T) {
	assert.True(t, Public().Valid())
	assert.True(t, Authenticated().Valid())
	assert.True(t, Perm("a:b:view").Valid())
	assert.False(t, Level{}.Valid(), "zero level is invalid")
	assert.False(t, AnyPerm().Valid(), "permission level needs codes")

	assert.Equal(t, "public", Public().String())
	assert.Equal(t, "authenticated", Authenticated().String())
	assert.Equal(t, "perm(a:b:view|a:c:view)", AnyPerm("a:b:view", "a:c:view").String())
	assert.Equal(t, "invalid", Level{}.String())
}

func TestCodesAreCopied(t *testing.T) {
	src := []string{"a:b:view"}
	lvl := AnyPerm(src...)
	src[0] = "x:y:z"
	codes := lvl.Codes()
	codes[0] = "mutated"
	assert.Equal(t, []string{"a:b:view"}, lvl.Codes())
}

func TestAllows(t *testing.T) {
	has := func(code string) bool { return code == "a:c:view" }
	assert.True(t, AnyPerm("a:b:view", "a:c:view").Allows(has))
	assert.False(t, Perm("a:b:view").Allows(has))
}
