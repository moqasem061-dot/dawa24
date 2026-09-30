package productmatch

import (
	"math"
	"strconv"
)

func compareStrength(row, prod IdentityFacts, comp *Comparison) {
	// Concentration ratio agreement overrides raw strength comparison
	if row.Concentration != nil && prod.Concentration != nil {
		cOutcome := CompareConcentrations(row.Concentration, prod.Concentration)
		if cOutcome == OutcomeAgree {
			comp.Attributes["strength"] = OutcomeAgree
			return
		}
		if cOutcome == OutcomeConflict {
			comp.Attributes["strength"] = OutcomeConflict
			comp.Hard = append(comp.Hard, ConflictStrength)
			return
		}
	}

	if len(row.Strengths) == 0 && len(prod.Strengths) == 0 {
		comp.Attributes["strength"] = OutcomeUnstated
		return
	}
	if len(row.Strengths) == 0 || len(prod.Strengths) == 0 {
		comp.Attributes["strength"] = OutcomeUnstated
		return
	}

	rLen := len(row.Strengths)
	pLen := len(prod.Strengths)

	if rLen > 1 || pLen > 1 {
		if rLen < pLen {
			// A row with one component against a multi-component product is unstated, never agree
			comp.Attributes["strength"] = OutcomeUnstated
			return
		}
		if rLen > pLen {
			// Row stated more components than candidate product
			comp.Attributes["strength"] = OutcomeConflict
			comp.Hard = append(comp.Hard, ConflictCombination)
			return
		}

		allAgree := true
		for i := 0; i < rLen; i++ {
			rS := row.Strengths[i]
			pS := prod.Strengths[i]

			// Compare strength components by position only when Component is known
			if rS.Component < 0 || pS.Component < 0 {
				comp.Attributes["strength"] = OutcomeUnstated
				return
			}
			if rS.Component != pS.Component {
				allAgree = false
				comp.Hard = append(comp.Hard, ConflictStrengthComponent)
				break
			}

			if !strengthsEqual(rS, pS) {
				allAgree = false
				comp.Hard = append(comp.Hard, ConflictStrengthComponent)
				break
			}
		}

		if allAgree {
			comp.Attributes["strength"] = OutcomeAgree
		} else {
			comp.Attributes["strength"] = OutcomeConflict
			comp.Hard = append(comp.Hard, ConflictCombination)
		}
		return
	}

	rS := row.Strengths[0]
	pS := prod.Strengths[0]

	if strengthsEqual(rS, pS) {
		comp.Attributes["strength"] = OutcomeAgree
	} else {
		comp.Attributes["strength"] = OutcomeConflict
		comp.Hard = append(comp.Hard, ConflictStrength)
	}
}

func strengthsEqual(a, b Strength) bool {
	if a.Value == b.Value && a.Unit == b.Unit {
		return true
	}

	vA, errA := strconv.ParseFloat(a.Value, 64)
	vB, errB := strconv.ParseFloat(b.Value, 64)
	if errA != nil || errB != nil {
		return false
	}

	mgA := toMilligrams(vA, a.Unit)
	mgB := toMilligrams(vB, b.Unit)
	if mgA > 0 && mgB > 0 {
		return math.Abs(mgA-mgB) < 1e-4
	}

	if a.Unit == b.Unit {
		return math.Abs(vA-vB) < 1e-4
	}

	return false
}

func toMilligrams(val float64, unit string) float64 {
	switch unit {
	case "mg":
		return val
	case "g":
		return val * 1000.0
	case "mcg":
		return val / 1000.0
	default:
		return -1
	}
}
