package audit

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"base-admin/server/internal/modules/auth"
)

// WriterConfig tunes the asynchronous log writer. Zero values use defaults.
type WriterConfig struct {
	// QueueSize bounds the number of pending log rows (default 4096).
	QueueSize int
	// BatchSize is the maximum number of rows per INSERT (default 200).
	BatchSize int
	// FlushInterval flushes a partial batch (default 1s).
	FlushInterval time.Duration
	// EnqueueWait is how long a producer waits for queue space before the
	// row is dropped (default 20ms). Producers never block longer.
	EnqueueWait time.Duration
	// FlushTimeout bounds one batch insert, including the final flush on
	// shutdown (default 5s).
	FlushTimeout time.Duration
}

func (c WriterConfig) withDefaults() WriterConfig {
	if c.QueueSize <= 0 {
		c.QueueSize = 4096
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 200
	}
	if c.FlushInterval <= 0 {
		c.FlushInterval = time.Second
	}
	if c.EnqueueWait < 0 {
		c.EnqueueWait = 0
	} else if c.EnqueueWait == 0 {
		c.EnqueueWait = 20 * time.Millisecond
	}
	if c.FlushTimeout <= 0 {
		c.FlushTimeout = 5 * time.Second
	}
	return c
}

// loginRow is one sys_login_logs row.
type loginRow struct {
	Kind      string
	Username  string
	UserID    *int64
	Success   bool
	Reason    string
	IP        string
	UserAgent string
	RequestID string
	CreatedAt time.Time
}

// opRow is one sys_operation_logs row.
type opRow struct {
	UserID       *int64
	Module       string
	Method       string
	Route        string
	Path         string
	Summary      string
	Status       int
	Code         int
	Success      bool
	DurationMs   int
	IP           string
	UserAgent    string
	RequestID    string
	RequestBody  string
	ErrorMessage string
	CreatedAt    time.Time
}

// entry is one queued row: exactly one field is set.
type entry struct {
	login *loginRow
	op    *opRow
}

func (e entry) kind() string {
	if e.login != nil {
		return "login"
	}
	return "operation"
}

// sink persists batches; the default writes to PostgreSQL.
type sink interface {
	insertLogins(ctx context.Context, rows []loginRow) error
	insertOps(ctx context.Context, rows []opRow) error
}

type dbSink struct{ db *gorm.DB }

func (s dbSink) insertLogins(ctx context.Context, rows []loginRow) error {
	return s.db.WithContext(ctx).Table(tableLoginLogs).Create(&rows).Error
}

func (s dbSink) insertOps(ctx context.Context, rows []opRow) error {
	return s.db.WithContext(ctx).Table(tableOperationLogs).Create(&rows).Error
}

// Writer records login and operation logs asynchronously through a bounded
// queue and batch inserts, so request latency never depends on the log
// database. It implements auth.LoginObserver. Run must be started once.
type Writer struct {
	cfg    WriterConfig
	sink   sink
	logger *zap.Logger

	queue chan entry
	// mu guards stopped: producers hold the read lock while enqueueing so
	// that nothing is enqueued after the final drain.
	mu      sync.RWMutex
	stopped bool

	dropped      atomic.Uint64
	failed       atomic.Uint64
	reported     uint64 // drops already logged; only touched by Run
	droppedTotal *prometheus.CounterVec
	failedTotal  *prometheus.CounterVec
}

var _ auth.LoginObserver = (*Writer)(nil)

func newWriter(cfg WriterConfig, s sink, logger *zap.Logger) *Writer {
	cfg = cfg.withDefaults()
	return &Writer{
		cfg:    cfg,
		sink:   s,
		logger: logger,
		queue:  make(chan entry, cfg.QueueSize),
		droppedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "audit_log_dropped_total",
			Help: "Audit log rows dropped because the queue was full or the writer had stopped.",
		}, []string{"kind"}),
		failedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "audit_log_write_failures_total",
			Help: "Audit log rows that could not be inserted.",
		}, []string{"kind"}),
	}
}

// Collectors returns the writer's Prometheus metrics for registration.
func (w *Writer) Collectors() []prometheus.Collector {
	return []prometheus.Collector{w.droppedTotal, w.failedTotal}
}

// Dropped returns the number of rows dropped so far.
func (w *Writer) Dropped() uint64 { return w.dropped.Load() }

