// Package stream keeps an in-flight answer alive independently of the browser
// that asked for it.
//
// The failure this replaces was ordinary and expensive. A turn ran on the
// streaming request's own context: resize the window, switch tabs, lose Wi-Fi
// for two seconds, or let a reverse proxy time out an idle socket, and the
// context cancelled, the upstream stream aborted, and the half-written answer
// was discarded — after the tenant had been billed for every token of it. There
// was no way to reopen the drawer and find it, because nothing was written
// until the very last event arrived.
//
// So a turn is now a server-side object. Chunks are appended to a numbered log
// as they are produced; a reader attaches with the sequence number it last saw,
// is replayed everything after it, and then tails. Disconnecting is not an
// event the producer notices.
//
// The log lives in Redis in production, which is what makes a turn survive both
// a reconnect to a different replica and a rolling deploy of the one it started
// on. Development has no Redis, so the same interface is served from process
// memory: single replica, same behaviour, no configuration.
package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Retention is how long a finished turn stays replayable. An hour covers "I
// closed the laptop and came back"; beyond that the conversation history is the
// right place to look, and it is durable.
const Retention = time.Hour

// Chunk is one event in a turn.
type Chunk struct {
	Seq  int64  `json:"seq"`
	Kind string `json:"kind"` // delta | reasoning | status | tool | usage | done | error
	Text string `json:"text,omitempty"`
	// Data carries the structured payload for status, tool, usage and error
	// events. It is always a small object, never a row from the database.
	Data map[string]any `json:"data,omitempty"`
}

// Terminal reports whether this chunk ends the turn.
func (c Chunk) Terminal() bool { return c.Kind == "done" || c.Kind == "error" }

// Buffer is an append-and-replay log of one turn's events.
type Buffer interface {
	// Append adds a chunk and wakes every attached reader.
	Append(ctx context.Context, turnID string, c Chunk) error
	// Read returns chunks with Seq greater than after, waiting up to timeout
	// for at least one if none are available yet. An empty slice with no error
	// means "nothing yet, ask again" — that is the caller's cue to send a
	// heartbeat rather than to give up.
	Read(ctx context.Context, turnID string, after int64, timeout time.Duration) ([]Chunk, error)
	// Close marks the turn finished and starts its retention clock.
	Close(ctx context.Context, turnID string) error
}

// ---------------------------------------------------------------------------
// Memory
// ---------------------------------------------------------------------------

type memoryBuffer struct {
	mu     sync.Mutex
	turns  map[string]*memoryTurn
	maxAge time.Duration
	stopCh chan struct{}
}

type memoryTurn struct {
	chunks   []Chunk
	closed   bool
	touched  time.Time
	waiters  []chan struct{}
	sequence int64
}

// NewMemoryBuffer returns a single-process buffer. Correct for development and
// for a single-replica deployment; it cannot serve a reader that reconnects to
// a different replica, which is why production uses Redis.
func NewMemoryBuffer() Buffer {
	b := &memoryBuffer{
		turns:  make(map[string]*memoryTurn),
		maxAge: Retention,
		stopCh: make(chan struct{}),
	}
	go b.sweep()
	return b
}

func (b *memoryBuffer) sweep() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-b.maxAge)
			b.mu.Lock()
			for id, t := range b.turns {
				if t.touched.Before(cutoff) {
					delete(b.turns, id)
				}
			}
			b.mu.Unlock()
		}
	}
}

// Stop terminates the background sweep goroutine.
func (b *memoryBuffer) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	select {
	case <-b.stopCh:
	default:
		close(b.stopCh)
	}
}

func (b *memoryBuffer) Append(_ context.Context, turnID string, c Chunk) error {
	b.mu.Lock()
	t, ok := b.turns[turnID]
	if !ok {
		t = &memoryTurn{}
		b.turns[turnID] = t
	}
	t.sequence++
	c.Seq = t.sequence
	t.chunks = append(t.chunks, c)
	t.touched = time.Now()
	waiters := t.waiters
	t.waiters = nil
	b.mu.Unlock()

	for _, w := range waiters {
		close(w)
	}
	return nil
}

