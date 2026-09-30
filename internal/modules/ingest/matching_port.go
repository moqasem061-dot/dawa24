package ingest

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// Matcher defines the product matching capability required by the vendor ingest module.
// In accordance with AGENTS.md rule 5, this port uses only shared productmatch and matchflow types.
type Matcher interface {
	Prepare(ctx context.Context, req productmatch.PrepareRequest) (productmatch.PrepareResult, error)
	Rematch(ctx context.Context, s productmatch.Scope, row productmatch.Row, rev int) (matchflow.RowDecision, error)
	Revalidate(ctx context.Context, s productmatch.Scope, ref productmatch.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error)
}
