package commerce

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func TestAnnouncePlacedRunsEveryHookAndContainsPanics(t *testing.T) {
	s := &Service{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	var calls []int64
	s.OnOrderPlaced(func(_ context.Context, o *Order) { panic("notification bug") })
	s.OnOrderPlaced(func(ctx context.Context, o *Order) {
		if ctx.Err() != nil {
			t.Error("hook context must outlive the request")
		}
		calls = append(calls, o.ID)
	})
	s.OnOrderPlaced(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.announcePlaced(ctx, &Order{ID: 9})
	s.announcePlaced(ctx, nil)

	if len(calls) != 1 || calls[0] != 9 {
		t.Fatalf("hooks after a panicking one must still run once per order, got %v", calls)
	}
}
