package ui

import (
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
)

// Carrying the stale-line report from the failed finalisation to the review
// screen that renders it.
//
// It is not persisted, and that is the right call: a stale line describes the
// world at one instant — a coverage window that had just closed, a box that had
// just sold out — and storing it would leave the buyer reading a stale
// explanation of staleness. It survives one redirect and is then discarded; if
// they finalise again, it is recomputed against the world as it is then.

const smartOrderStaleTTL = 15 * time.Minute

type staleEntry struct {
	lines  []smartorder.StaleLine
	stored time.Time
}

type smartOrderStaleStore struct {
	mu      sync.Mutex
	entries map[string]staleEntry
}

func newSmartOrderStaleStore() *smartOrderStaleStore {
	return &smartOrderStaleStore{entries: make(map[string]staleEntry)}
}

func (s *smartOrderStaleStore) put(runID string, lines []smartorder.StaleLine) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictLocked()
	s.entries[runID] = staleEntry{lines: lines, stored: time.Now()}
}

// take returns the report and removes it: it is shown once, on the render that
// follows the refused finalisation.
func (s *smartOrderStaleStore) take(runID string) []smartorder.StaleLine {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictLocked()
	e, ok := s.entries[runID]
	if !ok {
		return nil
	}
	delete(s.entries, runID)
	return e.lines
}

func (s *smartOrderStaleStore) drop(runID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, runID)
}

func (s *smartOrderStaleStore) evictLocked() {
	cutoff := time.Now().Add(-smartOrderStaleTTL)
	for k, e := range s.entries {
		if e.stored.Before(cutoff) {
			delete(s.entries, k)
		}
	}
}