func (b *memoryBuffer) Read(ctx context.Context, turnID string, after int64, timeout time.Duration) ([]Chunk, error) {
	deadline := time.Now().Add(timeout)
	for {
		b.mu.Lock()
		t, ok := b.turns[turnID]
		if !ok {
			// A reader that attached before the first token was produced. It
			// must WAIT here, not be told "nothing" — returning immediately
			// turned the handler's retry loop into a spin that wrote keep-alive
			// frames as fast as the socket accepted them for the whole second or
			// two the model took to start. The answer then arrived into a client
			// already drowning in noise, which is what "no response appears"
			// actually was.
			//
			// Registering the turn here is safe: the producer's first Append
			// finds it and fills it in.
			t = &memoryTurn{touched: time.Now()}
			b.turns[turnID] = t
		}
		var out []Chunk
		for _, c := range t.chunks {
			if c.Seq > after {
				out = append(out, c)
			}
		}
		if len(out) > 0 || t.closed {
			b.mu.Unlock()
			return out, nil
		}
		wait := make(chan struct{})
		t.waiters = append(t.waiters, wait)
		b.mu.Unlock()

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, nil
		}
		timer := time.NewTimer(remaining)
		select {
		case <-wait:
			timer.Stop()
		case <-timer.C:
			return nil, nil
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		}
	}
}

func (b *memoryBuffer) Close(_ context.Context, turnID string) error {
	b.mu.Lock()
	t, ok := b.turns[turnID]
	if ok {
		t.closed = true
		t.touched = time.Now()
		waiters := t.waiters
		t.waiters = nil
		b.mu.Unlock()
		for _, w := range waiters {
			close(w)
		}
		return nil
	}
	b.mu.Unlock()
	return nil
}

// ---------------------------------------------------------------------------
// Redis
// ---------------------------------------------------------------------------

type redisBuffer struct {
	rdb *redis.Client
}

// NewRedisBuffer returns a buffer backed by Redis lists.
//
// A list and not a stream: the access pattern is "give me everything after
// index N", which LRANGE answers directly, and the sequence number is then just
// the index. Streams would add consumer-group machinery for a problem that does
// not have consumers to coordinate.
func NewRedisBuffer(rdb *redis.Client) Buffer {
	return &redisBuffer{rdb: rdb}
}

func chunkKey(turnID string) string { return "capsule:turn:" + turnID }

func (b *redisBuffer) Append(ctx context.Context, turnID string, c Chunk) error {
	key := chunkKey(turnID)
	// Seq is assigned by the list's own length after the push, so it is
	// monotonic without a second counter to keep in sync.
	payload, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("assistant stream: marshal chunk: %w", err)
	}
	pipe := b.rdb.TxPipeline()
	pipe.RPush(ctx, key, payload)
	pipe.Expire(ctx, key, Retention)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("assistant stream: append: %w", err)
	}
	return nil
}

func (b *redisBuffer) Read(ctx context.Context, turnID string, after int64, timeout time.Duration) ([]Chunk, error) {
	key := chunkKey(turnID)
	deadline := time.Now().Add(timeout)

	for {
		raw, err := b.rdb.LRange(ctx, key, after, -1).Result()
		if err != nil && err != redis.Nil {
			return nil, fmt.Errorf("assistant stream: read: %w", err)
		}
		if len(raw) > 0 {
			out := make([]Chunk, 0, len(raw))
			for i, item := range raw {
				var c Chunk
				if err := json.Unmarshal([]byte(item), &c); err != nil {
					continue
				}
				// Index in the list is the sequence: entry n has Seq n+1, so a
				// reader that saw Seq k asks for index k next time.
				c.Seq = after + int64(i) + 1
				out = append(out, c)
			}
			return out, nil
		}

		// Nothing new. Poll rather than subscribe: the producer and the reader
		// are usually the same process, the interval is short, and one PUBSUB
		// connection per open drawer is a worse trade than a 250ms LRANGE.
		if time.Now().After(deadline) {
			return nil, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (b *redisBuffer) Close(ctx context.Context, turnID string) error {
	// The terminal chunk already told readers the turn is over; all that is
	// left is to start the retention clock.
	if err := b.rdb.Expire(ctx, chunkKey(turnID), Retention).Err(); err != nil {
		return fmt.Errorf("assistant stream: close: %w", err)
	}
	return nil
}
