package ids

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSON(t *testing.T) {
	type payload struct {
		ID     ID   `json:"id"`
		Parent *ID  `json:"parent"`
		List   []ID `json:"list"`
	}
	big := ID(9007199254740993) // beyond JS safe integers
	out, err := json.Marshal(payload{ID: big, List: []ID{1, 2}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"9007199254740993","parent":null,"list":["1","2"]}`, string(out))

	var in payload
	require.NoError(t, json.Unmarshal([]byte(`{"id":"9007199254740993","parent":"5","list":["3"]}`), &in))
	assert.Equal(t, big, in.ID)
	assert.Equal(t, ID(5), *in.Parent)

	for _, bad := range []string{`{"id":12}`, `{"id":"0"}`, `{"id":"-1"}`, `{"id":"abc"}`} {
		assert.Error(t, json.Unmarshal([]byte(bad), &in), bad)
	}
}

func TestParseAndConvert(t *testing.T) {
	id, err := Parse("42")
	require.NoError(t, err)
	assert.Equal(t, "42", id.String())
	_, err = Parse("4x")
	assert.Error(t, err)
	assert.Equal(t, []int64{1, 2}, Int64s([]ID{1, 2}))
	assert.Equal(t, []ID{3}, FromInt64s([]int64{3}))
}
