package matchflow_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// TestEngine_ModesEndByDeadline asserts that every simulator mode terminates
// within the configured deadline + 2 seconds (TerminalPublishSlack).
func TestEngine_ModesEndByDeadline(t *testing.T) {
	modes := []matchflow.SimulatorMode{
		matchflow.SimulatorModeFast,
		matchflow.SimulatorModeSlow,
		matchflow.SimulatorModeDown,
		matchflow.SimulatorModeBudget,
		matchflow.SimulatorModeMalformed,
	}

	testDeadline := 600 * time.Millisecond
	maxAllowed := testDeadline + matchflow.TerminalPublishSlack + 500*time.Millisecond

	for _, mode := range modes {
		t.Run(string(mode), func(t *testing.T) {
			sim := matchflow.NewSimulator(mode)
			sim.SlowDelay = 5 * time.Second // longer than testDeadline

			var questions []matchflow.Question
			for i := 1; i <= 5; i++ {
				questions = append(questions, matchflow.Question{
					Key: fmt.Sprintf("q%d", i),
					Item: matchflow.Item{
						Ref:     i,
						Text:    fmt.Sprintf("item %d", i),
						Options: []int64{int64(i)},
					},
					Window: []matchflow.CatalogEntry{
						{ProductID: int64(i), NameAR: fmt.Sprintf("منتج %d", i)},
					},
				})
			}

			start := time.Now()
			stats := matchflow.Run(context.Background(), sim, matchflow.Spec{
				Profile:     matchflow.ProfileVendor,
				Deadline:    testDeadline,
				EnqueueTime: start,
				Questions:   questions,
			})
			elapsed := time.Since(start)

			if elapsed > maxAllowed {
				t.Fatalf("mode %s took %v, exceeded deadline+2s (%v)", mode, elapsed, maxAllowed)
			}
			_ = stats
		})
	}
}

// TestEngine_AttemptNeverExceeds15s asserts that individual calls never receive a deadline > 15s.
func TestEngine_AttemptNeverExceeds15s(t *testing.T) {
	spy := &attemptSpy{
		maxSeenTimeout: 0,
		enhanceFunc: func(ctx context.Context, b matchflow.Batch) ([]matchflow.Decision, error) {
			dl, ok := ctx.Deadline()
			if ok {
				rem := time.Until(dl)
				if rem > 15*time.Second+50*time.Millisecond {
					return nil, fmt.Errorf("attempt timeout %v exceeded 15s", rem)
				}
			}
			var out []matchflow.Decision
			for _, it := range b.Items {
				opt := it.Options[0]
				out = append(out, matchflow.Decision{Ref: it.Ref, ProductID: &opt, Confidence: 0.95})
			}
			return out, nil
		},
	}

	questions := []matchflow.Question{
		{
			Key: "q1",
			Item: matchflow.Item{
				Ref:     1,
				Text:    "panadol",
				Options: []int64{101},
			},
			Window: []matchflow.CatalogEntry{
				{ProductID: 101, NameAR: "بانادول"},
			},
		},
	}

	// Run with a long 90s deadline; attempt must still be <= 15s
	stats := matchflow.Run(context.Background(), spy, matchflow.Spec{
		Profile:   matchflow.ProfileVendor,
		Deadline:  90 * time.Second,
		Questions: questions,
	})

	if spy.err != nil {
		t.Fatalf("call received invalid attempt deadline: %v", spy.err)
	}
	if stats.Requests == 0 {
		t.Fatal("expected at least 1 request")
	}
}

// TestEngine_MaxThreeAttemptsPerQuestion verifies no question is attempted > 3 times
// across initial proposal, split, retry, and verification.
func TestEngine_MaxThreeAttemptsPerQuestion(t *testing.T) {
	attemptsPerKey := make(map[string]int)
	var mu sync.Mutex

	flakyAI := &flakyAttemptAI{
		onAttempt: func(key string) {
			mu.Lock()
			attemptsPerKey[key]++
			mu.Unlock()
		},
	}

	// 10 items in one request that will fail and trigger splits/retries
	var questions []matchflow.Question
	for i := 1; i <= 10; i++ {
		key := fmt.Sprintf("question_%d", i)
		questions = append(questions, matchflow.Question{
			Key: key,
			Item: matchflow.Item{
				Ref:     i,
				Text:    fmt.Sprintf("text_%d", i),
				Options: []int64{int64(i * 10)},
			},
			Window: []matchflow.CatalogEntry{
				{ProductID: int64(i * 10), NameAR: fmt.Sprintf("p_%d", i)},
			},
		})
	}

	matchflow.Run(context.Background(), flakyAI, matchflow.Spec{
		Profile:    matchflow.ProfileVendor,
		VerifyPass: true,
		Deadline:   10 * time.Second,
		Questions:  questions,
	})

	mu.Lock()
	defer mu.Unlock()
	for key, count := range attemptsPerKey {
		if count > 3 {
			t.Errorf("question %s had %d attempts, exceeded max 3", key, count)
		}
	}
}

