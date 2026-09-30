package productmatch

import (
	"math"
	"strconv"
)

func compareStrength(row, prod IdentityFacts, comp *Comparison) {
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
		if rLen != pLen {
			comp.Attributes["strength"] = OutcomeUnstated
			return
		}

		allAgree := true
		for i := 0; i < rLen; i++ {
			rS := row.Strengths[i]
			pS := prod.Strengths[i]
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

func compareConcentration(row, prod IdentityFacts, comp *Comparison) {
	if row.Concentration == nil && prod.Concentration == nil {
		comp.Attributes["concentration"] = OutcomeUnstated
		return
	}
	if row.Concentration == nil || prod.Concentration == nil {
		comp.Attributes["concentration"] = OutcomeUnstated
		return
	}

	rRatio := concentrationValue(*row.Concentration)
	pRatio := concentrationValue(*prod.Concentration)

	if rRatio > 0 && pRatio > 0 {
		if math.Abs(rRatio-pRatio) < 1e-4 {
			comp.Attributes["concentration"] = OutcomeAgree
		} else {
			comp.Attributes["concentration"] = OutcomeConflict
			comp.Hard = append(comp.Hard, ConflictConcentration)
		}
	} else {
		comp.Attributes["concentration"] = OutcomeUncertain
	}
}

func concentrationValue(r Ratio) float64 {
	val, err := strconv.ParseFloat(r.Num.Value, 64)
	if err != nil || r.Den.Value <= 0 {
		return -1
	}
	mg := toMilligrams(val, r.Num.Unit)
	if mg <= 0 {
		mg = val
	}
	mL := r.Den.Value
	if r.Den.Unit == "l" {
		mL *= 1000.0
	}
	return mg / mL
}
