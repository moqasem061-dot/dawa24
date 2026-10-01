package productmatch

import (
	"strings"
)

// normalizePackUnit standardizes packaging unit names across Arabic and English variants.
func normalizePackUnit(u string) string {
	u = strings.TrimSpace(strings.ToLower(u))
	u = strings.TrimPrefix(u, "\x00")
	switch {
	case u == "strip" || u == "strips" || u == "blister" || u == "شريط" || u == "شرائط" || u == "شرايط" || u == "اشرطه" || u == "بليستر":
		return "strip"
	case u == "tablet" || u == "tablets" || u == "tab" || u == "tabs" || u == "قرص" || u == "أقراص" || u == "اقراص":
		return "tablet"
	case u == "capsule" || u == "capsules" || u == "cap" || u == "caps" || u == "كبسول" || u == "كبسولات":
		return "capsule"
	case u == "box" || u == "علبة" || u == "علبه" || u == "علب" || u == "carton" || u == "كرتونة" || u == "pack" || u == "package" || u == "باكت":
		return "box"
	case u == "bottle" || u == "زجاجة" || u == "ازازة" || u == "قارورة":
		return "bottle"
	case u == "ampoule" || u == "ampoules" || u == "amp" || u == "amps" || u == "أمبول" || u == "امبول" || u == "امبولات":
		return "ampoule"
	case u == "vial" || u == "vials" || u == "فيال" || u == "فيالات":
		return "vial"
	case u == "sachet" || u == "sachets" || u == "كيس" || u == "أكياس" || u == "اكياس":
		return "sachet"
	case u == "unit" || u == "piece" || u == "pcs" || u == "وحدة":
		return "unit"
	default:
		return u
	}
}

// Convert attempts to convert qty from fromUnit to toUnit using approved pack relationships.
// It uses exact integer math only and returns false when there is no relationship or conversion is non-integral.
func Convert(qty int, fromUnit, toUnit string, rels []PackRelationship) (int, bool) {
	from := normalizePackUnit(fromUnit)
	to := normalizePackUnit(toUnit)
	if from == "" || to == "" {
		return 0, false
	}
	if from == to {
		return qty, true
	}
	if len(rels) == 0 {
		return 0, false
	}

	type edge struct {
		to     string
		factor int
		isMul  bool
		relID  int64
	}

	adj := make(map[string][]edge)
	for _, r := range rels {
		if r.RevokedAt != nil || r.Factor <= 0 {
			continue
		}
		rf := normalizePackUnit(r.FromUnit)
		rt := normalizePackUnit(r.ToUnit)
		if rf == "" || rt == "" || rf == rt {
			continue
		}
		adj[rf] = append(adj[rf], edge{to: rt, factor: r.Factor, isMul: true, relID: r.ID})
		adj[rt] = append(adj[rt], edge{to: rf, factor: r.Factor, isMul: false, relID: r.ID})
	}

	type queueItem struct {
		unit string
		curr int
	}

	visited := make(map[string]bool)
	queue := []queueItem{{unit: from, curr: qty}}
	visited[from] = true

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.unit == to {
			return item.curr, true
		}

		for _, e := range adj[item.unit] {
			var nextVal int
			if e.isMul {
				nextVal = item.curr * e.factor
			} else {
				if item.curr%e.factor != 0 {
					continue // exact integer only
				}
				nextVal = item.curr / e.factor
			}

			if e.to == to {
				return nextVal, true
			}

			if !visited[e.to] {
				visited[e.to] = true
				queue = append(queue, queueItem{unit: e.to, curr: nextVal})
			}
		}
	}

	return 0, false
}

func findBridgingRel(qtyFrom int, fromUnit string, qtyTo int, toUnit string, rels []PackRelationship) int64 {
	from := normalizePackUnit(fromUnit)
	to := normalizePackUnit(toUnit)

	for _, r := range rels {
		if r.RevokedAt != nil || r.Factor <= 0 {
			continue
		}
		rf := normalizePackUnit(r.FromUnit)
		rt := normalizePackUnit(r.ToUnit)

		if rf == from && rt == to && qtyFrom*r.Factor == qtyTo {
			return r.ID
		}
		if rf == to && rt == from && qtyTo*r.Factor == qtyFrom {
			return r.ID
		}
		if (from == "" || to == "" || from == "unit" || to == "unit") &&
			(qtyFrom*r.Factor == qtyTo || qtyTo*r.Factor == qtyFrom) {
			return r.ID
		}
	}
	return 0
}

// PackOutcome evaluates pack alignment between row and product facts.
// Returns one of "agree", "conflict", "unstated", "needs_relationship", plus any bridging relationship ID.
func PackOutcome(row, product PackFacts, rels []PackRelationship) (outcome string, relID int64) {
	rowEmpty := row.Count <= 0 && row.Multiplier <= 0 && row.SubUnit == "" && row.Container == ""
	prodEmpty := product.Count <= 0 && product.Multiplier <= 0 && product.SubUnit == "" && product.Container == ""

	if rowEmpty || prodEmpty {
		return "unstated", 0
	}

	isRowMulti := row.Multiplier > 1
	isProdMulti := product.Multiplier > 1

	// Normative rule: 2×10 vs 20 => needs_relationship, never agree without relationship
	if isRowMulti != isProdMulti {
		var multiPF, singlePF PackFacts
		if isRowMulti {
			multiPF, singlePF = row, product
		} else {
			multiPF, singlePF = product, row
		}

		for _, r := range rels {
			if r.RevokedAt != nil || r.Factor <= 0 {
				continue
			}
			if r.Factor == multiPF.SubCount && multiPF.Count == singlePF.Count {
				return "agree", r.ID
			}
			if r.Factor == multiPF.Multiplier && multiPF.Count == singlePF.Count {
				return "agree", r.ID
			}
			if r.Factor == multiPF.Count && singlePF.Count == multiPF.Count {
				return "agree", r.ID
			}
			if multiPF.SubUnit != "" && singlePF.SubUnit != "" {
				if conv, ok := Convert(multiPF.Multiplier, multiPF.SubUnit, singlePF.SubUnit, rels); ok && conv == singlePF.Count {
					return "agree", r.ID
				}
			}
		}

		return "needs_relationship", 0
	}

	// Both are multi-packs (e.g. 2x10 vs 2x10)
	if isRowMulti && isProdMulti {
		if row.Multiplier == product.Multiplier && row.SubCount == product.SubCount {
			return "agree", 0
		}
		return "conflict", 0
	}

	// Neither is a multi-pack
	rUnit := normalizePackUnit(row.SubUnit)
	pUnit := normalizePackUnit(product.SubUnit)

	// Both have specific, distinct units that require relationship
	if rUnit != "" && pUnit != "" && rUnit != pUnit && rUnit != "unit" && pUnit != "unit" {
		if conv, ok := Convert(row.Count, rUnit, pUnit, rels); ok {
			if conv == product.Count {
				bridgingID := findBridgingRel(row.Count, rUnit, product.Count, pUnit, rels)
				return "agree", bridgingID
			}
			return "conflict", 0
		}
		return "needs_relationship", 0
	}

	// Compatible or unstated units
	if row.Count == product.Count && row.Count > 0 {
		return "agree", 0
	}

	// Count differs: check if active rels bridge the difference
	if bridgingID := findBridgingRel(row.Count, rUnit, product.Count, pUnit, rels); bridgingID > 0 {
		return "agree", bridgingID
	}

	if rUnit == "strip" || pUnit == "strip" {
		return "needs_relationship", 0
	}

	return "conflict", 0
}
