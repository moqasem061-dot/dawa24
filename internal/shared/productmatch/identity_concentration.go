package productmatch

import (
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

var (
	concentrationRatioRe = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(mg|مجم|ملجم|g|جم|جرام|gm|mcg|ميكروجرام|iu|وحدة|وحده|%)\s*/\s*(\d+(?:\.\d+)?)?\s*(ml|مللي|مل|لتر|l)?`)
)

// NormalizedConcentration represents an exact rational concentration.
type NormalizedConcentration struct {
	Num   *big.Rat // exact rational value in base units
	Unit  string   // base unit, e.g. "mg/ml", "%", "iu/ml", "iu"
	Valid bool
}

// NormalizeConcentration normalizes a concentration ratio using exact rational arithmetic.
// Rule: mg <-> g and mL <-> L only; percent and IU are never converted.
func NormalizeConcentration(r *Ratio) NormalizedConcentration {
	if r == nil {
		return NormalizedConcentration{}
	}

	numValStr := strings.TrimSpace(r.Num.Value)
	if numValStr == "" {
		return NormalizedConcentration{}
	}
	numValStr = strings.ReplaceAll(numValStr, ",", ".")

	numRat, ok := new(big.Rat).SetString(numValStr)
	if !ok || numRat.Sign() <= 0 {
		return NormalizedConcentration{}
	}

	numUnit := strings.ToLower(strings.TrimSpace(r.Num.Unit))
	var baseNumUnit string
	var numMul int64 = 1

	switch numUnit {
	case "mg", "مجم", "ملجم":
		baseNumUnit = "mg"
		numMul = 1
	case "g", "جم", "جرام", "gm":
		baseNumUnit = "mg"
		numMul = 1000
	case "%":
		baseNumUnit = "%"
	case "iu", "وحدة", "وحده":
		baseNumUnit = "iu"
	default:
		baseNumUnit = numUnit
	}

	if numMul != 1 {
		numRat.Mul(numRat, big.NewRat(numMul, 1))
	}

	if baseNumUnit == "%" {
		return NormalizedConcentration{
			Num:   numRat,
			Unit:  "%",
			Valid: true,
		}
	}

	denVal := r.Den.Value
	if denVal <= 0 {
		denVal = 1.0
	}
	denValStr := strconv.FormatFloat(denVal, 'f', -1, 64)
	denRat, ok := new(big.Rat).SetString(denValStr)
	if !ok || denRat.Sign() <= 0 {
		return NormalizedConcentration{}
	}

	denUnit := strings.ToLower(strings.TrimSpace(r.Den.Unit))
	var baseDenUnit string
	var denMul int64 = 1

	switch denUnit {
	case "ml", "مل", "مللي", "milliliter":
		baseDenUnit = "ml"
		denMul = 1
	case "l", "لتر", "liter":
		baseDenUnit = "ml"
		denMul = 1000
	case "":
		baseDenUnit = "ml"
		denMul = 1
	default:
		baseDenUnit = denUnit
	}

	if denMul != 1 {
		denRat.Mul(denRat, big.NewRat(denMul, 1))
	}

	finalRat := new(big.Rat).Quo(numRat, denRat)
	unit := fmt.Sprintf("%s/%s", baseNumUnit, baseDenUnit)

	return NormalizedConcentration{
		Num:   finalRat,
		Unit:  unit,
		Valid: true,
	}
}

// CompareConcentrations compares two concentration ratios using exact rational normalization.
func CompareConcentrations(rowConc, prodConc *Ratio) Outcome {
	if rowConc == nil && prodConc == nil {
		return OutcomeUnstated
	}
	if rowConc == nil || prodConc == nil {
		return OutcomeUnstated
	}

	rNorm := NormalizeConcentration(rowConc)
	pNorm := NormalizeConcentration(prodConc)

	if !rNorm.Valid || !pNorm.Valid {
		return OutcomeUncertain
	}

	if rNorm.Unit != pNorm.Unit {
		return OutcomeConflict
	}

	if rNorm.Num.Cmp(pNorm.Num) == 0 {
		return OutcomeAgree
	}

	return OutcomeConflict
}

func compareConcentration(row, prod IdentityFacts, comp *Comparison) {
	outcome := CompareConcentrations(row.Concentration, prod.Concentration)
	comp.Attributes["concentration"] = outcome
	if outcome == OutcomeConflict {
		comp.Hard = append(comp.Hard, ConflictConcentration)
	}
}

func extractConcentrationRatio(text string) *Ratio {
	m := concentrationRatioRe.FindStringSubmatch(text)
	if len(m) < 3 {
		return nil
	}

	numVal := m[1]
	numUnit := normalizeUnit(m[2])
	if numUnit == "" {
		return nil
	}

	denVal := 1.0
	if len(m) >= 4 && m[3] != "" {
		if v, err := strconv.ParseFloat(m[3], 64); err == nil && v > 0 {
			denVal = v
		}
	}

	denUnit := "ml"
	if len(m) >= 5 && m[4] != "" {
		u := normalizeUnit(m[4])
		if u != "" {
			denUnit = u
		}
	}

	return &Ratio{
		Num: Strength{
			Value:     numVal,
			Unit:      numUnit,
			Component: 0,
			Prov:      ProvOriginal,
			Cert:      CertParsed,
		},
		Den: Quantity{
			Value: denVal,
			Unit:  denUnit,
			Prov:  ProvOriginal,
			Cert:  CertParsed,
		},
		Prov: ProvOriginal,
		Cert: CertParsed,
	}
}
