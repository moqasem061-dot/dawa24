package stream_test

import (
	"context"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/stream"
)

// The buffer is what turns "the answer stopped when I resized the window" into
// a non-event. These tests describe the three things a reader needs: replay
// from a known point, no gap on reconnect, and a terminal frame that ends the
// stream instead of hanging it.

func TestReplayFromASequenceNumber(t *testing.T) {
	b := stream.NewMemoryBuffer()
	ctx := context.Background()

	for _, text := range []string{"إجمالي", " المشتريات", " 12,400 جنيه"} {
		if err := b.Append(ctx, "turn-1", stream.Chunk{Kind: "delta", Text: text}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	// A fresh reader sees everything.
	all, err := b.Read(ctx, "turn-1", 0, time.Second)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d chunks, want 3", len(all))
	}
	for i, c := range all {
		if c.Seq != int64(i+1) {
			t.Fatalf("chunk %d has seq %d", i, c.Seq)
		}
	}

	// A reconnecting reader that already saw the first two gets only the third
	// — no gap, and no duplicate.
	rest, err := b.Read(ctx, "turn-1", 2, time.Second)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if len(rest) != 1 || rest[0].Text != " 12,400 جنيه" {
		t.Fatalf("resume returned %+v", rest)
	}
}

// A reader that attaches before the producer has written anything must block
// until there is something, not spin or return early with nothing.
func TestReadWaitsForTheFirstChunk(t *testing.T) {
	b := stream.NewMemoryBuffer()
	ctx := context.Background()

	// Seed the turn so the buffer knows it exists.
	if err := b.Append(ctx, "turn-2", stream.Chunk{Kind: "status"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	go func() {
		time.Sleep(80 * time.Millisecond)
		_ = b.Append(ctx, "turn-2", stream.Chunk{Kind: "delta", Text: "متأخر"})
	}()

	started := time.Now()
	got, err := b.Read(ctx, "turn-2", 1, 2*time.Second)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 1 || got[0].Text != "متأخر" {
		t.Fatalf("got %+v", got)
	}
	if elapsed := time.Since(started); elapsed < 50*time.Millisecond {
		t.Fatalf("returned after %v; it did not wait", elapsed)
	}
}

// An idle turn must time out and return nothing, so the handler can send a
// heartbeat rather than holding the response open in silence.
func TestReadTimesOutQuietly(t *testing.T) {
	b := stream.NewMemoryBuffer()
	ctx := context.Background()
	_ = b.Append(ctx, "turn-3", stream.Chunk{Kind: "delta", Text: "x"})

	got, err := b.Read(ctx, "turn-3", 1, 120*time.Millisecond)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no chunks, got %+v", got)
	}
}

// After Close, a reader must not block waiting for more.
func TestClosedTurnDoesNotBlock(t *testing.T) {
	b := stream.NewMemoryBuffer()
	ctx := context.Background()
	_ = b.Append(ctx, "turn-4", stream.Chunk{Kind: "done"})
	_ = b.Close(ctx, "turn-4")

	started := time.Now()
	got, _ := b.Read(ctx, "turn-4", 1, 3*time.Second)
	if len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("a closed turn blocked for %v", elapsed)
	}
}

// A turn nobody started is not an error — the reader simply has nothing to
// show, which is what a stale link produces.
func TestUnknownTurnIsEmpty(t *testing.T) {
	b := stream.NewMemoryBuffer()
	got, err := b.Read(context.Background(), "never-existed", 0, time.Second)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestTerminalChunks(t *testing.T) {
	if !(stream.Chunk{Kind: "done"}).Terminal() {
		t.Error("done is not terminal")
	}
	if !(stream.Chunk{Kind: "error"}).Terminal() {
		t.Error("error is not terminal")
	}
	if (stream.Chunk{Kind: "delta"}).Terminal() {
		t.Error("delta is terminal")
	}
}

// Two readers of the same turn must both see everything: one browser tab does
// not consume another's stream.
func TestConcurrentReadersBothSeeEverything(t *testing.T) {
	b := stream.NewMemoryBuffer()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = b.Append(ctx, "turn-5", stream.Chunk{Kind: "delta", Text: "x"})
	}

	first, _ := b.Read(ctx, "turn-5", 0, time.Second)
	second, _ := b.Read(ctx, "turn-5", 0, time.Second)

	if len(first) != 5 || len(second) != 5 {
		t.Fatalf("readers saw %d and %d chunks, want 5 each", len(first), len(second))
	}
}
