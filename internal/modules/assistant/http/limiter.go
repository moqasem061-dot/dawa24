package http

import (
	"sync"
	"time"
)

// RateLimiter bounds how many questions one user may ask per minute.
//
// It is a sliding window over timestamps, and it evicts. The previous version
// kept a map[int64][]time.Time and never removed a key, so every user who had
// ever used the assistant stayed resident for the life of the process — a slow
// leak that only shows up on a long-running deployment with a lot of accounts.
//
// It is also process-local, which means N replicas allow N times the limit.
// That is the correct trade here: this limit exists to stop one person holding
// the drawer's send key down, not to enforce a quota — the Gateway's plan
// window does that, centrally and for real money.
type RateLimiter struct {
	mu      sync.Mutex
	limits  map[int64][]time.Time
	window  time.Duration
	maxReqs int
	// lastSweep bounds how often eviction runs, so a busy process is not
	// walking the whole map on every request.
	lastSweep time.Time
}

// NewRateLimiter creates a sliding window rate limiter.
func NewRateLimiter(maxReqs int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limits:    make(map[int64][]time.Time),
		window:    window,
		maxReqs:   maxReqs,
		lastSweep: time.Now(),
	}
}

// Allow reports whether a request by userID is within the limit.
func (rl *RateLimiter) Allow(userID int64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)
	rl.sweepLocked(cutoff)

	valid := rl.limits[userID][:0]
	for _, t := range rl.limits[userID] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.maxReqs {
		rl.limits[userID] = valid
		return false
	}

	rl.limits[userID] = append(valid, now)
	return true
}

// sweepLocked drops users whose entire window has expired.
func (rl *RateLimiter) sweepLocked(cutoff time.Time) {
	if time.Since(rl.lastSweep) < rl.window {
		return
	}
	rl.lastSweep = time.Now()
	for id, times := range rl.limits {
		if len(times) == 0 || times[len(times)-1].Before(cutoff) {
			delete(rl.limits, id)
		}
	}
}
