package ui

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The properties tested here are the ones that stop an administrator signing in
// from taking the platform down. Each is a separate failure mode that was
// live in production, so each gets its own test.

func withTimings(t *testing.T, ttl, budget, cold time.Duration) {
	t.Helper()
	oldTTL, oldBudget, oldCold := dashboardTTL, dashboardComputeBudget, dashboardColdWait
	dashboardTTL, dashboardComputeBudget, dashboardColdWait = ttl, budget, cold
	t.Cleanup(func() {
		dashboardTTL, dashboardComputeBudget, dashboardColdWait = oldTTL, oldBudget, oldCold
	})
}

// Fifty simultaneous dashboard loads must produce ONE computation, not fifty.
// Fifty is what a stuck page plus an impatient administrator actually generates,
// and fifty × fifteen transactions against a pool of twenty is the stall.
func TestDashboardRefreshIsSingleFlight(t *testing.T) {
	withTimings(t, time.Minute, 5*time.Second, 2*time.Second)

	var computes int64
	release := make(chan struct{})
	compute := func(context.Context) *dashboardSnapshot {
		atomic.AddInt64(&computes, 1)
		<-release
		return &dashboardSnapshot{totalUsers: 7}
	}

	c := &dashboardCache{}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.get(context.Background(), compute)
		}()
	}

	// Let them all pile onto the same in-flight refresh before it finishes.
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt64(&computes); got != 1 {
		t.Fatalf("ran %d computations for 50 concurrent loads, want exactly 1", got)
	}
}

// A stale snapshot is served immediately. The request must not wait on the
// refresh it just started — waiting is what the proxy times out on.
func TestStaleSnapshotIsServedWithoutWaiting(t *testing.T) {
	withTimings(t, 10*time.Millisecond, 5*time.Second, 2*time.Second)

	block := make(chan struct{})
	t.Cleanup(func() { close(block) })

	c := &dashboardCache{snap: &dashboardSnapshot{
		totalUsers: 42,
		computedAt: time.Now().Add(-time.Hour), // long stale
	}}

	start := time.Now()
	got := c.get(context.Background(), func(context.Context) *dashboardSnapshot {
		<-block // a refresh that never finishes
		return nil
	})
	elapsed := time.Since(start)

	if got == nil || got.totalUsers != 42 {
		t.Fatalf("did not serve the stale snapshot: %#v", got)
	}
	if elapsed > time.Second {
		t.Fatalf("waited %v on a background refresh; stale must be served immediately", elapsed)
	}
}

// With nothing cached at all — the first load after a restart — the request
// waits only for its budget and then renders. It must never hang until the
// proxy gives up.
func TestColdLoadIsBounded(t *testing.T) {
	withTimings(t, time.Minute, 10*time.Second, 150*time.Millisecond)

	block := make(chan struct{})
	t.Cleanup(func() { close(block) })

	c := &dashboardCache{}
	start := time.Now()
	c.get(context.Background(), func(context.Context) *dashboardSnapshot {
		<-block
		return nil
	})
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("cold load blocked for %v; the cold wait budget is %v", elapsed, dashboardColdWait)
	}
}

// Once a refresh lands it is served, and no further computation runs inside the
// TTL however many times the page is loaded.
func TestFreshSnapshotIsReusedWithinTTL(t *testing.T) {
	withTimings(t, time.Minute, 5*time.Second, 2*time.Second)

	var computes int64
	compute := func(context.Context) *dashboardSnapshot {
		atomic.AddInt64(&computes, 1)
		return &dashboardSnapshot{totalUsers: 3}
	}

	c := &dashboardCache{}
	if got := c.get(context.Background(), compute); got == nil || got.totalUsers != 3 {
		t.Fatalf("cold load did not return the computed snapshot: %#v", got)
	}
	for i := 0; i < 20; i++ {
		if got := c.get(context.Background(), compute); got.totalUsers != 3 {
			t.Fatalf("load %d returned %#v", i, got)
		}
	}
	if got := atomic.LoadInt64(&computes); got != 1 {
		t.Fatalf("ran %d computations across 21 loads inside the TTL, want 1", got)
	}
}

