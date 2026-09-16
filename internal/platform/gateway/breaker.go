package gateway

import (
	"context"
	"sync"
	"time"
)

// Failing fast, and retrying politely.
//
// Without a circuit breaker a Gateway outage turns every import row into a
// full-length timeout, and the import queue backs up until every worker is
// saturated waiting on a service that is not answering. Opening the circuit
// converts that into an immediate fallback, which is the outcome every caller
// here is already written to handle.

// breaker is a three-state circuit breaker: closed, open, half-open.
//
// Without it, a Gateway outage turns every import row into a 60-second timeout,
// and the import queue backs up until workers are saturated. Opening the circuit
// converts that into an immediate fallback.
type breaker struct {
	mu           sync.Mutex
	failures     int
	threshold    int
	openUntil    time.Time
	openDuration time.Duration
	window       time.Duration
	firstFailure time.Time
}

func newBreaker(threshold int, window, openDuration time.Duration) *breaker {
	return &breaker{threshold: threshold, window: window, openDuration: openDuration}
}

func (b *breaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if time.Now().Before(b.openUntil) {
		return false
	}
	return true
}

func (b *breaker) success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.firstFailure = time.Time{}
}

func (b *breaker) failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if b.firstFailure.IsZero() || now.Sub(b.firstFailure) > b.window {
		b.firstFailure = now
		b.failures = 0
	}
	b.failures++
	if b.failures >= b.threshold {
		b.openUntil = now.Add(b.openDuration)
		b.failures = 0
		b.firstFailure = time.Time{}
	}
}

func backoff(attempt int) time.Duration {
	base := time.Duration(1<<uint(attempt-1)) * 250 * time.Millisecond
	if base > 4*time.Second {
		base = 4 * time.Second
	}
	// Full jitter: spreads a thundering herd of import workers retrying together.
	return time.Duration(float64(base) * (0.5 + 0.5*pseudoRand()))
}

var (
	randMu    sync.Mutex
	randState uint64 = 0x2545F4914F6CDD1D
)

// pseudoRand is a tiny xorshift generator. Jitter does not need crypto entropy,
// and this avoids pulling math/rand's global lock into the hot retry path.
func pseudoRand() float64 {
	randMu.Lock()
	defer randMu.Unlock()
	randState ^= randState << 13
	randState ^= randState >> 7
	randState ^= randState << 17
	return float64(randState>>11) / float64(1<<53)
}

type traceKey struct{}

// WithTraceparent stores a W3C traceparent for propagation to the Gateway, so a
// slow AI call can be followed across both services in one trace.
func WithTraceparent(ctx context.Context, tp string) context.Context {
	return context.WithValue(ctx, traceKey{}, tp)
}

func traceparentFrom(ctx context.Context) string {
	tp, _ := ctx.Value(traceKey{}).(string)
	return tp
}
