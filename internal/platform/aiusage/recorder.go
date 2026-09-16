package aiusage

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// Recording usage without making anyone wait for it.
//
// A ledger write is bookkeeping. It happens after the model has answered and
// the tenant has already been billed, so nothing about the outcome of the
// request depends on it — and a synchronous insert on the completion path would
// add a database round trip to every AI call, including the ones inside an
// import loop that makes hundreds.
//
// So writes go through a buffered channel and a small pool of writers. The
// buffer is bounded and a full buffer drops the entry rather than blocking:
// losing a row from a usage report is a far smaller harm than stalling an
// import because the database is briefly slow, and the drop is counted and
// logged so the loss is never silent.

// bufferSize is how many events may be waiting to be written.
//
// Sized for the burst an import produces — a run adjudicating a large file can
// emit a few dozen calls in a second — with room to absorb a slow write without
// reaching the drop path.
const bufferSize = 1024

// writers is how many inserts run concurrently. More than one so a single slow
// write does not stall the queue; few enough that the ledger cannot monopolise
// the connection pool that serves actual requests.
const writers = 2

// flushTimeout bounds one insert. An event that cannot be written in this long
// is dropped rather than held, because holding it delays every event behind it.
const flushTimeout = 10 * time.Second

// Recorder writes gateway usage events to the ledger, asynchronously.
type Recorder struct {
	repo Repository
	log  *slog.Logger

	events chan Entry

	stopOnce sync.Once
	done     chan struct{}
	wg       sync.WaitGroup

	dropped atomic64
}

// NewRecorder starts the writer pool. Call Close on shutdown to flush.
func NewRecorder(repo Repository, log *slog.Logger) *Recorder {
	r := &Recorder{
		repo:   repo,
		log:    log.With("component", "ai_usage_recorder"),
		events: make(chan Entry, bufferSize),
		done:   make(chan struct{}),
	}
	r.wg.Add(writers)
	for i := 0; i < writers; i++ {
		go r.drain()
	}
	return r
}

// RecordAIUsage queues one event. It never blocks and never returns an error.
func (r *Recorder) RecordAIUsage(_ context.Context, event gateway.UsageEvent) {
	if r == nil || event.OrganizationID <= 0 {
		return
	}
	entry := Entry{
		OrganizationID: event.OrganizationID,
		UserID:         event.UserID,
		Capability:     event.Capability,
		Feature:        event.Feature,
		Model:          event.Model,
		RequestID:      event.RequestID,
		InputTokens:    event.InputTokens,
		OutputTokens:   event.OutputTokens,
		CostNanoUSD:    event.CostNanoUSD,
		CostKnown:      event.CostKnown,
		DurationMS:     int(event.Duration.Milliseconds()),
		Status:         event.Status,
		FinishReason:   event.FinishReason,
		ErrorMessage:   event.ErrorMessage,
		FromCache:      event.FromCache,
		Fallback:       event.Fallback,
		CreatedAt:      event.At,
	}

	select {
	case r.events <- entry:
	default:
		// The queue is full. Counting the loss matters more than the row: a
		// non-zero counter here means the ledger is understating a tenant's
		// consumption, which an operator needs to know before somebody
		// reconciles it against the Gateway's own totals and finds a gap.
		if n := r.dropped.add(1); n == 1 || n%100 == 0 {
			r.log.Warn("ai usage ledger buffer full; event dropped",
				"dropped_total", n, "org_id", event.OrganizationID,
				"capability", event.Capability)
		}
	}
}

// Close stops accepting events and waits for the queue to drain.
//
// Bounded by the caller's context: a shutdown must not hang on a database that
// has already gone away.
func (r *Recorder) Close(ctx context.Context) {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() { close(r.events) })

	drained := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-ctx.Done():
		r.log.Warn("ai usage ledger did not drain before shutdown",
			"dropped_total", r.dropped.load())
	}
	close(r.done)
}

// Dropped reports how many events were discarded because the buffer was full.
// Exposed so the status endpoint can surface a ledger falling behind.
func (r *Recorder) Dropped() int64 {
	if r == nil {
		return 0
	}
	return r.dropped.load()
}

func (r *Recorder) drain() {
	defer r.wg.Done()
	for entry := range r.events {
		// A fresh context, not the request's: the request that produced this
		// event has very often already returned, and writing under its
		// cancelled context would discard exactly the events for the calls that
		// took longest.
		ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
		if err := r.repo.Insert(ctx, entry); err != nil {
			r.log.WarnContext(ctx, "could not record ai usage",
				"org_id", entry.OrganizationID, "capability", entry.Capability,
				"error", err)
		}
		cancel()
	}
}

var _ gateway.UsageRecorder = (*Recorder)(nil)
