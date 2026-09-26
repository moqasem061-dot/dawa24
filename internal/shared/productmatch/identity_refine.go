package productmatch

// Three identity attributes the engine did not read, each found in the
// labelled benchmark as a wrong match applied at full confidence:
//
//   - The ROUTE of an injection. "سيفاكسوتريز 1 جم وريد فيال" and the same
//     brand's "عضل فيال" are two products — an intravenous and an
//     intramuscular vial — and the supplier's "i.v." was matched to the
//     catalogue's عضل because neither word was read.
//   - The SIZE of a dressing, a sheet or a bandage. "سيليكون شيت 2.5*2.5سم" was
//     matched to the 10*20 sheet; worse, "10*20سم" was read as a carton of
//     two hundred, so the one figure that separated them was counted as
//     something else.
//   - Unit words written with a hamza. "30 جرأم" read as no weight at all, so a
//     30 g tube was applied to the 75 g one.
//
// Route and size are compared only where BOTH sides state one. A catalogue
// that writes "فيال" without a route has not contradicted a row that says
// "i.m.", and treating that silence as disagreement would refuse the common
// correct match to protect the rare wrong one.

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

// routeWords maps the words that state an injection's route onto a key.
var routeWords = map[string]string{
	"عضل": "im", "عضلي": "im", "عضلى": "im", "للعضل": "im", "بالعضل": "im",
	"intramuscular": "im", "im": "im",
	"وريد": "iv", "وريدي": "iv", "وريدى": "iv", "للوريد": "iv", "بالوريد": "iv",
	"intravenous": "iv", "iv": "iv",
	"subcutaneous": "sc",
}

// routesIn is the set of routes a name states.
//
// "i.m./i.v." normalises to the letters "i m i v", so single letters are read
// in pairs; "im" and "iv" as whole tokens are read directly.
func routesIn(text string) map[string]struct{} {
	words := strings.Fields(sheet.NormalizeName(text))
	var out map[string]struct{}
	add := func(k string) {
		if k == "" {
			return
		}
		if out == nil {
			out = make(map[string]struct{}, 2)
		}
		out[k] = struct{}{}
	}
	for i, w := range words {
		if k, ok := routeWords[w]; ok {
			add(k)
			continue
		}
		if i+1 >= len(words) {
			continue
		}
		next := words[i+1]
		switch {
		case w == "i" && next == "m":
			add("im")
		case w == "i" && next == "v":
			add("iv")
		case w == "s" && next == "c":
			add("sc")
		case w == "تحت" && (next == "الجلد" || next == "جلد"):
			add("sc")
		}
	}
	return out
}

// routesConflict reports two stated routes with nothing in common.
func routesConflict(a, b map[string]struct{}) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	for k := range a {
		if _, ok := b[k]; ok {
			return false
		}
	}
	return true
}

// dimensionPattern finds a written size: two figures joined by *, x or ×, and
// a length unit.
var dimensionPattern = regexp.MustCompile(
	`(\d+(?:[.,]\d+)?)\s*[*x×X]\s*(\d+(?:[.,]\d+)?)\s*(سم|cm|مم|ملم|mm)`)

// dimensionsIn is the set of sizes a name states, each normalised to
// millimetres with the smaller side first, so "10*20سم" and "200 x 100 mm" are
// one size.
func dimensionsIn(text string) []string {
	norm := sheet.NormalizeDigits(text)
	if !strings.ContainsAny(norm, "*xX×") {
		return nil
	}
	var out []string
	for _, m := range dimensionPattern.FindAllStringSubmatch(norm, -1) {
		a, errA := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
		b, errB := strconv.ParseFloat(strings.ReplaceAll(m[2], ",", "."), 64)
		if errA != nil || errB != nil || a <= 0 || b <= 0 {
			continue
		}
		scale := 1.0
		if m[3] == "سم" || m[3] == "cm" {
			scale = 10
		}
		sides := []float64{a * scale, b * scale}
		sort.Float64s(sides)
		key := strconv.FormatFloat(sides[0], 'f', -1, 64) + "x" +
			strconv.FormatFloat(sides[1], 'f', -1, 64)
		out = append(out, key)
	}
	return out
}

// dimensionsConflict reports two stated sizes with nothing in common.
func dimensionsConflict(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return false
			}
		}
	}
	return true
}

// isDimensionMultiplier reports whether the figures around a multiplication
// sign are a size rather than a carton: a length unit follows the right
// factor, or either factor is a decimal. "2.5*2.5سم" is a dressing, and "20*10"
// followed by قرص is a carton.
func isDimensionMultiplier(runes []rune, leftStart, rightEnd int, out []rune) bool {
	if leftStart > 0 {
		if prev := out[leftStart-1]; prev == '.' || prev == ',' {
			return true
		}
	}
	rest := runes[rightEnd:]
	if len(rest) > 1 && (rest[0] == '.' || rest[0] == ',') && rest[1] >= '0' && rest[1] <= '9' {
		return true
	}
	tail := strings.TrimLeft(string(rest), " ")
	for _, unit := range []string{"سم", "cm", "مم", "ملم", "mm"} {
		if strings.HasPrefix(strings.ToLower(tail), unit) {
			return true
		}
	}
	return false
}

// foldAlef folds the hamza-carrying alefs onto the bare one, as the name
// normaliser already does, so a unit word spelled "جرأم" reads as grams.
func foldAlef(s string) string {
	if !strings.ContainsAny(s, "أإآٱ") {
		return s
	}
	return strings.NewReplacer("أ", "ا", "إ", "ا", "آ", "ا", "ٱ", "ا").Replace(s)
}
