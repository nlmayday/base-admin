// Package ids provides the entity ID type. IDs are bigint in the database
// and strings in JSON so that JavaScript never loses precision (design doc
// 8.1).
package ids

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// ID is a positive entity identifier.
type ID int64

// Parse parses a decimal ID such as a path parameter.
func Parse(s string) (ID, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid id %q", s)
	}
	return ID(n), nil
}

func (id ID) String() string { return strconv.FormatInt(int64(id), 10) }

// MarshalJSON encodes the ID as a JSON string.
func (id ID) MarshalJSON() ([]byte, error) {
	return []byte(`"` + id.String() + `"`), nil
}

// UnmarshalJSON accepts only a JSON string, so clients cannot silently send
// a rounded number.
func (id *ID) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return errors.New("id must be a string")
	}
	parsed, err := Parse(s)
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

// Int64s converts IDs for SQL parameters.
func Int64s(in []ID) []int64 {
	out := make([]int64, len(in))
	for i, id := range in {
		out[i] = int64(id)
	}
	return out
}

// FromInt64s converts SQL results to IDs.
func FromInt64s(in []int64) []ID {
	out := make([]ID, len(in))
	for i, n := range in {
		out[i] = ID(n)
	}
	return out
}
