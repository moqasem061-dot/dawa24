package pipeline

import (
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// matchCandidate builds a shortlist entry for tests.
func matchCandidate(id int64, name string) productmatch.MatchCandidate {
	return productmatch.MatchCandidate{ProductID: id, Name: name, Score: 0.5}
}

// isSummaryForTest reaches the domain helper without importing it into every test.
func isSummaryForTest(s string) bool { return smartorder.IsSummaryRow(s) }
