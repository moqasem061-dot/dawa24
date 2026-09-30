package matching

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// Repository defines data access for catalog revisions, match policies, and match runs.
type Repository interface {
	CurrentRevision(ctx context.Context, scope string) (int64, error)
	ActivePolicy(ctx context.Context, workflow string, orgID int64) (productmatch.Policy, error)
	CreateRun(ctx context.Context, run *MatchRun) error
	UpdateCounts(ctx context.Context, runID int64, counts matchflow.RunCounts) error
	SetState(ctx context.Context, runID int64, state string, finished bool) error
	Heartbeat(ctx context.Context, runID int64) error
}

// CuratedRepository defines data access for platform-curated identity facts and pack relationships.
type CuratedRepository interface {
	ListActive(ctx context.Context) (map[int64][]productmatch.CuratedFact, map[int64][]productmatch.PackRelationship, error)
}
