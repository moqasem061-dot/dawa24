package main

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	matchingPostgres "github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// newMatchingService constructs the singleton matching.Service for the platform.
func newMatchingService(db *database.DB) matching.Service {
	repo := matchingPostgres.NewRepository(db)
	curatedRepo := matchingPostgres.NewCuratedRepository(db)
	loader := matching.NewLoader(db)
	cache := matching.NewIndexCache(matching.NewDefaultIndexBuilderWithCuration(loader, curatedRepo))
	return matching.NewService(repo, cache, loader)
}

// compareMatcherAdapter adapts matching.Service to compare.Matcher.
type compareMatcherAdapter struct {
	svc matching.Service
}

func (a *compareMatcherAdapter) Prepare(ctx context.Context, req productmatch.PrepareRequest) (productmatch.PrepareResult, error) {
	return a.svc.Prepare(ctx, req)
}

func (a *compareMatcherAdapter) MarkReviewable(ctx context.Context, runID int64) error {
	return a.svc.MarkReviewable(ctx, runID)
}

func (a *compareMatcherAdapter) Rematch(ctx context.Context, s productmatch.Scope, row productmatch.Row, rev int) (matchflow.RowDecision, error) {
	return a.svc.Rematch(ctx, s, row, rev)
}

func (a *compareMatcherAdapter) Revalidate(ctx context.Context, s productmatch.Scope, ref productmatch.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
	return a.svc.Revalidate(ctx, s, ref, d)
}

func newCompareMatcher(svc matching.Service) compare.Matcher {
	return &compareMatcherAdapter{svc: svc}
}
