package compare

import (
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// isEquivalentOrCanonical implements the grouping gate for compare (T090).
//
// Invariants:
// - Only groups rows when both have the same accepted product ID at pack granularity,
//   or SameSellableItem returns true (then group_basis='source_equivalence').
// - If both have the same catalogue product ID, pack outcome is checked:
//   if there is a conflict or needs_relationship without a bridging relationship,
//   the rows are marked not_comparable and group_basis is 'not_comparable'.
// - Strip vs Box or differing pack sizes without a relationship are never directly comparable.
// - Returns grouped=true if they should group in comparison results, with the corresponding groupBasis.
func isEquivalentOrCanonical(sr, tr *CompareFileRow, rels []productmatch.PackRelationship) (grouped bool, groupBasis string, notComparable bool) {
	if sr == nil || tr == nil {
		return false, "", false
	}

	srFacts := productmatch.ExtractRowFacts(&productmatch.Row{Name: sr.RawName, SKU: sr.SKU}, nil)
	trFacts := productmatch.ExtractRowFacts(&productmatch.Row{Name: tr.RawName, SKU: tr.SKU}, nil)

	// Same master catalogue product ID
	if sr.MatchedProductID != nil && tr.MatchedProductID != nil && *sr.MatchedProductID > 0 && *sr.MatchedProductID == *tr.MatchedProductID {
		if srFacts.Pack != nil && trFacts.Pack != nil {
			outcome, _ := productmatch.PackOutcome(*srFacts.Pack, *trFacts.Pack, rels)
			if outcome == "conflict" || outcome == "needs_relationship" {
				return true, "not_comparable", true
			}
		}
		return true, "canonical_pack", false
	}

	// Not same catalogue ID (or absent from catalogue): requires positive SameSellableItem confirmation
	same, _ := productmatch.SameSellableItem(srFacts, trFacts, rels)
	if same {
		return true, "source_equivalence", false
	}

	return false, "", false
}

// canGroupWithProductRow checks if a file row can group with an existing multi-supplier comparison row.
func canGroupWithProductRow(r *CompareFileRow, compRow *ProductComparisonRow) (bool, string, bool) {
	if compRow == nil {
		return false, "", false
	}
	candRow := &CompareFileRow{
		RawName:          compRow.ProductName,
		SKU:              compRow.SKU,
		MatchedProductID: compRow.MatchedProductID,
	}
	return isEquivalentOrCanonical(r, candRow, nil)
}

type targetRowEntry struct {
	row       *CompareFileRow
	packCount float64
	hasCount  bool
}

// isPriceCompatible checks whether two public prices are within a plausible ratio (0.35x to 2.85x)
// to prevent matching different pack sizes (e.g. 20 tablets vs 200 tablets).
// If either price is zero/unknown, it returns true.
// Uses integer arithmetic on minor units (piastres) in accordance with Rule 1.
func isPriceCompatible(p1, p2 money.Amount) bool {
	if p1.IsZero() || p2.IsZero() {
		return true
	}
	m1 := p1.Minor()
	m2 := p2.Minor()
	if m1 <= 0 || m2 <= 0 {
		return true
	}
	return m1*100 >= m2*35 && m1*100 <= m2*285
}
