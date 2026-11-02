package smartorder

import "context"

// The smart order's answers, filed where the rest of the platform reads memory.
//
// catalog.match_decisions became read-only when catalog.match_knowledge replaced
// it (US8 T114), and this repository's SaveDecisions sat as a documented no-op
// afterwards: the pipeline's accepted answers were applied to the order and then
// forgotten, so the next order that asked the same question paid the model again
// and ذاكرة قرارات المطابقة never showed the answer.
//
// Routing the write needs no knowledge of the catalogue here — this module may
// not import the catalog module — so the decorator below takes an injectable
// writer, and the composition root supplies one backed by the matching service.
// That keeps the module boundary intact and the behaviour testable with a
// recorder.

// DecisionMemoryWriter files accepted answers in the shared decision memory.
type DecisionMemoryWriter interface {
	SaveDecisions(ctx context.Context, decisions []CachedDecision) error
}

// WithDecisionMemory returns a repository whose SaveDecisions goes to the shared
// memory while every other call is the repository's own.
//
// A nil writer leaves the repository as it is: forgetting answers is a slower
// platform, never a broken one, which is what lets a deployment without the
// matching service keep working.
func WithDecisionMemory(repo Repository, writer DecisionMemoryWriter) Repository {
	if writer == nil {
		return repo
	}
	return &decisionMemoryRepository{Repository: repo, writer: writer}
}

type decisionMemoryRepository struct {
	Repository
	writer DecisionMemoryWriter
}

// SaveDecisions writes to the shared memory instead of the retired table.
func (r *decisionMemoryRepository) SaveDecisions(ctx context.Context, decisions []CachedDecision) error {
	return r.writer.SaveDecisions(ctx, decisions)
}
