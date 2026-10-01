package compare

import (
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// isEquivalentOrCanonical implements the grouping gate for compare (T090).
//
// Invariants:
//   - Only groups rows when both have the same accepted product ID at pack granularity,
//     or SameSellableItem returns true (then group_basis='source_equivalence').
//   - If both have the same catalogue product ID, pack outcome is checked:
//     if there is a conflict or needs_relationship without a bridging relationship,
//     the rows are marked not_comparable and group_basis is 'not_comparable'.
//   - Strip vs Box or differing pack sizes without a relationship are never directly comparable.
//   - Returns grouped=true if they should group in comparison results, with the corresponding groupBasis.
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
				if _, ok := convertEquivalentPrice(money.FromMajor(100), srFacts, trFacts, rels); !ok {
					return true, "not_comparable", true
				}
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
func canGroupWithProductRow(r *CompareFileRow, compRow *ProductComparisonRow, rels []productmatch.PackRelationship) (bool, string, bool) {
	if compRow == nil {
		return false, "", false
	}
	candRow := &CompareFileRow{
		RawName:          compRow.ProductName,
		SKU:              compRow.SKU,
		MatchedProductID: compRow.MatchedProductID,
	}
	return isEquivalentOrCanonical(r, candRow, rels)
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

// convertEquivalentPrice converts priceB (the price of item B) to the equivalent
// quantity and packaging unit of item A, using productmatch.Convert only.
// In accordance with AGENTS.md Rule 1, money never touches float64.
// Returns (convertedPrice, true) if comparable/convertible, or (Zero, false) if not comparable.
func convertEquivalentPrice(priceB money.Amount, factsA, factsB productmatch.IdentityFacts, rels []productmatch.PackRelationship) (money.Amount, bool) {
	if factsA.Pack == nil && factsB.Pack == nil {
		return priceB, true
	}
	if factsA.Pack == nil || factsB.Pack == nil {
		return priceB, true
	}

	pA := factsA.Pack
	pB := factsB.Pack

	tryConvert := func(uA string, cA int, uB string, cB int) (money.Amount, bool) {
		if uA == "" || uB == "" {
			return money.Zero, false
		}
		if cA <= 0 {
			cA = 1
		}
		if cB <= 0 {
			cB = 1
		}
		if uA == uB {
			if cA == cB {
				return priceB, true
			}
			// When units are the same but counts differ (e.g. 20 tablets vs 10 tablets),
			// an active bridging relationship is strictly required.
			if bridgingID := productmatch.FindBridgingRel(cA, uA, cB, uB, rels); bridgingID > 0 {
				converted, err := priceB.MulRatio(int64(cA), int64(cB))
				if err == nil {
					return converted, true
				}
			}
			return money.Zero, false
		}
		if convAInB, ok := productmatch.Convert(cA, uA, uB, rels); ok && convAInB > 0 {
			converted, err := priceB.MulRatio(int64(convAInB), int64(cB))
			if err == nil {
				return converted, true
			}
		}
		if convBInA, ok := productmatch.Convert(cB, uB, uA, rels); ok && convBInA > 0 {
			converted, err := priceB.MulRatio(int64(cA), int64(convBInA))
			if err == nil {
				return converted, true
			}
		}
		return money.Zero, false
	}

	// 1. If packaging containers differ (e.g. strip vs box), conversion MUST bridge the containers
	if pA.Container != "" && pB.Container != "" && pA.Container != pB.Container {
		return tryConvert(pA.Container, 1, pB.Container, 1)
	}

	// 2. If both containers are set and equal
	if pA.Container != "" && pB.Container != "" && pA.Container == pB.Container {
		if conv, ok := tryConvert(pA.SubUnit, pA.Count, pB.SubUnit, pB.Count); ok {
			return conv, true
		}
	}

	// 3. Try container vs subunit
	if pA.Container != "" && pB.SubUnit != "" {
		if conv, ok := tryConvert(pA.Container, 1, pB.SubUnit, pB.Count); ok {
			return conv, true
		}
	}
	if pB.Container != "" && pA.SubUnit != "" {
		if conv, ok := tryConvert(pA.SubUnit, pA.Count, pB.Container, 1); ok {
			return conv, true
		}
	}

	// 4. Try subunit conversion
	if pA.SubUnit != "" && pB.SubUnit != "" {
		if conv, ok := tryConvert(pA.SubUnit, pA.Count, pB.SubUnit, pB.Count); ok {
			return conv, true
		}
	}

	return money.Zero, false
}

func isBarcode(s string) bool {
	if len(s) < 8 || len(s) > 14 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

type multiCompBaseEntry struct {
	compRow   *ProductComparisonRow
	packCount float64
	hasCount  bool
}
