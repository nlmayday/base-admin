package ids

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
	"time"
)

// Next returns a new positive, time-ordered int64 id, snowflake style:
// 41 bits of milliseconds since 2024-01-01 UTC, 10 bits of node and 12 bits
// of sequence. The node is random per process, so ids of several API
// instances do not collide in practice; inject the project's own generator
// (the Deps.NewID of generated modules) when another service already
// assigns ids to the same table.
func Next() int64 { return defaultGenerator.next() }

const (
	epochMillis  = 1704067200000 // 2024-01-01T00:00:00Z
	nodeBits     = 10
	sequenceBits = 12
	maxSequence  = 1<<sequenceBits - 1
)

type generator struct {
	mu   sync.Mutex
	node int64
	last int64
	seq  int64
	now  func() int64
}

var defaultGenerator = newGenerator(func() int64 { return time.Now().UnixMilli() })

func newGenerator(now func() int64) *generator {
	var b [2]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails (Go 1.24+)
	return &generator{node: int64(binary.BigEndian.Uint16(b[:])) & (1<<nodeBits - 1), now: now}
}

func (g *generator) next() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	ms := g.now()
	if ms < g.last {
		ms = g.last // the clock went back: keep counting on the last millisecond
	}
	if ms == g.last {
		g.seq = (g.seq + 1) & maxSequence
		if g.seq == 0 {
			for ms <= g.last { // sequence exhausted: wait for the next millisecond
				ms = max(g.now(), g.last+1)
			}
		}
	} else {
		g.seq = 0
	}
	g.last = ms
	return (ms-epochMillis)<<(nodeBits+sequenceBits) | g.node<<sequenceBits | g.seq
}
