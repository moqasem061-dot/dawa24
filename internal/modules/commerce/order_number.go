package commerce

import (
	"context"
	"time"
)

// OrderNumberAllocator is implemented by a repository that numbers orders from
// a database sequence (migration 258). Every server process then draws from the
// same sequence, so two processes can never print the same number.
type OrderNumberAllocator interface {
	NextOrderNumber(ctx context.Context) (string, error)
}

// nextOrderNumber asks the repository's sequence when it has one. Otherwise it
// falls back to the in-process counter, which is only unique inside one process.
func (s *Service) nextOrderNumber(ctx context.Context, now time.Time) (string, error) {
	if allocator, ok := s.repo.(OrderNumberAllocator); ok {
		return allocator.NextOrderNumber(ctx)
	}
	return GenerateOrderNumber(now, orderCounter.Add(1)), nil
}
