package stream

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Choosing the backing store at the right moment.
//
// Routes are mounted while the Redis connection is still being established in
// the background, so asking "is Redis available?" at boot always answered no —
// and production silently ran on the in-process buffer for the life of the
// process. That is not a graceful degradation: it is the wrong answer, decided
// a second too early and never revisited.
//
// This resolves on first use instead, and remembers the answer.
type lazyBuffer struct {
	redis  func() *redis.Client
	memory Buffer

	once     sync.Once
	resolved Buffer
}

// NewLazyBuffer returns a Buffer that picks Redis the first time it is actually
// used, and falls back to process memory if Redis is still not there.
func NewLazyBuffer(redisFn func() *redis.Client) Buffer {
	return &lazyBuffer{redis: redisFn, memory: NewMemoryBuffer()}
}

func (l *lazyBuffer) pick() Buffer {
	l.once.Do(func() {
		if l.redis != nil {
			if rdb := l.redis(); rdb != nil {
				l.resolved = NewRedisBuffer(rdb)
				return
			}
		}
		l.resolved = l.memory
	})
	return l.resolved
}

// Backend names the store in use, for the log line at first turn.
func (l *lazyBuffer) Backend() string {
	if _, ok := l.pick().(*redisBuffer); ok {
		return "redis"
	}
	return "memory"
}

func (l *lazyBuffer) Append(ctx context.Context, turnID string, c Chunk) error {
	return l.pick().Append(ctx, turnID, c)
}

func (l *lazyBuffer) Read(ctx context.Context, turnID string, after int64, timeout time.Duration) ([]Chunk, error) {
	return l.pick().Read(ctx, turnID, after, timeout)
}

func (l *lazyBuffer) Close(ctx context.Context, turnID string) error {
	return l.pick().Close(ctx, turnID)
}
