package main

import (
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	matchingPostgres "github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// newMatchingService constructs the singleton matching.Service for the platform.
func newMatchingService(db *database.DB) matching.Service {
	repo := matchingPostgres.NewRepository(db)
	curatedRepo := matchingPostgres.NewCuratedRepository(db)
	loader := matching.NewLoader(db)
	cache := matching.NewIndexCache(matching.NewDefaultIndexBuilderWithCuration(loader, curatedRepo))
	return matching.NewService(repo, cache, loader)
}
