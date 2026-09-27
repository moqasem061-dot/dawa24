package catalog

import (
	"context"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxFallbackSearchTerms = 6
	maxFallbackCandidates  = 1000
)

func (s *Service) searchByTerms(ctx context.Context, params SearchParams) ([]*Product, error) {
	terms := fallbackSearchTerms(params.Query)
	if len(terms) < 2 {
		return nil, nil
	}
	results, err := s.searchTermMatches(ctx, params, terms)
	if err != nil {
		return nil, err
	}
	return results.ranked(params.Limit, params.Offset), nil
}

func (s *Service) searchTermMatches(ctx context.Context, params SearchParams, terms []string) (*rankedProductResults, error) {
	limit := fallbackCandidateLimit(params.Limit, params.Offset)
	results := newRankedProductResults()
	for _, term := range terms {
		termParams := params
		termParams.Query = term
		termParams.FirstWord = FirstWordOf(term)
		termParams.Offset = 0
		termParams.Limit = limit
		products, err := s.repo.SearchProducts(ctx, termParams)
		if err != nil {
			return nil, err
		}
		results.add(products)
	}
	return results, nil
}

type rankedProductResults struct {
	byID    map[int64]*Product
	counts  map[int64]int
	ordered []int64
	seen    map[int64]struct{}
}

func newRankedProductResults() *rankedProductResults {
	return &rankedProductResults{
		byID:   make(map[int64]*Product),
		counts: make(map[int64]int),
		seen:   make(map[int64]struct{}),
	}
}

func (r *rankedProductResults) add(products []*Product) {
	for _, product := range products {
		if product == nil {
			continue
		}
		if _, ok := r.seen[product.ID]; !ok {
			r.ordered = append(r.ordered, product.ID)
			r.seen[product.ID] = struct{}{}
		}
		r.byID[product.ID] = product
		r.counts[product.ID]++
	}
}

func (r *rankedProductResults) ranked(limit, offset int) []*Product {
	ordered := make([]*Product, 0, len(r.ordered))
	for _, id := range r.ordered {
		ordered = append(ordered, r.byID[id])
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return r.counts[ordered[i].ID] > r.counts[ordered[j].ID]
	})
	limit = fallbackResultLimit(limit)
	if offset < 0 {
		offset = 0
	}
	if offset >= len(ordered) {
		return nil
	}
	ordered = ordered[offset:]
	if len(ordered) > limit {
		ordered = ordered[:limit]
	}
	return ordered
}

func fallbackSearchTerms(query string) []string {
	seen := make(map[string]struct{})
	terms := make([]string, 0, maxFallbackSearchTerms)
	for _, raw := range strings.Fields(query) {
		term := cleanFallbackSearchTerm(raw)
		if term == "" {
			continue
		}
		if addFallbackSearchTerm(seen, &terms, term) {
			continue
		}
		if len(terms) == maxFallbackSearchTerms {
			break
		}
	}
	return terms
}

func cleanFallbackSearchTerm(raw string) string {
	term := strings.TrimFunc(raw, func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSymbol(r) })
	if utf8.RuneCountInString(term) < 2 && !containsDigit(term) {
		return ""
	}
	return term
}

func containsDigit(value string) bool {
	for _, r := range value {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func addFallbackSearchTerm(seen map[string]struct{}, terms *[]string, term string) bool {
	key := strings.ToLower(term)
	if _, ok := seen[key]; ok {
		return true
	}
	seen[key] = struct{}{}
	*terms = append(*terms, term)
	return false
}

func fallbackCandidateLimit(limit, offset int) int {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	if limit > maxFallbackCandidates-offset {
		return maxFallbackCandidates
	}
	limit += offset
	if limit < 100 {
		return 100
	}
	return limit
}

func fallbackResultLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > maxFallbackCandidates {
		return maxFallbackCandidates
	}
	return limit
}
