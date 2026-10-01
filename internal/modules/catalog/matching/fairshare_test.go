package matching_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
)

// TestFairScheduler_SmallOrgNotStarved asserts that a small organization's request
// does not wait behind all requests of a large organization, but is served within 1 slot round-robin.
func TestFairScheduler_SmallOrgNotStarved(t *testing.T) {
	// Create a scheduler with 1 slot so every request must serialize
	sched := matching.NewFairScheduler(1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const orgBig int64 = 1001
	const orgSmall int64 = 1002

	// 1. Occupy the single available slot with an initial task from orgBig
	rel1, err := sched.AcquireSlot(ctx, orgBig)
	if err != nil {
		t.Fatalf("failed to acquire initial slot: %v", err)
	}

	// 2. OrgBig enqueues 10 requests while the slot is occupied
	var order []int64
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rel, err := sched.AcquireSlot(ctx, orgBig)
			if err != nil {
				return
			}
			mu.Lock()
			order = append(order, orgBig)
			mu.Unlock()
			time.Sleep(5 * time.Millisecond)
			rel()
		}(i)
	}

	// Give goroutines time to enqueue
	time.Sleep(50 * time.Millisecond)

	// 3. OrgSmall enqueues 1 request
	wg.Add(1)
	go func() {
		defer wg.Done()
		rel, err := sched.AcquireSlot(ctx, orgSmall)
		if err != nil {
			return
		}
		mu.Lock()
		order = append(order, orgSmall)
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		rel()
	}()

	// Give small org goroutine time to enqueue
	time.Sleep(50 * time.Millisecond)

	// 4. Release the initial slot
	rel1()

	// Wait for all queued requests to finish
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	if len(order) != 11 {
		t.Fatalf("expected 11 tasks completed, got %d", len(order))
	}

	// Because of round-robin:
	// After orgBig finishes one task, orgSmall must be chosen next (within 1 slot)!
	// So orgSmall should be either index 0 or index 1 in the executed order.
	smallIdx := -1
	for idx, org := range order {
		if org == orgSmall {
			smallIdx = idx
			break
		}
	}

	if smallIdx == -1 {
		t.Fatal("orgSmall was never executed")
	}
	if smallIdx > 1 {
		t.Fatalf("orgSmall starved! executed at index %d instead of <= 1. Order: %v", smallIdx, order)
	}
}
