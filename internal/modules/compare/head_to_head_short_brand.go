package compare

import "github.com/moqasem061-dot/dawa24/internal/shared/productmatch"

// shortBrandPairs is head-to-head's last matching stage: brands whose names are
// too short for the catalogue matcher's skeleton fold (Mobic, Tareg, Daonil)
// but that two supplier lists spell in different alphabets.
//
// A key is used only when it identifies exactly one row in each file. Two rows
// on either side sharing it means the short skeleton is ambiguous here, and an
// unmatched row is better than a price compared against the wrong medicine.
type shortBrandPairs struct {
	sourceCount map[string]int
	targets     map[string][]*CompareFileRow
}

func newShortBrandPairs(sourceRows, targetRows []*CompareFileRow) shortBrandPairs {
	p := shortBrandPairs{
		sourceCount: make(map[string]int, len(sourceRows)),
		targets:     make(map[string][]*CompareFileRow, len(targetRows)),
	}
	for _, sr := range sourceRows {
		if k := productmatch.ShortBrandKey(sr.RawName); k != "" {
			p.sourceCount[k]++
		}
	}
	for _, tr := range targetRows {
		if k := productmatch.ShortBrandKey(tr.RawName); k != "" {
			p.targets[k] = append(p.targets[k], tr)
		}
	}
	return p
}

// pair returns the one target row sr pairs with, or nil.
func (p shortBrandPairs) pair(sr *CompareFileRow) *CompareFileRow {
	k := productmatch.ShortBrandKey(sr.RawName)
	if k == "" || p.sourceCount[k] != 1 {
		return nil
	}
	candidates := p.targets[k]
	if len(candidates) != 1 || !isPriceCompatible(sr.Price, candidates[0].Price) {
		return nil
	}
	cand := candidates[0]
	// Short-brand grouping alone must never group without SameSellableItem confirmation
	srFacts := productmatch.ExtractRowFacts(&productmatch.Row{Name: sr.RawName, SKU: sr.SKU}, nil)
	trFacts := productmatch.ExtractRowFacts(&productmatch.Row{Name: cand.RawName, SKU: cand.SKU}, nil)
	if same, _ := productmatch.SameSellableItem(srFacts, trFacts, nil); !same {
		return nil
	}
	return cand
}
