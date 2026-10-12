package productmatch

import (
	"unicode"
	"unicode/utf8"

	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

// A combination such as 5/10 mg is not a dilution; the denominator must name a volume.
func statedConcentration(name string) *Ratio {
	normalized := FoldDoseText(sheet.NormalizeDigits(name))
	parts := concentrationRatioRe.FindStringSubmatchIndex(normalized)
	if len(parts) < 10 || parts[8] < 0 {
		return nil
	}
	if parts[9] < len(normalized) {
		next, _ := utf8.DecodeRuneInString(normalized[parts[9]:])
		if unicode.IsLetter(next) {
			return nil
		}
	}
	return extractConcentrationRatio(normalized)
}

func doseConcentrationOutcome(q *query, facts *nameFacts) Outcome {
	outcome := CompareConcentrations(q.concentration, facts.concentration)
	// Equal density does not make 10mg/5mL and 50mg/25mL vials interchangeable.
	if outcome == OutcomeAgree && (q.formKey != "liquid" || q.injForm != "") {
		return OutcomeUnstated
	}
	return outcome
}
