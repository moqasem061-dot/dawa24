package productmatch

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Two spellings of an international unit that no amount of unit vocabulary can
// read, because the damage is done before the unit is reached.
//
//	"a-viton 50.000 i.u. 20 caps"  ->  no strength at all
//
// "i.u." is not "iu" — the dots make it three tokens — and "50.000" is the
// European thousands separator every Egyptian pack of vitamin A and D is
// printed with, which parses as fifty. So the row stated nothing, and a
// 50,000 IU capsule was free to match a 100 mg one.
//
// The thousands rule is deliberately narrow: it applies only where the unit
// that follows is an international unit, because IU doses in this catalogue are
// large whole numbers and never fractional. Applied generally it would rewrite
// "0.075 %", which is a real concentration.
var (
	dottedIU     = regexp.MustCompile(`(?i)\bi\s*\.\s*u\s*\.?`)
	iuThousands  = regexp.MustCompile(`(?i)(\d{1,3})\.(\d{3})(\s*(?:iu|وحدة|وحده))`)
	dottedLatinU = regexp.MustCompile(`(?i)\bu\s*\.\s*i\s*\.?`)
)

// FoldDoseText rewrites the dose spellings that defeat strengthPattern.
//
// Exported so the importers that read a concentration column directly get the
// same reading as the matcher. Two normalisers that disagree about what "50.000
// i.u." means is the class of bug this package exists to stop.
func FoldDoseText(text string) string {
	if text == "" {
		return text
	}
	// A combination written with a dash is the same combination.
	//
	// The catalogue writes "10/160مجم" and half the supplier files write
	// "10-160 مجم", "100 - 60 مجم", "30-2 مجم". Only the slash form parsed as a
	// ratio, so the row stated ONE component where the product stated two, and
	// dosesPartsDiffer — the check that tells اتاكاند from اتاكاند بلس — fired
	// against a row whose name, pack count and printed price agreed to the
	// piastre. It was the whole of the dose-parts refusals on every live file
	// measured.
	//
	// Only between figures. A dash elsewhere is a name separator — "اوست - ماب",
	// "بيلوبا-سيل" — and is left exactly where it is.
	text = dashRatio.ReplaceAllString(text, "$1/$2")
	if strings.ContainsRune(text, '.') {
		text = dottedIU.ReplaceAllString(text, "iu")
		text = dottedLatinU.ReplaceAllString(text, "iu")
		text = iuThousands.ReplaceAllString(text, "${1}${2}${3}")
	}
	return text
}

// dashRatio matches the dash an Egyptian price list writes between the halves
// of a combination dose, with or without the spaces around it.
var dashRatio = regexp.MustCompile(`(\d)\s*[-–—]\s*(\d)`)

// Reading every dose a name states, including the ones written as a ratio.
//
// This file moved out of recall.go, and it moved because it was wrong in a way
// that cost the engine four of the twenty-five wrong matches it was measured
// making.
//
// strengthPattern captures "a figure, optionally a second figure after a
// separator, then a unit". So "10/20mg" arrives here as ONE match containing
// both figures — and parseStrength, whose job is to answer "what is this
// product's strength" with a single number, cuts the numeric part at the first
// "/" and keeps the head. The 20 was thrown away.
//
// ratioLeads could not recover it, because ratioLeads looks BEHIND the match
// and the 20 is inside it. So:
//
//	"alkor plus 10/20mg"   ->  [10 mg]
//	"الكور بلس 10/40 مجم"  ->  [10 mg]
//	"الكور بلس 20/10 مجم"  ->  [20 mg]
//
// which reads as: the row AGREES with the wrong product and CONTRADICTS the
// right one. Every combination family written "A/B unit" with B a whole number
// behaved this way — Alkor Plus, Amloveran, Amlosazide, Kerdipine Plus — and
// the engine applied a sibling at the wrong dose with "تطابق التركيز" printed
// beside it as the reason.
//
// The correction is to expand the whole ratio rather than its head. Order does
// not matter and must not: comparison is set-based, so 10/20 and 20/10 are the
// same product written by two people, while 10/40 is a different one. That is
// the property the fix exists to restore.

// maxRatioParts bounds how many components one dose may carry.
//
// Four. The widest real combination in this catalogue is three
// ("املوفيران بلس 10/2.5/5مجم"), and a fourth is allowed for the ones written
// with a vehicle. Past that the figures are a lot number or a date, and reading
// them as doses would invent contradictions.
const maxRatioParts = 4

