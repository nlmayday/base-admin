package export

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTimeCells(t *testing.T) {
	assert.Equal(t, "创建时间(UTC)", TimeHeader("创建时间"))
	at := time.Date(2026, 3, 1, 16, 0, 0, 0, time.FixedZone("GMT+8", 8*3600))
	assert.Equal(t, "2026-03-01 08:00:00", Time(at), "written in UTC")
	assert.Empty(t, Time(time.Time{}))
}
