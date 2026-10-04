package compare

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// MarketCatalogSearcher retrieves public catalog candidates without creating a
// match run or exposing an organization's private matching knowledge.
type MarketCatalogSearcher interface {
	SearchMarketCatalog(context.Context, string) ([]matchflow.CandidateView, error)
}

func (s *Service) SetMarketCatalogSearcher(searcher MarketCatalogSearcher) {
	s.marketSearcher = searcher
}

func (s *Service) marketCatalogIDs(ctx context.Context, query string) []int64 {
	if s.marketSearcher == nil || len([]rune(query)) < 2 {
		return nil
	}
	key := fmt.Sprintf("compare:market:catalog:v1:%x", sha256.Sum256([]byte(query)))
	var ids []int64
	if s.cache != nil && s.cache.GetJSON(ctx, key, &ids) == nil {
		return ids
	}
	searchCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	candidates, err := s.marketSearcher.SearchMarketCatalog(searchCtx, query)
	if err != nil {
		s.log.WarnContext(ctx, "market catalog search unavailable; using indexed text search", "error", err)
		return nil
	}
	ids = eligibleMarketCatalogIDs(candidates)
	if s.cache != nil {
		_ = s.cache.SetJSON(ctx, key, ids, marketCountTTL)
	}
	return ids
}

func marketQuery(query string) string {
	query = strings.Join(strings.Fields(query), " ")
	chars := []rune(query)
	if len(chars) > 128 {
		query = string(chars[:128])
	}
	return query
}

func eligibleMarketCatalogIDs(candidates []matchflow.CandidateView) []int64 {
	var ids []int64
	seen := make(map[int64]bool)
	for _, candidate := range candidates {
		if candidate.ProductID > 0 && candidate.Eligible && candidate.Score >= 0.3 && !seen[candidate.ProductID] {
			seen[candidate.ProductID] = true
			ids = append(ids, candidate.ProductID)
		}
		if len(ids) == 50 {
			break
		}
	}
	return ids
}
