package pipeline

import (
	"fmt"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// convertCandidatePack validates pack compatibility and computes converted quantities.
// If requested pack and offered unit differ without an approved relationship, it returns eligible=false, ReasonBlockedPack.
func convertCandidatePack(l *smartorder.Line, o smartorder.Offer, rels []productmatch.PackRelationship) (int, bool, smartorder.IneligibleReason) {
	reqQty := int(l.EffectiveQty)
	if reqQty <= 0 {
		reqQty = 1
	}

	reqPack := extractReqPackSize(l)
	offerPack := extractOfferPackSize(o.Unit)

	// If neither side states a conflicting pack size, no conversion is needed.
	if reqPack == 0 || offerPack == 0 || reqPack == offerPack {
		return reqQty, true, ""
	}

	// Pack size mismatch (e.g. requested pack 30 vs offered unit 10).
	// Attempt conversion via approved pack relationships.
	if len(rels) > 0 {
		// 1. Try standard unit names with productmatch.Convert
		unitPairs := [][2]string{
			{fmt.Sprintf("%d", reqPack), fmt.Sprintf("%d", offerPack)},
			{fmt.Sprintf("pack_%d", reqPack), fmt.Sprintf("pack_%d", offerPack)},
			{"box", "strip"},
			{"علبة", "شريط"},
			{"pack", "unit"},
			{"tablet", "strip"},
			{"strip", "tablet"},
		}
		for _, pair := range unitPairs {
			if conv, ok := productmatch.Convert(reqQty, pair[0], pair[1], rels); ok && conv > 0 {
				return conv, true, ""
			}
		}

		// 2. Check if any relationship matches the exact mathematical ratio between the packs
		for _, r := range rels {
			if r.RevokedAt == nil && r.Factor > 0 {
				if reqPack == offerPack*r.Factor {
					if conv, ok := productmatch.Convert(reqQty, r.FromUnit, r.ToUnit, rels); ok && conv > 0 {
						return conv, true, ""
					}
					// Exact integer conversion by factor
					return reqQty * r.Factor, true, ""
				}
			}
		}
	}

	// No valid relationship bridges the requested pack and offered unit.
	return 0, false, smartorder.ReasonBlockedPack
}

func extractReqPackSize(l *smartorder.Line) int {
	row := BuildRow(l)
	if row.PackSize > 0 {
		return row.PackSize
	}
	facts := productmatch.ExtractRowFacts(row, nil)
	if facts.Pack != nil && facts.Pack.Count > 0 {
		return facts.Pack.Count
	}
	return 0
}

func extractOfferPackSize(unit string) int {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return 0
	}
	if n := atoiSafe(unit); n > 0 {
		return n
	}
	if m := packPattern.FindStringSubmatch(unit); m != nil {
		if n := atoiSafe(m[1]); n > 0 {
			return n
		}
	}
	row := &productmatch.Row{Name: unit}
	facts := productmatch.ExtractRowFacts(row, nil)
	if facts.Pack != nil && facts.Pack.Count > 0 {
		return facts.Pack.Count
	}
	return 0
}