// strengthSet is every dose a text states, not merely the first.
//
// parseStrength answers "what is this product strength", which is the right
// question for scoring and the wrong one for a veto. A catalogue entry written
// "اتاكاند بلس 32/25 ملجم" states two doses, and a pharmacy line that says 32
// agrees with it — but parseStrength reads only one figure, so a first-figure
// comparison would block the correct match as a conflict. Blocking correct
// matches is the failure this guard exists to prevent, not to cause.
//
// A ratio's figures reach this function from two places: the ones the pattern
// captured with the unit, and the ones written before it. Both are scaled by
// the unit that governs them, because only the figure carrying the unit was
// scaled by parseStrength. Ratios of *different* units ("250مجم/5مل") do not
// have this shape and are left alone, which is correct: those are
// concentrations, not combinations, and each half is already recorded under its
// own unit.
func strengthSet(text string) []strength {
	norm := FoldDoseText(sheet.NormalizeDigits(text))
	locs := strengthPattern.FindAllStringIndex(norm, -1)
	if len(locs) == 0 {
		return nil
	}

	out := make([]strength, 0, len(locs)*2)
	add := func(s strength) {
		if !s.known() {
			return
		}
		for _, have := range out {
			if have == s {
				return
			}
		}
		out = append(out, s)
	}

	for _, loc := range locs {
		match := norm[loc[0]:loc[1]]
		parsed := parseStrength(match)
		if !parsed.known() {
			continue
		}
		// The figures inside the match, and the ones the writer put in front of
		// it. numeric[0] is the one parseStrength read, so it is the one whose
		// scaling is known and the one every other figure is scaled against.
		numeric := numericComponents(match)
		if len(numeric) == 0 || numeric[0] <= 0 {
			add(parsed)
			continue
		}
		scale := parsed.value / numeric[0]

		leads := ratioLeads(norm[:loc[0]])
		total := len(leads) + len(numeric)
		if total > maxRatioParts {
			// More figures than any combination carries. Trust only the one
			// that carried the unit rather than inventing components.
			parsed.parts = 1
			add(parsed)
			continue
		}

		for _, v := range leads {
			add(strength{value: v * scale, unit: parsed.unit, parts: total})
		}
		for _, v := range numeric {
			add(strength{value: v * scale, unit: parsed.unit, parts: total})
		}
	}
	return out
}

// numericComponents reads the figures of a matched strength's numeric head.
//
// "10/20mg" is two components, "12.5مجم" is one, "5/12.5/40 mg" is three. The
// walk stops at the first rune that can begin a unit, so the unit's own letters
// are never read as a figure.
func numericComponents(match string) []float64 {
	end := 0
	for end < len(match) {
		c := match[end]
		if (c >= '0' && c <= '9') || c == '.' || c == ',' || c == '/' {
			end++
			continue
		}
		break
	}
	head := strings.TrimRight(match[:end], "/.,")
	if head == "" {
		return nil
	}

	var out []float64
	for _, field := range strings.Split(head, "/") {
		field = strings.TrimSpace(strings.Replace(field, ",", ".", 1))
		if field == "" {
			continue
		}
		v, err := strconv.ParseFloat(strings.Trim(field, "."), 64)
		if err != nil || v <= 0 {
			continue
		}
		out = append(out, v)
	}
	return out
}

// ratioLeads reads the figures written BEFORE a matched strength, where the
// text offers them as the leading halves of a ratio.
//
// It walks backwards for as long as the text keeps offering "<number>/", which
// is what makes املوفيران بلس 10/2.5/5مجم state three components rather than
// two. Stopping at the first one left the widest combination families
// indistinguishable from their two-part siblings.
//
// A slash preceded by a UNIT rather than by a figure ends the walk, and that is
// the whole safety of it: "250مجم/5مل" is a concentration, not a combination,
// and the 250 is already recorded in its own unit.
func ratioLeads(before string) []float64 {
	var out []float64
	for len(out) < maxRatioParts {
		trimmed := strings.TrimRight(before, " ")
		if !strings.HasSuffix(trimmed, "/") {
			break
		}
		digits := strings.TrimRight(trimmed[:len(trimmed)-1], " ")
		start := len(digits)
		for start > 0 {
			c := digits[start-1]
			if (c >= '0' && c <= '9') || c == '.' || c == ',' {
				start--
				continue
			}
			break
		}
		if start == len(digits) {
			break
		}
		v, err := strconv.ParseFloat(strings.Replace(digits[start:], ",", ".", 1), 64)
		if err != nil || v <= 0 {
			break
		}
		out = append(out, v)
		before = digits[:start]
	}
	return out
}