// TestEngine_SeparateCounters verifies that unattempted, abstained, rejected,
// and accepted_new are counted separately.
func TestEngine_SeparateCounters(t *testing.T) {
	// Items 1, 2, 3 are in the first batch (accepted, abstained, rejected).
	// Items 4..25 form a second batch that exceeds budget.MaxRequests=1 and remain unattempted.
	var questions []matchflow.Question
	responses := map[int]matchflow.Decision{
		1: {Ref: 1, ProductID: int64Ptr(101), Confidence: 0.95}, // accepted
		2: {Ref: 2, ProductID: nil, Confidence: 0.95},            // abstained
		3: {Ref: 3, ProductID: int64Ptr(9999), Confidence: 0.95}, // rejected (not offered)
	}

	for i := 1; i <= 25; i++ {
		questions = append(questions, matchflow.Question{
			Key: fmt.Sprintf("q%d", i),
			Item: matchflow.Item{
				Ref:     i,
				Text:    fmt.Sprintf("item %d", i),
				Options: []int64{int64(100 + i)},
			},
			Window: []matchflow.CatalogEntry{{ProductID: int64(100 + i), NameAR: fmt.Sprintf("p%d", i)}},
		})
	}

	customAI := &scriptedOutcomeAI{responses: responses}
	budget := matchflow.NewBudget(1, 0, 0, money.Zero) // Allow only 1 request (cap is 20 items per request)

	stats := matchflow.Run(context.Background(), customAI, matchflow.Spec{
		Profile:           matchflow.ProfileVendor,
		Budget:            budget,
		MaxCostPerAttempt: money.Zero,
		Questions:         questions,
	})

	if stats.AcceptedNew != 1 {
		t.Errorf("expected accepted_new=1, got %d", stats.AcceptedNew)
	}
	if stats.Abstained != 1 {
		t.Errorf("expected abstained=1, got %d", stats.Abstained)
	}
	if stats.Rejected != 1 {
		t.Errorf("expected rejected=1, got %d", stats.Rejected)
	}
	if stats.Unattempted != 5 {
		t.Errorf("expected unattempted=5, got %d", stats.Unattempted)
	}
}

// TestEngine_ZeroGainRunReportsZero asserts that runs with zero gain report 0.
func TestEngine_ZeroGainRunReportsZero(t *testing.T) {
	sim := matchflow.NewSimulator(matchflow.SimulatorModeDown)

	questions := []matchflow.Question{
		{
			Key: "q1",
			Item: matchflow.Item{
				Ref:     1,
				Text:    "p1",
				Options: []int64{101},
			},
			Window: []matchflow.CatalogEntry{{ProductID: 101, NameAR: "p1"}},
		},
	}

	stats := matchflow.Run(context.Background(), sim, matchflow.Spec{
		Profile:   matchflow.ProfileVendor,
		Deadline:  2 * time.Second,
		Questions: questions,
	})

	if stats.AcceptedNew != 0 {
		t.Fatalf("expected accepted_new=0 on zero-gain run, got %d", stats.AcceptedNew)
	}
}

type attemptSpy struct {
	maxSeenTimeout time.Duration
	enhanceFunc    func(context.Context, matchflow.Batch) ([]matchflow.Decision, error)
	err            error
	mu             sync.Mutex
}

func (s *attemptSpy) Enhance(ctx context.Context, b matchflow.Batch) ([]matchflow.Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dl, ok := ctx.Deadline()
	if ok {
		rem := time.Until(dl)
		if rem > s.maxSeenTimeout {
			s.maxSeenTimeout = rem
		}
	}
	if s.enhanceFunc != nil {
		ds, err := s.enhanceFunc(ctx, b)
		if err != nil {
			s.err = err
		}
		return ds, err
	}
	return nil, nil
}

type flakyAttemptAI struct {
	onAttempt func(string)
}

func (f *flakyAttemptAI) Enhance(ctx context.Context, b matchflow.Batch) ([]matchflow.Decision, error) {
	for _, it := range b.Items {
		f.onAttempt(fmt.Sprintf("question_%d", it.Ref))
	}
	// Always fail to trigger maximum retries / splits
	return nil, fmt.Errorf("simulated failure")
}

func (f *flakyAttemptAI) Verify(ctx context.Context, b matchflow.CheckBatch) ([]matchflow.Check, error) {
	for _, p := range b.Pairs {
		f.onAttempt(fmt.Sprintf("question_%d", p.Item.Ref))
	}
	return nil, fmt.Errorf("simulated verify failure")
}

type scriptedOutcomeAI struct {
	responses map[int]matchflow.Decision
}

func (s *scriptedOutcomeAI) Enhance(_ context.Context, b matchflow.Batch) ([]matchflow.Decision, error) {
	var out []matchflow.Decision
	for _, it := range b.Items {
		if resp, ok := s.responses[it.Ref]; ok {
			out = append(out, resp)
		}
	}
	return out, nil
}

func int64Ptr(v int64) *int64 {
	return &v
}
