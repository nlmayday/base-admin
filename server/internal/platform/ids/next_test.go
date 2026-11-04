package ids

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNextIsUniqueAndOrdered(t *testing.T) {
	const n = 20000
	seen := make(map[int64]bool, n)
	prev := int64(0)
	for range n {
		id := Next()
		require.Positive(t, id)
		require.Greater(t, id, prev, "time-ordered within a process")
		require.False(t, seen[id])
		seen[id], prev = true, id
	}
}

func TestNextIsSafeForConcurrentUse(t *testing.T) {
	var mu sync.Mutex
	seen := map[int64]bool{}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 2000 {
				id := Next()
				mu.Lock()
				assert.False(t, seen[id])
				seen[id] = true
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	assert.Len(t, seen, 16000)
}

func TestNextSurvivesClockGoingBack(t *testing.T) {
	clock := int64(1_800_000_000_000)
	g := newGenerator(func() int64 { return clock })
	a := g.next()
	clock -= 5000
	b := g.next()
	assert.Greater(t, b, a)
}
