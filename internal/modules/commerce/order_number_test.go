package commerce

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

// allocatorRepo is a Repository that also allocates order numbers. The embedded
// interface is nil: nextOrderNumber only calls NextOrderNumber on this value.
type allocatorRepo struct {
	Repository
	next  string
	err   error
	calls int
}

func (r *allocatorRepo) NextOrderNumber(context.Context) (string, error) {
	r.calls++
	return r.next, r.err
}

// plainRepo implements Repository without the allocator method.
type plainRepo struct{ Repository }

func TestNextOrderNumberUsesAllocatorWhenImplemented(t *testing.T) {
	repo := &allocatorRepo{next: "45012"}
	svc := NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	got, err := svc.nextOrderNumber(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("nextOrderNumber: %v", err)
	}
	if got != "45012" || repo.calls != 1 {
		t.Fatalf("got %q with %d allocator calls, want 45012 and 1 call", got, repo.calls)
	}
}

func TestNextOrderNumberFallsBackToCounterWithoutAllocator(t *testing.T) {
	svc := NewService(plainRepo{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	first, err := svc.nextOrderNumber(context.Background(), now)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := svc.nextOrderNumber(context.Background(), now)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first == "" || first == second {
		t.Fatalf("fallback numbers must differ, got %q and %q", first, second)
	}
}

func TestNextOrderNumberAllocatorErrorFailsTheCheckout(t *testing.T) {
	want := errors.New("sequence unavailable")
	svc := NewService(&allocatorRepo{err: want}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := svc.nextOrderNumber(context.Background(), time.Now()); !errors.Is(err, want) {
		t.Fatalf("err = %v, want the allocator error", err)
	}
}
