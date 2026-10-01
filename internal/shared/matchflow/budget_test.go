package matchflow_test

import (
	"sync"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func TestBudget_ReserveAndSettle_ExactCost(t *testing.T) {
	maxCost := money.FromMinor(100) // 1.00
	perAttempt := money.FromMinor(30)

	b := matchflow.NewBudget(10, 1000, 1000, maxCost)

	// Attempt 1: Reserve 30, Settle 25 (known)
	if !b.Reserve(perAttempt) {
		t.Fatal("expected reservation 1 to succeed")
	}
	b.Settle(perAttempt, money.FromMinor(25), true)

	_, _, _, spent, known := b.Stats()
	if spent.Minor() != 25 {
		t.Fatalf("expected spent 25, got %d", spent.Minor())
	}
	if !known {
		t.Fatal("expected cost to be known")
	}

	// Attempt 2: Reserve 30, Settle unknown
	if !b.Reserve(perAttempt) {
		t.Fatal("expected reservation 2 to succeed")
	}
	b.Settle(perAttempt, money.Zero, false)

	_, _, _, spent, known = b.Stats()
	// spent should be 25 + 30 (reserved amount) = 55
	if spent.Minor() != 55 {
		t.Fatalf("expected spent 55, got %d", spent.Minor())
	}
	if known {
		t.Fatal("expected cost to be marked unknown")
	}

	// Attempt 3: Reserve 30 (spent 55 + 30 = 85 <= 100)
	if !b.Reserve(perAttempt) {
		t.Fatal("expected reservation 3 to succeed")
	}

	// Attempt 4: Reserve 30 (55 spent + 30 reserved + 30 = 115 > 100) => should fail
	if b.Reserve(perAttempt) {
		t.Fatal("expected reservation 4 to fail due to cost limit")
	}

	// Settle attempt 3 with actual 15
	b.Settle(perAttempt, money.FromMinor(15), true)
	// spent is now 55 + 15 = 70. Available: 30 remaining.
	_, _, _, spent, _ = b.Stats()
	if spent.Minor() != 70 {
		t.Fatalf("expected spent 70, got %d", spent.Minor())
	}

	// Now Attempt 4 with perAttempt 30 should succeed (70 + 30 <= 100)
	if !b.Reserve(perAttempt) {
		t.Fatal("expected reservation to succeed after settling attempt 3")
	}
}

func TestBudget_RequestCeiling(t *testing.T) {
	b := matchflow.NewBudget(2, 0, 0, money.Zero)

	if !b.Reserve(money.Zero) {
		t.Fatal("expected first reservation to succeed")
	}
	if !b.Reserve(money.Zero) {
		t.Fatal("expected second reservation to succeed")
	}
	if b.Reserve(money.Zero) {
		t.Fatal("expected third reservation to fail (request ceiling reached)")
	}
}

func TestBudget_TokenCeilings(t *testing.T) {
	b := matchflow.NewBudget(10, 50, 50, money.Zero)

	b.RecordTokens(20, 10)
	if !b.Reserve(money.Zero) {
		t.Fatal("expected reserve to succeed")
	}

	b.RecordTokens(40, 0) // input tokens now 60 > 50
	if b.Reserve(money.Zero) {
		t.Fatal("expected reserve to fail when input tokens exceeded")
	}
}

func TestBudget_ConcurrentReservations(t *testing.T) {
	maxCost := money.FromMinor(1000)
	perAttempt := money.FromMinor(10)
	b := matchflow.NewBudget(200, 0, 0, maxCost)

	var wg sync.WaitGroup
	successes := make(chan bool, 200)

	for i := 0; i < 150; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if b.Reserve(perAttempt) {
				successes <- true
				b.Settle(perAttempt, perAttempt, true)
			} else {
				successes <- false
			}
		}()
	}
	wg.Wait()
	close(successes)

	successfulCount := 0
	for ok := range successes {
		if ok {
			successfulCount++
		}
	}

	// Exactly 100 reservations should succeed before 1000 minor units is exhausted (100 * 10 = 1000)
	if successfulCount != 100 {
		t.Fatalf("expected exactly 100 successful reservations, got %d", successfulCount)
	}

	_, _, _, spent, known := b.Stats()
	if spent.Minor() != 1000 {
		t.Fatalf("expected spent 1000, got %d", spent.Minor())
	}
	if !known {
		t.Fatal("expected cost to be known")
	}
}