// The refresh must outlive the request that started it. An administrator who
// navigates away mid-load previously cancelled the work every later request was
// about to be served from, so the next load started from nothing again.
func TestRefreshSurvivesRequestCancellation(t *testing.T) {
	withTimings(t, time.Minute, 5*time.Second, 50*time.Millisecond)

	started := make(chan struct{})
	finished := make(chan struct{})
	compute := func(ctx context.Context) *dashboardSnapshot {
		close(started)
		select {
		case <-ctx.Done():
			return nil // cancelled: the bug
		case <-time.After(200 * time.Millisecond):
		}
		close(finished)
		return &dashboardSnapshot{totalUsers: 11}
	}

	reqCtx, cancel := context.WithCancel(context.Background())
	c := &dashboardCache{}
	go c.get(reqCtx, compute)

	<-started
	cancel() // the browser goes away

	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("refresh was cancelled with its request; it must run to completion")
	}

	if got := c.get(context.Background(), compute); got == nil || got.totalUsers != 11 {
		t.Fatalf("completed refresh was not cached: %#v", got)
	}
}

// runParallel must actually overlap. Sequential groups are what made the page
// cost the sum of every query rather than the slowest one.
func TestRunParallelOverlaps(t *testing.T) {
	const n = 6
	var running, peak int64
	var mu sync.Mutex

	groups := make([]func(), 0, n)
	for i := 0; i < n; i++ {
		groups = append(groups, func() {
			cur := atomic.AddInt64(&running, 1)
			mu.Lock()
			if cur > peak {
				peak = cur
			}
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			atomic.AddInt64(&running, -1)
		})
	}

	start := time.Now()
	runParallel(groups...)
	elapsed := time.Since(start)

	if peak < 2 {
		t.Fatalf("peak concurrency was %d; the groups ran sequentially", peak)
	}
	if elapsed > time.Duration(n)*50*time.Millisecond {
		t.Fatalf("took %v, which is the sequential cost", elapsed)
	}
}

// A refresh that FAILS must not be retried by the next request, and the one
// after that, and the one after that.
//
// This is the case where the database is already struggling — precisely when a
// retry storm turns a slow dashboard into an outage. The backoff is keyed on
// when a refresh was last attempted, not on how old the last good snapshot is.
func TestFailedRefreshDoesNotRetryOnEveryRequest(t *testing.T) {
	withTimings(t, time.Minute, 5*time.Second, 50*time.Millisecond)

	var computes int64
	compute := func(context.Context) *dashboardSnapshot {
		atomic.AddInt64(&computes, 1)
		return nil // the budget blew; nothing to cache
	}

	c := &dashboardCache{}
	for i := 0; i < 25; i++ {
		if got := c.get(context.Background(), compute); got != nil {
			t.Fatalf("load %d returned a snapshot from a failed refresh: %#v", i, got)
		}
	}

	if got := atomic.LoadInt64(&computes); got != 1 {
		t.Fatalf("ran %d computations across 25 loads after a failure, want 1", got)
	}
}

// A failed refresh must also not wipe out a snapshot that is merely stale. Old
// numbers are still numbers; zeros are a broken page.
func TestFailedRefreshKeepsTheLastGoodSnapshot(t *testing.T) {
	withTimings(t, 10*time.Millisecond, 5*time.Second, 50*time.Millisecond)

	done := make(chan struct{})
	c := &dashboardCache{snap: &dashboardSnapshot{
		totalUsers: 99,
		computedAt: time.Now().Add(-time.Hour),
	}}

	got := c.get(context.Background(), func(context.Context) *dashboardSnapshot {
		defer close(done)
		return nil
	})
	if got == nil || got.totalUsers != 99 {
		t.Fatalf("stale snapshot was not served: %#v", got)
	}

	<-done
	time.Sleep(50 * time.Millisecond) // let the refresh goroutine finish storing

	c.mu.Lock()
	snap := c.snap
	c.mu.Unlock()
	if snap == nil || snap.totalUsers != 99 {
		t.Fatalf("a failed refresh replaced the last good snapshot: %#v", snap)
	}
}
