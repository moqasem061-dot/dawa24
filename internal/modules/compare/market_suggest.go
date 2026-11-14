package compare

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"
)

// The search box's dropdown.
//
// Typing in the search box used to re-run the whole board on every pause. The
// box now asks this instead: a handful of lines, one per product, each saying
// how many offers the board holds for it, in how many warehouses, at what best
// discount and from what lowest public price. The board itself only changes
// when the reader commits (Enter, "view all", or picking a line).
//
// Decisions worth knowing:
//
//   - No COUNT of the board and no fuzzy retry. The suggestions are a lookup,
//     not a search: the fuzzy catalogue retrieval can take up to 500 ms on its
//     own, which is longer than the whole suggestion budget. A misspelt name
//     gets "no suggestions" here and the full search (which does retry fuzzily)
//     behind the "view all" line.
//   - Prices are read live and never cached, the same rule as the board. The
//     grouping could be cached; at 8 lines it is not worth a second key space.
//   - A deadline, not an error page. When the budget runs out the caller gets
//     ErrSuggestUnavailable and renders an empty dropdown with a note.

const (
	// MarketSuggestMinChars is the shortest query that asks the database.
	MarketSuggestMinChars = 2
	// MarketSuggestMaxChars matches the board's own query limit.
	MarketSuggestMaxChars = 128
	// MarketSuggestMaxItems is the most lines the dropdown shows.
	MarketSuggestMaxItems = 8

	marketSuggestBudget = 400 * time.Millisecond
)

// ErrSuggestUnavailable means suggestions could not be produced in time (or at
// all). It is not a reason to fail a page.
var ErrSuggestUnavailable = errors.New("compare: market suggestions unavailable")

// marketSuggester is implemented by the Postgres repository, and only by it.
type marketSuggester interface {
	SuggestMarketDiscounts(ctx context.Context, query string, limit int) ([]MarketSuggestion, error)
}

// SuggestMarketDiscounts returns up to limit lines (at most
// MarketSuggestMaxItems) matching query on the public board. A query shorter
// than MarketSuggestMinChars characters returns nothing without touching the
// database.
func (s *Service) SuggestMarketDiscounts(ctx context.Context, query string, limit int) ([]MarketSuggestion, error) {
	query = marketQuery(query)
	if utf8.RuneCountInString(query) < MarketSuggestMinChars {
		return nil, nil
	}
	if limit <= 0 || limit > MarketSuggestMaxItems {
		limit = MarketSuggestMaxItems
	}
	repo, ok := s.repo.(marketSuggester)
	if !ok {
		return nil, ErrSuggestUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, marketSuggestBudget)
	defer cancel()
	out, err := repo.SuggestMarketDiscounts(ctx, query, limit)
	if err != nil {
		s.warn(ctx, "market board: suggestions unavailable", err)
		return nil, ErrSuggestUnavailable
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
