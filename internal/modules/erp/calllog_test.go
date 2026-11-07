package erp

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"
)

type callLogRepo struct {
	*fakeRepo
	mu      sync.Mutex
	calls   []CallRecord
	flushed chan struct{}
}

func (r *callLogRepo) InsertCallsAsSystem(_ context.Context, calls []CallRecord) error {
	r.mu.Lock()
	r.calls = append(r.calls, calls...)
	r.mu.Unlock()
	select {
	case r.flushed <- struct{}{}:
	default:
	}
	return nil
}
func (r *callLogRepo) count() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.calls) }
func waitCallFlush(t *testing.T, r *callLogRepo, want int) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for r.count() < want {
		select {
		case <-r.flushed:
		case <-deadline:
			t.Fatalf("flushed %d, want %d", r.count(), want)
		}
	}
}
func TestCallLogFlush(t *testing.T) {
	r := &callLogRepo{fakeRepo: newFakeRepo(), flushed: make(chan struct{}, 10)}
	logger := NewCallLogger(r, slog.New(slog.NewTextHandler(io.Discard, nil)))
	logger.interval = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go logger.Run(ctx)
	for i := 0; i < 200; i++ {
		logger.Record(CallRecord{RequestID: "test"})
	}
	waitCallFlush(t, r, 200)
	logger.Record(CallRecord{RequestID: "later"})
	waitCallFlush(t, r, 201)
	logger.Record(CallRecord{RequestID: "close"})
	closeCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	if err := logger.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
	if r.count() != 202 {
		t.Errorf("close flushed %d", r.count())
	}
}
func TestCallLogDropsAndContainsNoAuthorization(t *testing.T) {
	r := &callLogRepo{fakeRepo: newFakeRepo(), flushed: make(chan struct{}, 1)}
	logger := NewCallLogger(r, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for i := 0; i < 4097; i++ {
		logger.Record(CallRecord{})
	}
	if logger.Dropped() != 1 {
		t.Errorf("dropped=%d", logger.Dropped())
	}
	typ := reflect.TypeOf(CallRecord{})
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).Name == "Authorization" {
			t.Fatal("call record holds authorization")
		}
	}
}