// RecordLogin implements auth.LoginObserver. It never blocks longer than
// the configured enqueue wait.
func (w *Writer) RecordLogin(_ context.Context, e auth.LoginEvent) {
	at := e.At
	if at.IsZero() {
		at = time.Now()
	}
	var uid *int64
	if e.UserID > 0 {
		id := e.UserID
		uid = &id
	}
	w.enqueue(entry{login: &loginRow{
		Kind:      clean(e.Kind, 16),
		Username:  clean(e.Username, 64),
		UserID:    uid,
		Success:   e.Success,
		Reason:    clean(e.Reason, 32),
		IP:        clean(e.IP, 64),
		UserAgent: clean(e.UserAgent, 512),
		RequestID: clean(e.RequestID, 64),
		CreatedAt: at,
	}})
}

// enqueue adds e to the queue, waiting at most EnqueueWait for space.
func (w *Writer) enqueue(e entry) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.stopped {
		w.drop(e)
		return false
	}
	select {
	case w.queue <- e:
		return true
	default:
	}
	timer := time.NewTimer(w.cfg.EnqueueWait)
	defer timer.Stop()
	select {
	case w.queue <- e:
		return true
	case <-timer.C:
		w.drop(e)
		return false
	}
}

func (w *Writer) drop(e entry) {
	w.dropped.Add(1)
	w.droppedTotal.WithLabelValues(e.kind()).Inc()
}

// batch accumulates rows between flushes.
type batch struct {
	logins []loginRow
	ops    []opRow
}

func (b *batch) add(e entry) {
	if e.login != nil {
		b.logins = append(b.logins, *e.login)
	} else if e.op != nil {
		b.ops = append(b.ops, *e.op)
	}
}

func (b *batch) size() int { return len(b.logins) + len(b.ops) }

// Run consumes the queue until ctx is cancelled, then stops accepting rows,
// drains the queue and flushes it before returning. Cancel ctx only after
// the HTTP server has shut down so in-flight requests are still recorded.
func (w *Writer) Run(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.FlushInterval)
	defer ticker.Stop()
	var b batch
	for {
		select {
		case e := <-w.queue:
			b.add(e)
			if b.size() >= w.cfg.BatchSize {
				w.flush(ctx, &b)
			}
		case <-ticker.C:
			w.flush(ctx, &b)
			w.reportDrops()
		case <-ctx.Done():
			w.shutdown(ctx, &b)
			return
		}
	}
}

func (w *Writer) shutdown(ctx context.Context, b *batch) {
	w.mu.Lock()
	w.stopped = true
	w.mu.Unlock()
	for {
		select {
		case e := <-w.queue:
			b.add(e)
			if b.size() >= w.cfg.BatchSize {
				w.flush(ctx, b)
			}
		default:
			w.flush(ctx, b)
			w.reportDrops()
			return
		}
	}
}

// flush inserts the pending rows. The parent context only provides values:
// a flush must still complete while the application shuts down.
func (w *Writer) flush(parent context.Context, b *batch) {
	if b.size() == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), w.cfg.FlushTimeout)
	defer cancel()
	if len(b.logins) > 0 {
		writeBatch(ctx, w, "login", b.logins, w.sink.insertLogins)
		b.logins = nil
	}
	if len(b.ops) > 0 {
		writeBatch(ctx, w, "operation", b.ops, w.sink.insertOps)
		b.ops = nil
	}
}

// writeBatch inserts rows in one statement and, if that fails, row by row
// so that one bad row cannot lose the whole batch.
func writeBatch[T any](ctx context.Context, w *Writer, kind string, rows []T,
	insert func(context.Context, []T) error) {
	err := insert(ctx, rows)
	if err == nil {
		return
	}
	if len(rows) == 1 {
		w.failRows(kind, 1, err)
		return
	}
	var failed uint64
	var last error
	for i := range rows {
		if err := insert(ctx, rows[i:i+1]); err != nil {
			failed++
			last = err
		}
	}
	if failed > 0 {
		w.failRows(kind, failed, last)
	}
}

func (w *Writer) failRows(kind string, n uint64, err error) {
	w.failed.Add(n)
	w.failedTotal.WithLabelValues(kind).Add(float64(n))
	w.logger.Error("audit log write failed", zap.String("kind", kind), zap.Uint64("rows", n), zap.Error(err))
}

// reportDrops logs drops since the last report, at most once per flush
// interval, so a flood of drops cannot flood the log.
func (w *Writer) reportDrops() {
	total := w.dropped.Load()
	if total == w.reported {
		return
	}
	w.logger.Warn("audit log rows dropped: queue full or writer stopped",
		zap.Uint64("dropped", total-w.reported), zap.Uint64("droppedTotal", total))
	w.reported = total
}
