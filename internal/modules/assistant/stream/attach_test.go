package stream_test

import (
	"context"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/stream"
)

// Attaching before the first token must BLOCK, not return "nothing".
//
// This is the regression for the bug behind "no response appears". The handler
// loops on Read; when Read returned immediately for a turn the producer had not
// touched yet, that loop became a spin writing keep-alive frames as fast as the
// socket accepted them for the whole second or two the model took to start.
func TestReadWaitsForATurnThatHasNotStarted(t *testing.T) {
	b := stream.NewMemoryBuffer()
	ctx := context.Background()

	started := time.Now()
	done := make(chan []stream.Chunk, 1)
	go func() {
		got, _ := b.Read(ctx, "not-started-yet", 0, 2*time.Second)
		done <- got
	}()

	// The producer's first chunk arrives well after the reader attached.
	time.Sleep(120 * time.Millisecond)
	if err := b.Append(ctx, "not-started-yet",
		stream.Chunk{Kind: "delta", Text: "أول كلمة"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	select {
	case got := <-done:
		if len(got) != 1 || got[0].Text != "أول كلمة" {
			t.Fatalf("got %+v, want the first delta", got)
		}
		if elapsed := time.Since(started); elapsed < 100*time.Millisecond {
			t.Fatalf("returned after %v — it did not wait, which is the spin", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reader never woke")
	}
}

// The producer's Append must find the entry a waiting reader created, and keep
// numbering from one — a reader that pre-registered the turn must not shift the
// sequence.
func TestSequenceStartsAtOneAfterAPreAttach(t *testing.T) {
	b := stream.NewMemoryBuffer()
	ctx := context.Background()

	go func() { _, _ = b.Read(ctx, "pre", 0, 500*time.Millisecond) }()
	time.Sleep(50 * time.Millisecond)

	_ = b.Append(ctx, "pre", stream.Chunk{Kind: "delta", Text: "a"})
	_ = b.Append(ctx, "pre", stream.Chunk{Kind: "delta", Text: "b"})

	got, err := b.Read(ctx, "pre", 0, time.Second)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d chunks, want 2", len(got))
	}
	if got[0].Seq != 1 || got[1].Seq != 2 {
		t.Fatalf("sequence = %d,%d — want 1,2", got[0].Seq, got[1].Seq)
	}
}
