package erp

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

type CallLogger struct {
	repo     Repository
	log      *slog.Logger
	records  chan CallRecord
	stop     chan struct{}
	done     chan struct{}
	interval time.Duration
	mu       sync.Mutex
	closed   bool
	dropped  atomic.Int64
}

func NewCallLogger(repo Repository, log *slog.Logger) *CallLogger {
	return &CallLogger{repo: repo, log: log, records: make(chan CallRecord, 4096), stop: make(chan struct{}), done: make(chan struct{}), interval: time.Second}
}
func (l *CallLogger) Record(record CallRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		l.dropped.Add(1)
		return
	}
	select {
	case l.records <- record:
	default:
		l.dropped.Add(1)
	}
}
func (l *CallLogger) Dropped() int64 { return l.dropped.Load() }
func (l *CallLogger) Run(ctx context.Context) {
	defer close(l.done)
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()
	batch := make([]CallRecord, 0, 200)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := l.repo.InsertCallsAsSystem(ctx, batch); err != nil && l.log != nil {
			l.log.ErrorContext(ctx, "ERP call log flush failed", "error", err)
		}
		batch = make([]CallRecord, 0, 200)
	}
	for {
		select {
		case record := <-l.records:
			batch = append(batch, record)
			if len(batch) == 200 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-l.stop:
			for {
				select {
				case record := <-l.records:
					batch = append(batch, record)
					if len(batch) == 200 {
						flush()
					}
				default:
					flush()
					return
				}
			}
		case <-ctx.Done():
			flush()
			return
		}
	}
}
func (l *CallLogger) Close(ctx context.Context) error {
	l.mu.Lock()
	if !l.closed {
		l.closed = true
		close(l.stop)
	}
	l.mu.Unlock()
	select {
	case <-l.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
