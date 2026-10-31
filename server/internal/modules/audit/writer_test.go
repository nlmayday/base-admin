package audit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"base-admin/server/internal/modules/auth"
)

// fakeSink records batches; block, when set, holds every insert until it
// is closed, and failOn rejects rows with that username or path.
type fakeSink struct {
	mu      sync.Mutex
	logins  [][]loginRow
	ops     [][]opRow
	block   chan struct{}
	failOn  string
	inserts int
}

func (s *fakeSink) wait() {
	if s.block != nil {
		<-s.block
	}
}

func (s *fakeSink) insertLogins(_ context.Context, rows []loginRow) error {
	s.wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inserts++
	for _, r := range rows {
		if s.failOn != "" && r.Username == s.failOn {
			return errors.New("bad row")
		}
	}
	s.logins = append(s.logins, append([]loginRow(nil), rows...))
	return nil
}

func (s *fakeSink) insertOps(_ context.Context, rows []opRow) error {
	s.wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inserts++
	for _, r := range rows {
		if s.failOn != "" && r.Path == s.failOn {
			return errors.New("bad row")
		}
	}
	s.ops = append(s.ops, append([]opRow(nil), rows...))
	return nil
}

func (s *fakeSink) loginCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, b := range s.logins {
		n += len(b)
	}
	return n
}

func (s *fakeSink) opCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, b := range s.ops {
		n += len(b)
	}
	return n
}

// runWriter starts w.Run and returns a stop function that cancels it and
// waits until it returned.
func runWriter(w *Writer) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	return func() {
		cancel()
		<-done
	}
}

func login(name string) auth.LoginEvent {
	return auth.LoginEvent{Kind: auth.EventLogin, Username: name, UserID: 7, Success: true, IP: "10.0.0.1"}
}

func TestWriterFlushesOnShutdown(t *testing.T) {
	s := &fakeSink{}
	w := newWriter(WriterConfig{FlushInterval: time.Hour, BatchSize: 1000}, s, zap.NewNop())
	stop := runWriter(w)
	for i := range 10 {
		w.RecordLogin(context.Background(), login("u"+string(rune('a'+i))))
	}
	w.enqueue(entry{op: &opRow{Method: "POST", Path: "/x"}})
	stop()
	assert.Equal(t, 10, s.loginCount())
	assert.Equal(t, 1, s.opCount())
	assert.Zero(t, w.Dropped())

	// After shutdown nothing is accepted, and nothing blocks.
	start := time.Now()
	w.RecordLogin(context.Background(), login("late"))
	assert.Less(t, time.Since(start), 100*time.Millisecond)
	assert.Equal(t, uint64(1), w.Dropped())
	assert.Equal(t, 10, s.loginCount())
}

func TestWriterFlushesFullBatches(t *testing.T) {
	s := &fakeSink{}
	w := newWriter(WriterConfig{FlushInterval: time.Hour, BatchSize: 5}, s, zap.NewNop())
	stop := runWriter(w)
	defer stop()
	for range 5 {
		w.RecordLogin(context.Background(), login("u"))
	}
	require.Eventually(t, func() bool { return s.loginCount() == 5 }, time.Second, 5*time.Millisecond)
}

func TestWriterFlushesOnInterval(t *testing.T) {
	s := &fakeSink{}
	w := newWriter(WriterConfig{FlushInterval: 10 * time.Millisecond, BatchSize: 1000}, s, zap.NewNop())
	stop := runWriter(w)
	defer stop()
	w.RecordLogin(context.Background(), login("u"))
	require.Eventually(t, func() bool { return s.loginCount() == 1 }, time.Second, 5*time.Millisecond)
}

func TestWriterDropsWhenQueueFull(t *testing.T) {
	// No Run: the queue fills up and producers give up after EnqueueWait.
	s := &fakeSink{}
	w := newWriter(WriterConfig{QueueSize: 2, EnqueueWait: 10 * time.Millisecond}, s, zap.NewNop())
	w.RecordLogin(context.Background(), login("a"))
	w.RecordLogin(context.Background(), login("b"))
	start := time.Now()
	w.RecordLogin(context.Background(), login("c"))
	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed, 10*time.Millisecond, "waits briefly for space")
	assert.Less(t, elapsed, 500*time.Millisecond, "never blocks indefinitely")
	assert.Equal(t, uint64(1), w.Dropped())

	// Once Run starts, the queued rows are written.
	stop := runWriter(w)
	stop()
	assert.Equal(t, 2, s.loginCount())
}

func TestWriterOverflowWhileSinkIsSlow(t *testing.T) {
	s := &fakeSink{block: make(chan struct{})}
	w := newWriter(WriterConfig{QueueSize: 4, BatchSize: 1, EnqueueWait: time.Millisecond, FlushTimeout: time.Second}, s, zap.NewNop())
	stop := runWriter(w)
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() { w.RecordLogin(context.Background(), login("x")) })
	}
	wg.Wait() // producers return although the sink is stuck
	assert.Positive(t, w.Dropped())
	close(s.block)
	stop()
	assert.Equal(t, 50, s.loginCount()+int(w.Dropped()), "every row is either written or counted as dropped")
}

func TestWriterIsolatesBadRows(t *testing.T) {
	s := &fakeSink{failOn: "bad"}
	w := newWriter(WriterConfig{FlushInterval: time.Hour}, s, zap.NewNop())
	stop := runWriter(w)
	w.RecordLogin(context.Background(), login("good1"))
	w.RecordLogin(context.Background(), login("bad"))
	w.RecordLogin(context.Background(), login("good2"))
	stop()
	assert.Equal(t, 2, s.loginCount(), "good rows survive a failing batch")
	assert.Equal(t, uint64(1), w.failed.Load())
}

func TestRecordLoginMapsEvent(t *testing.T) {
	s := &fakeSink{}
	w := newWriter(WriterConfig{}, s, zap.NewNop())
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	w.RecordLogin(context.Background(), auth.LoginEvent{Kind: auth.EventLogin, Username: "bob\x00", Success: false,
		Reason: auth.ReasonBadCredentials, IP: "1.2.3.4", UserAgent: "ua", RequestID: "req_1", At: at})
	stop := runWriter(w)
	stop()
	require.Len(t, s.logins, 1)
	r := s.logins[0][0]
	assert.Nil(t, r.UserID, "unknown users are stored as NULL")
	assert.Equal(t, "bob", r.Username, "NUL bytes are removed")
	assert.Equal(t, at, r.CreatedAt)
	assert.Equal(t, auth.ReasonBadCredentials, r.Reason)
}

func TestWriterConfigDefaults(t *testing.T) {
	c := WriterConfig{EnqueueWait: -1}.withDefaults()
	assert.Equal(t, 4096, c.QueueSize)
	assert.Equal(t, 200, c.BatchSize)
	assert.Equal(t, time.Second, c.FlushInterval)
	assert.Zero(t, c.EnqueueWait, "negative wait means drop immediately")
	assert.Equal(t, 20*time.Millisecond, WriterConfig{}.withDefaults().EnqueueWait)
}
