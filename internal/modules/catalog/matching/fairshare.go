package matching

import (
	"context"
	"sync"
)

// FairScheduler manages process-wide concurrency for AI enhancement attempts,
// enforcing round-robin fairness across tenant organizations so a large tenant
// cannot starve smaller tenants (FR-087, R9).
type FairScheduler struct {
	mu         sync.Mutex
	maxSlots   int
	usedSlots  int
	orgQueues  map[int64][]chan struct{}
	activeOrgs []int64
	nextOrgIdx int
}

// GlobalFairScheduler is the default process-wide attempt scheduler.
var GlobalFairScheduler = NewFairScheduler(10)

// NewFairScheduler creates a new FairScheduler with the specified total concurrency slots.
func NewFairScheduler(maxSlots int) *FairScheduler {
	if maxSlots <= 0 {
		maxSlots = 10
	}
	return &FairScheduler{
		maxSlots:  maxSlots,
		orgQueues: make(map[int64][]chan struct{}),
	}
}

// AcquireSlot acquires an execution slot for the specified organization.
// If all slots are occupied, the request waits fairly in a round-robin queue.
// The returned release function must be invoked when the attempt finishes.
func (s *FairScheduler) AcquireSlot(ctx context.Context, orgID int64) (func(), error) {
	s.mu.Lock()

	// If a slot is immediately available and no other orgs are waiting
	if s.usedSlots < s.maxSlots && len(s.activeOrgs) == 0 {
		s.usedSlots++
		s.mu.Unlock()
		return s.makeRelease(), nil
	}

	// Queue the request for this org
	waitCh := make(chan struct{})
	queue, exists := s.orgQueues[orgID]
	if !exists || len(queue) == 0 {
		s.activeOrgs = append(s.activeOrgs, orgID)
	}
	s.orgQueues[orgID] = append(queue, waitCh)
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		s.mu.Lock()
		// Remove waitCh from org queue if still waiting
		s.dequeue(orgID, waitCh)
		s.mu.Unlock()
		return nil, ctx.Err()
	case <-waitCh:
		return s.makeRelease(), nil
	}
}

func (s *FairScheduler) makeRelease() func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			s.release()
		})
	}
}

func (s *FairScheduler) release() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.activeOrgs) == 0 {
		s.usedSlots--
		if s.usedSlots < 0 {
			s.usedSlots = 0
		}
		return
	}

	// Round-robin selection of the next active org
	if s.nextOrgIdx >= len(s.activeOrgs) {
		s.nextOrgIdx = 0
	}
	chosenOrg := s.activeOrgs[s.nextOrgIdx]
	s.nextOrgIdx = (s.nextOrgIdx + 1) % len(s.activeOrgs)

	queue := s.orgQueues[chosenOrg]
	waiter := queue[0]
	s.orgQueues[chosenOrg] = queue[1:]

	if len(s.orgQueues[chosenOrg]) == 0 {
		delete(s.orgQueues, chosenOrg)
		s.removeActiveOrg(chosenOrg)
	}

	close(waiter)
}

func (s *FairScheduler) dequeue(orgID int64, target chan struct{}) {
	queue := s.orgQueues[orgID]
	for i, ch := range queue {
		if ch == target {
			s.orgQueues[orgID] = append(queue[:i], queue[i+1:]...)
			break
		}
	}
	if len(s.orgQueues[orgID]) == 0 {
		delete(s.orgQueues, orgID)
		s.removeActiveOrg(orgID)
	}
}

func (s *FairScheduler) removeActiveOrg(orgID int64) {
	for i, id := range s.activeOrgs {
		if id == orgID {
			s.activeOrgs = append(s.activeOrgs[:i], s.activeOrgs[i+1:]...)
			if s.nextOrgIdx >= len(s.activeOrgs) {
				s.nextOrgIdx = 0
			}
			break
		}
	}
}
