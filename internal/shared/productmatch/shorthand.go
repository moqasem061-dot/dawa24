package productmatch

import (
	"strings"
	"unicode"

	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

// Egyptian order-list shorthand.
//
// Pharmacies and distributors in Egypt abbreviate the dosage form to a single
// letter, almost always glued to the count: "كونكور 5مج 30ق", "اوجمنتين 1جم
// 14ق", "نيكسيوم 40مج 14ك", "ابراميون 100مجم زجاجة ش", "سيفترياكسون 1جم 1ف".
// Read literally, "ق" and "ك" are single Arabic letters, and a single letter
// after a brand is exactly how this engine recognises an identity letter
// (بتنوفيت ان, بتنوفيت سي). So the correct product — which says قرص — was
// contradicted on its "identity letter" and refused, on about one Egyptian
// shorthand row in five.
//
// ExpandShorthand rewrites those abbreviations into the words the rest of the
// engine already understands, before anything else reads the row. It is
// applied to supplier and pharmacy rows only; catalogue names are written out.
//
// Two contexts, because the same letter means different things in them:
//
//   - after a figure it counts something: 30ق is thirty tablets, 2ش two
//     strips, 1ف one vial, 2ع two boxes;
//   - standing alone it names the form: "زجاجة ش" is a bottle of syrup.
//
// ك is expanded only after a figure, because standing alone it is also how
// vitamin K is written (فيتامين ك).

// afterFigure maps an abbreviation that follows a count.
var afterFigure = map[string]string{
	"ق": "قرص", "اق": "اقراص", "ك": "كبسولة", "كبس": "كبسولة",
	"ش": "شريط", "ف": "فيال", "امب": "امبول", "ع": "علبة",
}

// standalone maps an abbreviation that stands on its own.
var standalone = map[string]string{
	"ق": "قرص", "اق": "اقراص", "ش": "شراب", "امب": "امبول", "كبس": "كبسولة",
}

// afterDose maps an abbreviation that follows a strength — "500ملجم ف",
// "10مجم ك" — where it names the form of what was just measured.
var afterDose = map[string]string{
	"ق": "قرص", "اق": "اقراص", "ك": "كبسولة", "كبس": "كبسولة", "ش": "شراب",
	"ف": "فيال", "امب": "امبول",
}

// vitaminWords precede a single letter that is a vitamin, not a form.
var vitaminWords = map[string]bool{"فيتامين": true, "فيتامينات": true, "vitamin": true, "vit": true}

// ExpandShorthand rewrites Egyptian form abbreviations into full words.
func ExpandShorthand(text string) string {
	if text == "" {
		return text
	}
	fields := strings.Fields(sheet.NormalizeDigits(text))
	out := make([]string, 0, len(fields)+4)
	changed := false
	prevFigure := false
	prevDose := false
	prevWord := ""
	for _, f := range fields {
		num, suffix := splitFigure(f)
		switch {
		case num != "" && suffix != "":
			if full, ok := afterFigure[suffix]; ok {
				out = append(out, num, full)
				changed = true
				prevFigure, prevDose, prevWord = false, false, full
				continue
			}
		case num == "":
			key := strings.TrimRight(f, ".")
			if prevDose {
				if full, ok := afterDose[key]; ok {
					out = append(out, full)
					changed = true
					prevFigure, prevDose, prevWord = false, false, full
					continue
				}
			}
			if prevFigure {
				if full, ok := afterFigure[key]; ok {
					out = append(out, full)
					changed = true
					prevFigure, prevWord = false, full
					continue
				}
			} else if full, ok := standalone[key]; ok && !vitaminWords[prevWord] && prevWord != "" {
				out = append(out, full)
				changed = true
				prevFigure, prevWord = false, full
				continue
			}
		}
		out = append(out, f)
		prevFigure = num != "" && suffix == ""
		prevDose = hasDigit(f) && !prevFigure
		prevWord = f
	}
	if !changed {
		return text
	}
	return strings.Join(out, " ")
}

// splitFigure splits "30ق" into "30" and "ق". A word with no leading figure
// returns an empty number.
func splitFigure(w string) (num, suffix string) {
	i := 0
	for i < len(w) {
		r := rune(w[i])
		if (r >= '0' && r <= '9') || r == '.' {
			i++
			continue
		}
		break
	}
	if i == 0 {
		return "", w
	}
	rest := w[i:]
	if rest != "" && !unicode.IsLetter([]rune(rest)[0]) {
		return w, ""
	}
	return w[:i], rest
}
